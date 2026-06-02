// Package postgres provides the PostgreSQL implementation of the organization,
// user, and API key repositories for the multi-tenant SaaS layer.
//
// Design decisions:
//
//  1. database/sql instead of GORM or sqlx.
//     The standard library's database/sql provides connection pooling, prepared
//     statements, and transaction support without adding ORM complexity.
//     For a CRUD-heavy SaaS layer with 6 tables, raw SQL is clearer and more
//     performant than an ORM. We use pg-specific features ($1 placeholders,
//     RETURNING, ON CONFLICT) directly.
//
//  2. Single Repository struct.
//     Unlike the event ingestion layer (which has separate Kafka and ClickHouse
//     writers), the SaaS repositories share a single PostgreSQL connection pool.
//     A single Repository struct with method receivers avoids duplicating the
//     *sql.DB reference across 3 interface implementations.
//
//  3. Context-based timeouts.
//     Every database call accepts a context.Context for cancellation and timeout
//     propagation from the HTTP handler layer. The default query timeout is
//     inherited from the parent context (typically 10s from HTTP middleware).
//
//  4. Soft deletes.
//     All DELETE operations set deleted_at = NOW() instead of physically removing
//     rows. Queries filter WHERE deleted_at IS NULL by default.
package postgres

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/argus-ai/event-collector/internal/application/port"
	"github.com/argus-ai/event-collector/internal/domain/entity"
	"go.uber.org/zap"

	// PostgreSQL driver.
	_ "github.com/lib/pq"
)

// ---------------------------------------------------------------------------
// Repository
// ---------------------------------------------------------------------------

// Repository implements OrgRepository, UserRepository, and APIKeyRepository
// against a PostgreSQL database.
type Repository struct {
	db     *sql.DB
	logger *zap.Logger
}

// Config holds PostgreSQL connection parameters.
type Config struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Database string `yaml:"database"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	SSLMode  string `yaml:"ssl_mode"`

	// Connection pool settings.
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
}

// DefaultConfig returns sensible defaults for local development.
func DefaultConfig() Config {
	return Config{
		Host:            "localhost",
		Port:            5432,
		Database:        "argus",
		Username:        "argus",
		Password:        "argus",
		SSLMode:         "disable",
		MaxOpenConns:    25,
		MaxIdleConns:    10,
		ConnMaxLifetime: 5 * time.Minute,
	}
}

// NewRepository creates a new PostgreSQL repository with connection pooling.
func NewRepository(cfg Config, logger *zap.Logger) (*Repository, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.Database, cfg.SSLMode,
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: open connection: %w", err)
	}

	// Configure connection pool.
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// Verify connection.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("postgres: ping failed: %w", err)
	}

	logger.Info("PostgreSQL connected",
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port),
		zap.String("database", cfg.Database),
	)

	return &Repository{db: db, logger: logger.Named("postgres")}, nil
}

// Close closes the database connection pool.
func (r *Repository) Close() error {
	return r.db.Close()
}

// DB exposes the underlying *sql.DB for transaction management.
// Handlers that need atomic multi-table operations (e.g., create org + admin
// user in one transaction) use this to call BeginTx.
func (r *Repository) DB() *sql.DB {
	return r.db
}

// Ping checks database connectivity.
func (r *Repository) Ping(ctx context.Context) error {
	return r.db.PingContext(ctx)
}

// EnsureLiveKitRecordingsTable creates the recording metadata table used to map
// LiveKit Egress webhooks back to proctoring sessions.
func (r *Repository) EnsureLiveKitRecordingsTable(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS livekit_recordings (
			egress_id      TEXT PRIMARY KEY,
			session_id     TEXT NOT NULL,
			user_id        TEXT NOT NULL,
			room_name      TEXT NOT NULL,
			video_track_id TEXT NOT NULL DEFAULT '',
			audio_track_id TEXT NOT NULL DEFAULT '',
			status         TEXT NOT NULL,
			file_url       TEXT NOT NULL DEFAULT '',
			error_message  TEXT NOT NULL DEFAULT '',
			started_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			ended_at       TIMESTAMPTZ,
			created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_livekit_recordings_session
			ON livekit_recordings (session_id, created_at DESC);

		CREATE INDEX IF NOT EXISTS idx_livekit_recordings_user
			ON livekit_recordings (user_id, created_at DESC);
	`
	if _, err := r.db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("postgres: ensure livekit_recordings table: %w", err)
	}
	return nil
}

func (r *Repository) CreateRecording(ctx context.Context, rec *entity.Recording) error {
	query := `
		INSERT INTO livekit_recordings (
			egress_id, session_id, user_id, room_name,
			video_track_id, audio_track_id, status, file_url, error_message
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (egress_id) DO UPDATE SET
			session_id = EXCLUDED.session_id,
			user_id = EXCLUDED.user_id,
			room_name = EXCLUDED.room_name,
			video_track_id = EXCLUDED.video_track_id,
			audio_track_id = EXCLUDED.audio_track_id,
			status = EXCLUDED.status,
			file_url = COALESCE(NULLIF(EXCLUDED.file_url, ''), livekit_recordings.file_url),
			error_message = COALESCE(NULLIF(EXCLUDED.error_message, ''), livekit_recordings.error_message),
			updated_at = NOW()
		RETURNING started_at, created_at, updated_at`

	if err := r.db.QueryRowContext(ctx, query,
		rec.EgressID,
		rec.SessionID,
		rec.UserID,
		rec.RoomName,
		rec.VideoTrackID,
		rec.AudioTrackID,
		rec.Status,
		rec.FileURL,
		rec.ErrorMessage,
	).Scan(&rec.StartedAt, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return fmt.Errorf("postgres: create livekit recording: %w", err)
	}
	return nil
}

func (r *Repository) UpdateRecordingStopped(ctx context.Context, egressID string) error {
	query := `
		UPDATE livekit_recordings
		SET status = 'EGRESS_ENDING',
			updated_at = NOW()
		WHERE egress_id = $1`
	res, err := r.db.ExecContext(ctx, query, egressID)
	if err != nil {
		return fmt.Errorf("postgres: mark livekit recording stopping: %w", err)
	}
	if rows, err := res.RowsAffected(); err == nil && rows == 0 {
		r.logger.Warn("livekit recording not found when marking stopping", zap.String("egress_id", egressID))
	}
	return nil
}

func (r *Repository) UpdateRecordingEnded(ctx context.Context, egressID, status, fileURL, errorMessage string) error {
	query := `
		UPDATE livekit_recordings
		SET status = $2,
			file_url = COALESCE(NULLIF($3, ''), file_url),
			error_message = COALESCE(NULLIF($4, ''), error_message),
			ended_at = COALESCE(ended_at, NOW()),
			updated_at = NOW()
		WHERE egress_id = $1`
	res, err := r.db.ExecContext(ctx, query, egressID, status, fileURL, errorMessage)
	if err != nil {
		return fmt.Errorf("postgres: update livekit recording ended: %w", err)
	}
	if rows, err := res.RowsAffected(); err == nil && rows == 0 {
		r.logger.Warn("livekit recording not found for egress webhook", zap.String("egress_id", egressID))
	}
	return nil
}

// RetentionCleanupParams describes a bounded retention cleanup pass for one org.
type RetentionCleanupParams struct {
	OrgID              string
	VideoRetentionDays int
	AuditRetentionDays int
	DryRun             bool
}

// RetentionCleanupResult summarizes what a retention cleanup did or would do.
type RetentionCleanupResult struct {
	OrgID              string `json:"orgId"`
	DryRun             bool   `json:"dryRun"`
	VideoRetentionDays int    `json:"videoRetentionDays"`
	AuditRetentionDays int    `json:"auditRetentionDays"`
	VideoCutoff        string `json:"videoCutoff"`
	AuditCutoff        string `json:"auditCutoff"`
	RecordingsMatched  int64  `json:"recordingsMatched"`
	RecordingsDeleted  int64  `json:"recordingsDeleted"`
	AuditMatched       int64  `json:"auditMatched"`
	AuditDeleted       int64  `json:"auditDeleted"`
}

// ApplyRetentionCleanup removes old PostgreSQL metadata for a single org.
// It intentionally does not delete S3/MinIO objects; file_url values are
// metadata only here, and object deletion must be wired through the object store
// with governance/retention checks.
func (r *Repository) ApplyRetentionCleanup(ctx context.Context, params RetentionCleanupParams) (*RetentionCleanupResult, error) {
	if params.OrgID == "" {
		return nil, fmt.Errorf("postgres: retention cleanup: org_id is required")
	}
	if params.VideoRetentionDays <= 0 {
		params.VideoRetentionDays = 180
	}
	if params.AuditRetentionDays <= 0 {
		params.AuditRetentionDays = 365
	}

	now := time.Now().UTC()
	videoCutoff := now.AddDate(0, 0, -params.VideoRetentionDays)
	auditCutoff := now.AddDate(0, 0, -params.AuditRetentionDays)

	result := &RetentionCleanupResult{
		OrgID:              params.OrgID,
		DryRun:             params.DryRun,
		VideoRetentionDays: params.VideoRetentionDays,
		AuditRetentionDays: params.AuditRetentionDays,
		VideoCutoff:        videoCutoff.Format(time.RFC3339),
		AuditCutoff:        auditCutoff.Format(time.RFC3339),
	}

	recordingCountQuery := `
		SELECT COUNT(*)
		FROM livekit_recordings lr
		JOIN external_sessions es ON es.session_id = lr.session_id
		WHERE es.org_id = $1 AND lr.created_at < $2`
	if err := r.db.QueryRowContext(ctx, recordingCountQuery, params.OrgID, videoCutoff).Scan(&result.RecordingsMatched); err != nil {
		return nil, fmt.Errorf("postgres: count old livekit recordings: %w", err)
	}

	auditCountQuery := `SELECT COUNT(*) FROM audit_log WHERE org_id = $1 AND created_at < $2`
	if err := r.db.QueryRowContext(ctx, auditCountQuery, params.OrgID, auditCutoff).Scan(&result.AuditMatched); err != nil {
		return nil, fmt.Errorf("postgres: count old audit log: %w", err)
	}

	if params.DryRun {
		return result, nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("postgres: retention cleanup: begin tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && rbErr != sql.ErrTxDone {
			r.logger.Error("retention cleanup rollback failed", zap.Error(rbErr))
		}
	}()

	recordingDeleteQuery := `
		DELETE FROM livekit_recordings lr
		USING external_sessions es
		WHERE es.session_id = lr.session_id
			AND es.org_id = $1
			AND lr.created_at < $2`
	recordingDelete, err := tx.ExecContext(ctx, recordingDeleteQuery, params.OrgID, videoCutoff)
	if err != nil {
		return nil, fmt.Errorf("postgres: delete old livekit recording metadata: %w", err)
	}
	result.RecordingsDeleted, _ = recordingDelete.RowsAffected()

	auditDeleteQuery := `DELETE FROM audit_log WHERE org_id = $1 AND created_at < $2`
	auditDelete, err := tx.ExecContext(ctx, auditDeleteQuery, params.OrgID, auditCutoff)
	if err != nil {
		return nil, fmt.Errorf("postgres: delete old audit log: %w", err)
	}
	result.AuditDeleted, _ = auditDelete.RowsAffected()

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("postgres: retention cleanup: commit: %w", err)
	}

	return result, nil
}

// ==========================================================================
// Organization Repository Implementation
// ==========================================================================

func (r *Repository) CreateOrg(ctx context.Context, org *entity.Organization) error {
	featuresJSON, err := json.Marshal(org.AllowedFeatures)
	if err != nil {
		return fmt.Errorf("postgres: marshal allowed_features: %w", err)
	}
	if org.AllowedFeatures == nil {
		featuresJSON = []byte("{}")
	}

	query := `
		INSERT INTO organizations (org_id, name, slug, org_type, contact_email, contact_phone,
			city, region, plan, max_sessions, max_events_rps, retention_days, is_active,
			allowed_features, session_limit, sessions_used, trial_ends_at,
			created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
			$14, $15, $16, $17, $18, $18)
		RETURNING id, created_at, updated_at`

	return r.db.QueryRowContext(ctx, query,
		org.OrgID, org.Name, org.Slug, org.OrgType,
		org.ContactEmail, org.ContactPhone, org.City, org.Region,
		string(org.Plan), org.MaxSessions, org.MaxEventsRPS, org.RetentionDays,
		org.IsActive,
		featuresJSON, org.SessionLimit, org.SessionsUsed, org.TrialEndsAt,
		org.CreatedBy,
	).Scan(&org.ID, &org.CreatedAt, &org.UpdatedAt)
}

func (r *Repository) GetOrgByOrgID(ctx context.Context, orgID string) (*entity.Organization, error) {
	query := `
		SELECT id, org_id, name, slug, org_type,
			COALESCE(contact_email, ''), COALESCE(contact_phone, ''),
			COALESCE(city, ''), COALESCE(region, ''),
			plan, max_sessions, max_events_rps, retention_days,
			is_active, created_at, updated_at, deleted_at, created_by, updated_by,
			COALESCE(allowed_features, '{}'::jsonb), session_limit, sessions_used, trial_ends_at
		FROM organizations
		WHERE org_id = $1 AND deleted_at IS NULL`

	org := &entity.Organization{}
	var plan string
	var featuresJSON []byte
	err := r.db.QueryRowContext(ctx, query, orgID).Scan(
		&org.ID, &org.OrgID, &org.Name, &org.Slug, &org.OrgType,
		&org.ContactEmail, &org.ContactPhone, &org.City, &org.Region,
		&plan, &org.MaxSessions, &org.MaxEventsRPS, &org.RetentionDays,
		&org.IsActive, &org.CreatedAt, &org.UpdatedAt, &org.DeletedAt,
		&org.CreatedBy, &org.UpdatedBy,
		&featuresJSON, &org.SessionLimit, &org.SessionsUsed, &org.TrialEndsAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: get org: %w", err)
	}
	org.Plan = entity.Plan(plan)
	if len(featuresJSON) > 0 {
		if err := json.Unmarshal(featuresJSON, &org.AllowedFeatures); err != nil {
			r.logger.Warn("Failed to unmarshal allowed_features",
				zap.Error(err), zap.String("org_id", orgID))
		}
	}
	return org, nil
}

func (r *Repository) GetOrgBySlug(ctx context.Context, slug string) (*entity.Organization, error) {
	query := `
		SELECT id, org_id, name, slug, org_type,
			COALESCE(contact_email, ''), COALESCE(contact_phone, ''),
			COALESCE(city, ''), COALESCE(region, ''),
			plan, max_sessions, max_events_rps, retention_days,
			is_active, created_at, updated_at, deleted_at, created_by, updated_by,
			COALESCE(allowed_features, '{}'::jsonb), session_limit, sessions_used, trial_ends_at
		FROM organizations
		WHERE slug = $1 AND deleted_at IS NULL`

	org := &entity.Organization{}
	var plan string
	var featuresJSON []byte
	err := r.db.QueryRowContext(ctx, query, slug).Scan(
		&org.ID, &org.OrgID, &org.Name, &org.Slug, &org.OrgType,
		&org.ContactEmail, &org.ContactPhone, &org.City, &org.Region,
		&plan, &org.MaxSessions, &org.MaxEventsRPS, &org.RetentionDays,
		&org.IsActive, &org.CreatedAt, &org.UpdatedAt, &org.DeletedAt,
		&org.CreatedBy, &org.UpdatedBy,
		&featuresJSON, &org.SessionLimit, &org.SessionsUsed, &org.TrialEndsAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: get org by slug: %w", err)
	}
	org.Plan = entity.Plan(plan)
	if len(featuresJSON) > 0 {
		if err := json.Unmarshal(featuresJSON, &org.AllowedFeatures); err != nil {
			r.logger.Warn("Failed to unmarshal allowed_features",
				zap.Error(err), zap.String("slug", slug))
		}
	}
	return org, nil
}

func (r *Repository) ListOrgs(ctx context.Context, filter port.OrgFilter) ([]*entity.Organization, error) {
	var conditions []string
	var args []interface{}
	argIndex := 1

	conditions = append(conditions, "deleted_at IS NULL")

	if filter.OrgType != "" {
		conditions = append(conditions, fmt.Sprintf("org_type = $%d", argIndex))
		args = append(args, filter.OrgType)
		argIndex++
	}
	if filter.Plan != "" {
		conditions = append(conditions, fmt.Sprintf("plan = $%d", argIndex))
		args = append(args, filter.Plan)
		argIndex++
	}
	if filter.IsActive != nil {
		conditions = append(conditions, fmt.Sprintf("is_active = $%d", argIndex))
		args = append(args, *filter.IsActive)
		argIndex++
	}
	if filter.Search != "" {
		conditions = append(conditions, fmt.Sprintf(
			"(name ILIKE $%d OR slug ILIKE $%d OR city ILIKE $%d)",
			argIndex, argIndex, argIndex,
		))
		args = append(args, "%"+filter.Search+"%")
	}

	// Exclude the internal Super Admin organization from listings.
	conditions = append(conditions, "org_id != '*'")

	query := fmt.Sprintf(`
		SELECT id, org_id, name, slug, org_type,
			COALESCE(contact_email, ''), COALESCE(contact_phone, ''),
			COALESCE(city, ''), COALESCE(region, ''),
			plan, max_sessions, max_events_rps, retention_days,
			is_active, created_at, updated_at, created_by, updated_by,
			COALESCE(allowed_features, '{}'::jsonb), session_limit, sessions_used, trial_ends_at
		FROM organizations
		WHERE %s
		ORDER BY name ASC`, strings.Join(conditions, " AND "))

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list orgs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	orgs := make([]*entity.Organization, 0)
	for rows.Next() {
		org := &entity.Organization{}
		var plan string
		var featuresJSON []byte
		if err := rows.Scan(
			&org.ID, &org.OrgID, &org.Name, &org.Slug, &org.OrgType,
			&org.ContactEmail, &org.ContactPhone, &org.City, &org.Region,
			&plan, &org.MaxSessions, &org.MaxEventsRPS, &org.RetentionDays,
			&org.IsActive, &org.CreatedAt, &org.UpdatedAt,
			&org.CreatedBy, &org.UpdatedBy,
			&featuresJSON, &org.SessionLimit, &org.SessionsUsed, &org.TrialEndsAt,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan org: %w", err)
		}
		org.Plan = entity.Plan(plan)
		if len(featuresJSON) > 0 {
			if err := json.Unmarshal(featuresJSON, &org.AllowedFeatures); err != nil {
				r.logger.Warn("Failed to unmarshal allowed_features",
					zap.Error(err), zap.String("org_id", org.OrgID))
			}
		}
		orgs = append(orgs, org)
	}

	return orgs, rows.Err()
}

func (r *Repository) UpdateOrg(ctx context.Context, org *entity.Organization) error {
	featuresJSON, err := json.Marshal(org.AllowedFeatures)
	if err != nil {
		return fmt.Errorf("postgres: marshal allowed_features: %w", err)
	}
	if org.AllowedFeatures == nil {
		featuresJSON = []byte("{}")
	}

	query := `
		UPDATE organizations
		SET name = $2, slug = $3, org_type = $4, contact_email = $5, contact_phone = $6,
			city = $7, region = $8, plan = $9, max_sessions = $10, max_events_rps = $11,
			retention_days = $12, is_active = $13,
			allowed_features = $14, session_limit = $15, sessions_used = $16, trial_ends_at = $17,
			updated_at = NOW(), updated_by = $18
		WHERE org_id = $1 AND deleted_at IS NULL`

	_, err = r.db.ExecContext(ctx, query,
		org.OrgID, org.Name, org.Slug, org.OrgType,
		org.ContactEmail, org.ContactPhone, org.City, org.Region,
		string(org.Plan), org.MaxSessions, org.MaxEventsRPS, org.RetentionDays,
		org.IsActive,
		featuresJSON, org.SessionLimit, org.SessionsUsed, org.TrialEndsAt,
		org.UpdatedBy,
	)
	return err
}

func (r *Repository) SoftDeleteOrg(ctx context.Context, orgID string) error {
	query := `UPDATE organizations SET deleted_at = NOW() WHERE org_id = $1 AND deleted_at IS NULL`
	_, err := r.db.ExecContext(ctx, query, orgID)
	return err
}

func (r *Repository) CountOrgs(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM organizations WHERE deleted_at IS NULL AND org_id != '*'`,
	).Scan(&count)
	return count, err
}

// ==========================================================================
// User Repository Implementation
// ==========================================================================

func (r *Repository) CreateUser(ctx context.Context, user *entity.User) error {
	query := `
		INSERT INTO users (org_id, phone, password_hash, full_name, email, role, is_active, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
		RETURNING id, created_at, updated_at`

	return r.db.QueryRowContext(ctx, query,
		user.OrgID, user.Phone, user.PasswordHash,
		user.FullName, user.Email, string(user.Role),
		user.IsActive, user.CreatedBy,
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
}

func (r *Repository) GetUserByID(ctx context.Context, userID string) (*entity.User, error) {
	query := `
		SELECT id, org_id, phone, password_hash, full_name, COALESCE(email, ''), role,
			is_active, last_login_at, created_at, updated_at, deleted_at, created_by, updated_by
		FROM users
		WHERE id = $1 AND deleted_at IS NULL`

	return r.scanUser(r.db.QueryRowContext(ctx, query, userID))
}

func (r *Repository) GetUserByPhone(ctx context.Context, phone string) (*entity.User, error) {
	query := `
		SELECT id, org_id, phone, password_hash, full_name, COALESCE(email, ''), role,
			is_active, last_login_at, created_at, updated_at, deleted_at, created_by, updated_by
		FROM users
		WHERE phone = $1 AND deleted_at IS NULL`

	return r.scanUser(r.db.QueryRowContext(ctx, query, phone))
}

func (r *Repository) ListUsersByOrg(ctx context.Context, orgID string) ([]*entity.User, error) {
	query := `
		SELECT id, org_id, phone, full_name, COALESCE(email, ''), role,
			is_active, last_login_at, created_at, updated_at, created_by, updated_by
		FROM users
		WHERE org_id = $1 AND deleted_at IS NULL
		ORDER BY full_name ASC`

	rows, err := r.db.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list users by org: %w", err)
	}
	defer func() { _ = rows.Close() }()

	users := make([]*entity.User, 0)
	for rows.Next() {
		u := &entity.User{}
		var role string
		if err := rows.Scan(
			&u.ID, &u.OrgID, &u.Phone, &u.FullName, &u.Email, &role,
			&u.IsActive, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt,
			&u.CreatedBy, &u.UpdatedBy,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan user: %w", err)
		}
		u.Role = entity.Role(role)
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *Repository) ListAllUsers(ctx context.Context, filter port.UserFilter) ([]*entity.User, error) {
	var conditions []string
	var args []interface{}
	argIndex := 1

	conditions = append(conditions, "deleted_at IS NULL")

	if filter.OrgID != "" {
		conditions = append(conditions, fmt.Sprintf("org_id = $%d", argIndex))
		args = append(args, filter.OrgID)
		argIndex++
	}
	if filter.Role != "" {
		conditions = append(conditions, fmt.Sprintf("role = $%d", argIndex))
		args = append(args, string(filter.Role))
		argIndex++
	}
	if filter.Search != "" {
		conditions = append(conditions, fmt.Sprintf(
			"(full_name ILIKE $%d OR phone ILIKE $%d OR email ILIKE $%d)",
			argIndex, argIndex, argIndex,
		))
		args = append(args, "%"+filter.Search+"%")
	}

	query := fmt.Sprintf(`
		SELECT id, org_id, phone, full_name, COALESCE(email, ''), role,
			is_active, last_login_at, created_at, updated_at, created_by, updated_by
		FROM users
		WHERE %s
		ORDER BY created_at DESC`, strings.Join(conditions, " AND "))

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list all users: %w", err)
	}
	defer rows.Close()

	users := make([]*entity.User, 0)
	for rows.Next() {
		u := &entity.User{}
		var role string
		if err := rows.Scan(
			&u.ID, &u.OrgID, &u.Phone, &u.FullName, &u.Email, &role,
			&u.IsActive, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt,
			&u.CreatedBy, &u.UpdatedBy,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan user: %w", err)
		}
		u.Role = entity.Role(role)
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *Repository) UpdateUser(ctx context.Context, user *entity.User) error {
	query := `
		UPDATE users
		SET full_name = $2, email = $3, role = $4, is_active = $5,
			updated_at = NOW(), updated_by = $6
		WHERE id = $1 AND deleted_at IS NULL`

	_, err := r.db.ExecContext(ctx, query,
		user.ID, user.FullName, user.Email, string(user.Role),
		user.IsActive, user.UpdatedBy,
	)
	return err
}

func (r *Repository) UpdateUserLastLogin(ctx context.Context, userID string) error {
	query := `UPDATE users SET last_login_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, userID)
	return err
}

func (r *Repository) SoftDeleteUser(ctx context.Context, userID string) error {
	query := `UPDATE users SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`
	_, err := r.db.ExecContext(ctx, query, userID)
	return err
}

// scanUser scans a single user row.
func (r *Repository) scanUser(row *sql.Row) (*entity.User, error) {
	u := &entity.User{}
	var role string
	err := row.Scan(
		&u.ID, &u.OrgID, &u.Phone, &u.PasswordHash, &u.FullName, &u.Email, &role,
		&u.IsActive, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt, &u.DeletedAt,
		&u.CreatedBy, &u.UpdatedBy,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: scan user: %w", err)
	}
	u.Role = entity.Role(role)
	return u, nil
}

// ==========================================================================
// API Key Repository Implementation
// ==========================================================================

func (r *Repository) CreateAPIKey(ctx context.Context, key *entity.APIKey) error {
	query := `
		INSERT INTO api_keys (org_id, name, key_id, secret_hash, secret_prefix,
			permissions, rate_limit_rps, is_active, expires_at, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)
		RETURNING id, created_at, updated_at`

	return r.db.QueryRowContext(ctx, query,
		key.OrgID, key.Name, key.KeyID, key.SecretHash, key.SecretPrefix,
		pqStringArray(key.Permissions), key.RateLimitRPS, key.IsActive,
		key.ExpiresAt, key.CreatedBy,
	).Scan(&key.ID, &key.CreatedAt, &key.UpdatedAt)
}

func (r *Repository) GetAPIKeyByKeyID(ctx context.Context, keyID string) (*entity.APIKey, error) {
	query := `
		SELECT id, org_id, name, key_id, secret_hash, secret_prefix,
			permissions, rate_limit_rps, is_active, expires_at, last_used_at,
			created_at, updated_at, deleted_at, created_by, updated_by
		FROM api_keys
		WHERE key_id = $1 AND deleted_at IS NULL`

	return r.scanAPIKey(r.db.QueryRowContext(ctx, query, keyID))
}

func (r *Repository) ListAPIKeysByOrg(ctx context.Context, orgID string) ([]*entity.APIKey, error) {
	query := `
		SELECT id, org_id, name, key_id, secret_prefix,
			permissions, rate_limit_rps, is_active, expires_at, last_used_at,
			created_at, updated_at, created_by, updated_by
		FROM api_keys
		WHERE org_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC`

	rows, err := r.db.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list api keys: %w", err)
	}
	defer rows.Close()

	keys := make([]*entity.APIKey, 0)
	for rows.Next() {
		k := &entity.APIKey{}
		var perms string
		if err := rows.Scan(
			&k.ID, &k.OrgID, &k.Name, &k.KeyID, &k.SecretPrefix,
			&perms, &k.RateLimitRPS, &k.IsActive, &k.ExpiresAt, &k.LastUsedAt,
			&k.CreatedAt, &k.UpdatedAt, &k.CreatedBy, &k.UpdatedBy,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan api key: %w", err)
		}
		k.Permissions = parsePgArray(perms)
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (r *Repository) ListAllAPIKeys(ctx context.Context) ([]*entity.APIKey, error) {
	query := `
		SELECT id, org_id, name, key_id, secret_prefix,
			permissions, rate_limit_rps, is_active, expires_at, last_used_at,
			created_at, updated_at, created_by, updated_by
		FROM api_keys
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("postgres: list all api keys: %w", err)
	}
	defer rows.Close()

	keys := make([]*entity.APIKey, 0)
	for rows.Next() {
		k := &entity.APIKey{}
		var perms string
		if err := rows.Scan(
			&k.ID, &k.OrgID, &k.Name, &k.KeyID, &k.SecretPrefix,
			&perms, &k.RateLimitRPS, &k.IsActive, &k.ExpiresAt, &k.LastUsedAt,
			&k.CreatedAt, &k.UpdatedAt, &k.CreatedBy, &k.UpdatedBy,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan api key: %w", err)
		}
		k.Permissions = parsePgArray(perms)
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (r *Repository) UpdateAPIKey(ctx context.Context, key *entity.APIKey) error {
	query := `
		UPDATE api_keys
		SET name = $2, permissions = $3, rate_limit_rps = $4, is_active = $5,
			expires_at = $6, updated_at = NOW(), updated_by = $7
		WHERE key_id = $1 AND deleted_at IS NULL`

	_, err := r.db.ExecContext(ctx, query,
		key.KeyID, key.Name, pqStringArray(key.Permissions),
		key.RateLimitRPS, key.IsActive, key.ExpiresAt, key.UpdatedBy,
	)
	return err
}

func (r *Repository) RevokeAPIKey(ctx context.Context, keyID string) error {
	query := `UPDATE api_keys SET is_active = false, updated_at = NOW() WHERE key_id = $1`
	_, err := r.db.ExecContext(ctx, query, keyID)
	return err
}

func (r *Repository) UpdateAPIKeyLastUsed(ctx context.Context, keyID string) error {
	query := `UPDATE api_keys SET last_used_at = NOW() WHERE key_id = $1`
	_, err := r.db.ExecContext(ctx, query, keyID)
	return err
}

func (r *Repository) SoftDeleteAPIKey(ctx context.Context, keyID string) error {
	query := `UPDATE api_keys SET deleted_at = NOW() WHERE key_id = $1 AND deleted_at IS NULL`
	_, err := r.db.ExecContext(ctx, query, keyID)
	return err
}

// scanAPIKey scans a single API key row.
func (r *Repository) scanAPIKey(row *sql.Row) (*entity.APIKey, error) {
	k := &entity.APIKey{}
	var perms string
	err := row.Scan(
		&k.ID, &k.OrgID, &k.Name, &k.KeyID, &k.SecretHash, &k.SecretPrefix,
		&perms, &k.RateLimitRPS, &k.IsActive, &k.ExpiresAt, &k.LastUsedAt,
		&k.CreatedAt, &k.UpdatedAt, &k.DeletedAt, &k.CreatedBy, &k.UpdatedBy,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: scan api key: %w", err)
	}
	k.Permissions = parsePgArray(perms)
	return k, nil
}

// ==========================================================================
// API Key Generation Utilities
// ==========================================================================

// GenerateAPIKeyPair creates a new API key ID and secret.
// Returns (keyID, rawSecret) — the rawSecret is shown once and never stored.
func GenerateAPIKeyPair(env string) (keyID string, rawSecret string, err error) {
	// Key ID: argus_{env}_{16 random hex chars}
	keyRandom := make([]byte, 16)
	if _, err := rand.Read(keyRandom); err != nil {
		return "", "", fmt.Errorf("generate key: %w", err)
	}
	keyID = fmt.Sprintf("argus_%s_%s", env, hex.EncodeToString(keyRandom))

	// Secret: sk_{env}_{32 random hex chars}
	secretRandom := make([]byte, 32)
	if _, err := rand.Read(secretRandom); err != nil {
		return "", "", fmt.Errorf("generate secret: %w", err)
	}
	rawSecret = fmt.Sprintf("sk_%s_%s", env, hex.EncodeToString(secretRandom))

	return keyID, rawSecret, nil
}

// ==========================================================================
// PostgreSQL Array Helpers
// ==========================================================================

// pqStringArray converts a Go string slice to a PostgreSQL array literal.
func pqStringArray(arr []string) string {
	if len(arr) == 0 {
		return "{}"
	}
	escaped := make([]string, len(arr))
	for i, s := range arr {
		escaped[i] = `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return "{" + strings.Join(escaped, ",") + "}"
}

// parsePgArray parses a PostgreSQL array string into a Go string slice.
func parsePgArray(s string) []string {
	if s == "{}" || s == "" {
		return []string{}
	}
	// Remove braces.
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")

	var result []string
	for _, item := range strings.Split(s, ",") {
		item = strings.TrimSpace(item)
		item = strings.Trim(item, `"`)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

// ==========================================================================
// Audit Log Repository Implementation
// ==========================================================================

// AuditFilter specifies criteria for querying the audit log.
type AuditFilter struct {
	OrgID        string
	UserID       string
	Action       string
	ResourceType string
	Limit        int
	Offset       int
}

// CreateAuditEntry inserts a new audit log record.
// The details field is marshalled to JSON for storage in the JSONB column.
func (r *Repository) CreateAuditEntry(ctx context.Context, entry *entity.AuditEntry) error {
	// Marshal details to JSON.
	var detailsJSON []byte
	var err error
	if entry.Details != nil {
		detailsJSON, err = json.Marshal(entry.Details)
		if err != nil {
			r.logger.Warn("audit: failed to marshal details, storing null",
				zap.Error(err),
				zap.String("action", entry.Action),
			)
			detailsJSON = nil
		}
	}

	query := `
		INSERT INTO audit_log (user_id, user_phone, user_role, org_id,
			action, resource_type, resource_id, details, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at`

	var userID interface{}
	if entry.UserID != "" {
		userID = entry.UserID
	}

	return r.db.QueryRowContext(ctx, query,
		userID, entry.UserPhone, entry.UserRole, entry.OrgID,
		entry.Action, entry.ResourceType, entry.ResourceID,
		detailsJSON, entry.IPAddress, entry.UserAgent,
	).Scan(&entry.ID, &entry.CreatedAt)
}

// ListAuditLogs queries the audit log with optional filtering and pagination.
func (r *Repository) ListAuditLogs(ctx context.Context, filter AuditFilter) ([]*entity.AuditEntry, error) {
	var conditions []string
	var args []interface{}
	argIndex := 1

	if filter.OrgID != "" {
		conditions = append(conditions, fmt.Sprintf("org_id = $%d", argIndex))
		args = append(args, filter.OrgID)
		argIndex++
	}
	if filter.UserID != "" {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", argIndex))
		args = append(args, filter.UserID)
		argIndex++
	}
	if filter.Action != "" {
		conditions = append(conditions, fmt.Sprintf("action = $%d", argIndex))
		args = append(args, filter.Action)
		argIndex++
	}
	if filter.ResourceType != "" {
		conditions = append(conditions, fmt.Sprintf("resource_type = $%d", argIndex))
		args = append(args, filter.ResourceType)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	query := fmt.Sprintf(`
		SELECT id, COALESCE(user_id::text, ''), COALESCE(user_phone, ''),
			COALESCE(user_role, ''), COALESCE(org_id, ''),
			action, resource_type, COALESCE(resource_id, ''),
			COALESCE(details::text, '{}'), COALESCE(ip_address, ''),
			COALESCE(user_agent, ''), created_at
		FROM audit_log
		%s
		ORDER BY created_at DESC
		LIMIT %d OFFSET %d`, whereClause, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list audit logs: %w", err)
	}
	defer rows.Close()

	entries := make([]*entity.AuditEntry, 0)
	for rows.Next() {
		e := &entity.AuditEntry{}
		var detailsStr string
		if err := rows.Scan(
			&e.ID, &e.UserID, &e.UserPhone, &e.UserRole, &e.OrgID,
			&e.Action, &e.ResourceType, &e.ResourceID,
			&detailsStr, &e.IPAddress, &e.UserAgent, &e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan audit entry: %w", err)
		}
		// Parse the JSONB details back into a map.
		if detailsStr != "" && detailsStr != "{}" {
			var details map[string]interface{}
			if err := json.Unmarshal([]byte(detailsStr), &details); err == nil {
				e.Details = details
			} else {
				e.Details = detailsStr
			}
		}
		entries = append(entries, e)
	}

	return entries, rows.Err()
}

// ==========================================================================
// Session Termination
// ==========================================================================

// EnsureSessionTerminationsTable creates the session_terminations table if it
// does not exist. Called during startup to auto-migrate.
//
// Multi-tenancy: org_id is required for tenant data isolation. The unique
// constraint is on (org_id, session_id) to prevent cross-org collisions.
func (r *Repository) EnsureSessionTerminationsTable(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS session_terminations (
			id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			org_id          VARCHAR(64) NOT NULL DEFAULT '',
			session_id      VARCHAR(255) NOT NULL,
			terminated_by   VARCHAR(255) NOT NULL,
			reason          TEXT NOT NULL DEFAULT '',
			terminated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (org_id, session_id)
		);
		CREATE INDEX IF NOT EXISTS idx_session_terminations_session
			ON session_terminations (session_id);
		CREATE INDEX IF NOT EXISTS idx_session_terminations_org
			ON session_terminations (org_id, terminated_at DESC);
	`
	_, err := r.db.ExecContext(ctx, query)
	return err
}

// TerminateSession inserts a termination record for the given session.
// orgID scopes the termination to the correct tenant.
func (r *Repository) TerminateSession(ctx context.Context, orgID, sessionID, terminatedBy, reason string) error {
	query := `
		INSERT INTO session_terminations (org_id, session_id, terminated_by, reason)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (org_id, session_id) DO UPDATE SET
			terminated_by = EXCLUDED.terminated_by,
			reason = EXCLUDED.reason,
			terminated_at = NOW()`

	_, err := r.db.ExecContext(ctx, query, orgID, sessionID, terminatedBy, reason)
	if err != nil {
		return fmt.Errorf("postgres: terminate session: %w", err)
	}
	return nil
}

// GetTerminatedSessions returns terminated session IDs for a specific org.
// If orgID is empty or "*", returns sessions across all orgs (super_admin).
func (r *Repository) GetTerminatedSessions(ctx context.Context, orgID string) ([]string, error) {
	var query string
	var args []interface{}

	if orgID == "" || orgID == "*" {
		query = `SELECT session_id FROM session_terminations ORDER BY terminated_at DESC LIMIT 1000`
	} else {
		query = `SELECT session_id FROM session_terminations WHERE org_id = $1 ORDER BY terminated_at DESC LIMIT 1000`
		args = append(args, orgID)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: get terminated sessions: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("postgres: scan terminated session: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// IsSessionTerminated checks if a specific session has been terminated.
// orgID scopes the check to the correct tenant.
func (r *Repository) IsSessionTerminated(ctx context.Context, orgID, sessionID string) (bool, error) {
	var exists bool
	var query string
	var args []interface{}

	if orgID == "" || orgID == "*" {
		query = `SELECT EXISTS(SELECT 1 FROM session_terminations WHERE session_id = $1)`
		args = append(args, sessionID)
	} else {
		query = `SELECT EXISTS(SELECT 1 FROM session_terminations WHERE org_id = $1 AND session_id = $2)`
		args = append(args, orgID, sessionID)
	}

	err := r.db.QueryRowContext(ctx, query, args...).Scan(&exists)
	return exists, err
}

// ---------------------------------------------------------------------------
// Review Decisions
// ---------------------------------------------------------------------------

// EnsureReviewDecisionsTable creates the review_decisions table if it doesn't exist.
//
// Multi-tenancy: Primary key is (org_id, session_id) — prevents cross-org
// collision when different orgs have overlapping session IDs.
func (r *Repository) EnsureReviewDecisionsTable(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS review_decisions (
			org_id          VARCHAR(64) NOT NULL,
			session_id      VARCHAR(255) NOT NULL,
			reviewer_id     UUID NOT NULL,
			reviewer_name   VARCHAR(255) NOT NULL,
			decision        VARCHAR(32) NOT NULL CHECK (decision IN ('confirmed', 'dismissed', 'escalated')),
			notes           TEXT NOT NULL DEFAULT '',
			evidence_ids    TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
			integrity_score DOUBLE PRECISION NOT NULL DEFAULT 0,
			reviewed_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (org_id, session_id)
		);
		CREATE INDEX IF NOT EXISTS idx_review_decisions_org
			ON review_decisions (org_id, reviewed_at DESC);
		CREATE INDEX IF NOT EXISTS idx_review_decisions_decision
			ON review_decisions (decision, reviewed_at DESC);
	`
	_, err := r.db.ExecContext(ctx, query)
	return err
}

// CreateReviewDecision inserts or updates a review decision for a session.
func (r *Repository) CreateReviewDecision(ctx context.Context, review *entity.ReviewDecision) error {
	query := `
		INSERT INTO review_decisions (org_id, session_id, reviewer_id, reviewer_name, decision, notes, evidence_ids, integrity_score)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (org_id, session_id) DO UPDATE SET
			reviewer_id = EXCLUDED.reviewer_id,
			reviewer_name = EXCLUDED.reviewer_name,
			decision = EXCLUDED.decision,
			notes = EXCLUDED.notes,
			evidence_ids = EXCLUDED.evidence_ids,
			integrity_score = EXCLUDED.integrity_score,
			reviewed_at = NOW()`

	evidenceIDs := "{}"
	if len(review.EvidenceIDs) > 0 {
		evidenceIDs = "{" + strings.Join(review.EvidenceIDs, ",") + "}"
	}

	_, err := r.db.ExecContext(ctx, query,
		review.OrgID,
		review.SessionID,
		review.ReviewerID,
		review.ReviewerName,
		review.Decision,
		review.Notes,
		evidenceIDs,
		review.IntegrityScore,
	)
	if err != nil {
		return fmt.Errorf("postgres: create review decision: %w", err)
	}
	return nil
}

// GetReviewDecision retrieves the review decision for a session.
func (r *Repository) GetReviewDecision(ctx context.Context, sessionID string) (*entity.ReviewDecision, error) {
	query := `
		SELECT session_id, reviewer_id, reviewer_name, org_id, decision, notes, evidence_ids, integrity_score, reviewed_at
		FROM review_decisions
		WHERE session_id = $1`

	var review entity.ReviewDecision
	var evidenceIDs string
	err := r.db.QueryRowContext(ctx, query, sessionID).Scan(
		&review.SessionID,
		&review.ReviewerID,
		&review.ReviewerName,
		&review.OrgID,
		&review.Decision,
		&review.Notes,
		&evidenceIDs,
		&review.IntegrityScore,
		&review.ReviewedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: get review decision: %w", err)
	}

	// Parse PostgreSQL array format: {id1,id2,id3}
	review.EvidenceIDs = parsePGArray(evidenceIDs)

	return &review, nil
}

// GetReviewStats returns aggregate statistics about review decisions.
func (r *Repository) GetReviewStats(ctx context.Context, orgID string) (confirmed, dismissed, escalated int, err error) {
	query := `
		SELECT
			COALESCE(SUM(CASE WHEN decision = 'confirmed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN decision = 'dismissed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN decision = 'escalated' THEN 1 ELSE 0 END), 0)
		FROM review_decisions`

	args := []interface{}{}
	if orgID != "*" {
		query += ` WHERE org_id = $1`
		args = append(args, orgID)
	}

	err = r.db.QueryRowContext(ctx, query, args...).Scan(&confirmed, &dismissed, &escalated)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("postgres: get review stats: %w", err)
	}

	return confirmed, dismissed, escalated, nil
}

// ListReviewedSessionIDs returns a map of session_id → decision for all reviewed sessions.
// Used by the archive handler to determine session status (reviewed vs pending).
func (r *Repository) ListReviewedSessionIDs(ctx context.Context, orgID string) (map[string]string, error) {
	query := `SELECT session_id, decision FROM review_decisions`
	args := []interface{}{}
	if orgID != "*" {
		query += ` WHERE org_id = $1`
		args = append(args, orgID)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list reviewed session ids: %w", err)
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var sessionID, decision string
		if err := rows.Scan(&sessionID, &decision); err != nil {
			return nil, fmt.Errorf("postgres: scan reviewed session: %w", err)
		}
		result[sessionID] = decision
	}
	return result, rows.Err()
}

// parsePGArray parses a PostgreSQL text array literal "{a,b,c}" into a Go slice.
func parsePGArray(s string) []string {
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	if s == "" {
		return []string{}
	}
	return strings.Split(s, ",")
}

// ==========================================================================
// Exam Proctoring Settings
// ==========================================================================

// EnsureExamProctoringSettingsTable creates the exam_proctoring_settings table
// if it doesn't exist. Called during startup to auto-migrate.
func (r *Repository) EnsureExamProctoringSettingsTable(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS exam_proctoring_settings (
			org_id     VARCHAR(64)  NOT NULL,
			exam_id    VARCHAR(255) NOT NULL,
			settings   JSONB        NOT NULL DEFAULT '{}',
			updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
			updated_by VARCHAR(255) NOT NULL DEFAULT '',
			PRIMARY KEY (org_id, exam_id)
		);
		CREATE INDEX IF NOT EXISTS idx_exam_proctoring_settings_org
			ON exam_proctoring_settings (org_id);
	`
	_, err := r.db.ExecContext(ctx, query)
	return err
}

// SaveExamProctoringSettings upserts the full proctoring configuration for an exam.
func (r *Repository) SaveExamProctoringSettings(ctx context.Context, settings *entity.ExamProctoringSettings) error {
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("postgres: marshal proctoring settings: %w", err)
	}

	query := `
		INSERT INTO exam_proctoring_settings (org_id, exam_id, settings, updated_at, updated_by)
		VALUES ($1, $2, $3, NOW(), $4)
		ON CONFLICT (org_id, exam_id) DO UPDATE SET
			settings   = EXCLUDED.settings,
			updated_at = NOW(),
			updated_by = EXCLUDED.updated_by`

	_, err = r.db.ExecContext(ctx, query,
		settings.OrgID,
		settings.ExamID,
		settingsJSON,
		settings.UpdatedBy,
	)
	if err != nil {
		return fmt.Errorf("postgres: save proctoring settings: %w", err)
	}
	return nil
}

// GetExamProctoringSettings retrieves the proctoring settings for a specific exam.
// Returns nil if no custom settings exist (caller should use defaults).
func (r *Repository) GetExamProctoringSettings(ctx context.Context, orgID, examID string) (*entity.ExamProctoringSettings, error) {
	query := `SELECT settings FROM exam_proctoring_settings WHERE org_id = $1 AND exam_id = $2`

	var settingsJSON []byte
	err := r.db.QueryRowContext(ctx, query, orgID, examID).Scan(&settingsJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: get proctoring settings: %w", err)
	}

	var settings entity.ExamProctoringSettings
	if err := json.Unmarshal(settingsJSON, &settings); err != nil {
		return nil, fmt.Errorf("postgres: unmarshal proctoring settings: %w", err)
	}

	// Ensure identity fields are populated from the key columns.
	settings.OrgID = orgID
	settings.ExamID = examID
	return &settings, nil
}

// ListExamProctoringSettingsByOrg returns all exam proctoring settings for an org.
func (r *Repository) ListExamProctoringSettingsByOrg(ctx context.Context, orgID string) ([]*entity.ExamProctoringSettings, error) {
	query := `SELECT org_id, exam_id, settings FROM exam_proctoring_settings WHERE org_id = $1 ORDER BY exam_id`
	rows, err := r.db.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list proctoring settings: %w", err)
	}
	defer rows.Close()

	var result []*entity.ExamProctoringSettings
	for rows.Next() {
		var oid, eid string
		var settingsJSON []byte
		if err := rows.Scan(&oid, &eid, &settingsJSON); err != nil {
			return nil, fmt.Errorf("postgres: scan proctoring settings: %w", err)
		}
		var s entity.ExamProctoringSettings
		if err := json.Unmarshal(settingsJSON, &s); err != nil {
			return nil, fmt.Errorf("postgres: unmarshal proctoring settings row: %w", err)
		}
		s.OrgID = oid
		s.ExamID = eid
		result = append(result, &s)
	}
	return result, rows.Err()
}

// DeleteExamProctoringSettings removes the custom proctoring settings for an exam.
func (r *Repository) DeleteExamProctoringSettings(ctx context.Context, orgID, examID string) error {
	query := `DELETE FROM exam_proctoring_settings WHERE org_id = $1 AND exam_id = $2`
	_, err := r.db.ExecContext(ctx, query, orgID, examID)
	if err != nil {
		return fmt.Errorf("postgres: delete proctoring settings: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Org Lifecycle — Data Cleanup (GDPR Right to Erasure)
// ---------------------------------------------------------------------------

// ArchiveOrgSessions marks all sessions for an org as "archived" by inserting
// termination records with reason "org_archived". Returns the number of sessions
// that were newly marked.
//
// This is a soft-delete operation — data is preserved but sessions are flagged
// as terminated. Used as a preliminary step before PurgeOrgData.
func (r *Repository) ArchiveOrgSessions(ctx context.Context, orgID string) (int64, error) {
	query := `
		INSERT INTO session_terminations (org_id, session_id, terminated_by, reason)
		SELECT DISTINCT $1, session_id, 'system', 'org_archived'
		FROM session_terminations
		WHERE org_id = $1
		ON CONFLICT (org_id, session_id) DO NOTHING`

	result, err := r.db.ExecContext(ctx, query, orgID)
	if err != nil {
		return 0, fmt.Errorf("postgres: archive org sessions: %w", err)
	}

	affected, _ := result.RowsAffected()
	return affected, nil
}

// PurgeOrgData permanently removes all data for the given org from PostgreSQL.
// This is the GDPR "right to erasure" implementation. It deletes from:
//   - session_terminations (all termination records for this org)
//   - review_decisions (all review decisions for this org)
//   - exam_proctoring_settings (all custom settings for this org)
//   - audit_log (all audit entries for this org)
//
// This operation is idempotent — safe to call multiple times for the same org.
func (r *Repository) PurgeOrgData(ctx context.Context, orgID string) error {
	if orgID == "" {
		return fmt.Errorf("postgres: purge org data: org_id is required")
	}

	// Use a transaction to ensure atomicity.
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("postgres: purge org data: begin tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && rbErr != sql.ErrTxDone {
			r.logger.Error("Failed to rollback transaction", zap.Error(rbErr))
		}
	}()

	// Delete from all org-scoped tables.
	tables := []struct {
		name  string
		query string
	}{
		{"session_terminations", "DELETE FROM session_terminations WHERE org_id = $1"},
		{"review_decisions", "DELETE FROM review_decisions WHERE org_id = $1"},
		{"exam_proctoring_settings", "DELETE FROM exam_proctoring_settings WHERE org_id = $1"},
		{"audit_log", "DELETE FROM audit_log WHERE org_id = $1"},
	}

	for _, t := range tables {
		if _, err := tx.ExecContext(ctx, t.query, orgID); err != nil {
			// Table may not exist — log and continue.
			// This handles graceful degradation when some tables haven't been migrated.
			continue
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("postgres: purge org data: commit: %w", err)
	}

	return nil
}

// ==========================================================================
// External Session CRUD
// ==========================================================================

func (r *Repository) CreateExternalSession(ctx context.Context, s *entity.ExternalSession) error {
	query := `
		INSERT INTO external_sessions (
			session_id, org_id, exam_id, student_id,
			student_name, exam_name, callback_url, metadata,
			idempotency_key, session_token, token_expires_at, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at, updated_at`

	var metadata interface{}
	if len(s.Metadata) > 0 {
		metadata = s.Metadata
	}
	var idempotencyKey interface{}
	if strings.TrimSpace(s.IdempotencyKey) != "" {
		idempotencyKey = strings.TrimSpace(s.IdempotencyKey)
	}
	return r.db.QueryRowContext(ctx, query,
		s.SessionID, s.OrgID, s.ExamID, s.StudentID,
		s.StudentName, s.ExamName, s.CallbackURL, metadata,
		idempotencyKey, s.SessionToken, s.TokenExpiresAt, s.Status,
	).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)
}

func (r *Repository) GetExternalSessionByIdempotencyKey(ctx context.Context, orgID, key string) (*entity.ExternalSession, error) {
	query := `
		SELECT id, session_id, org_id, exam_id, student_id,
			student_name, exam_name, callback_url, metadata, COALESCE(idempotency_key, ''),
			session_token, token_expires_at, status,
			verdict, verdict_details, integrity_score, violation_count,
			started_at, completed_at, created_at, updated_at
		FROM external_sessions
		WHERE org_id = $1 AND idempotency_key = $2 AND deleted_at IS NULL`

	s := &entity.ExternalSession{}
	err := r.db.QueryRowContext(ctx, query, orgID, key).Scan(
		&s.ID, &s.SessionID, &s.OrgID, &s.ExamID, &s.StudentID,
		&s.StudentName, &s.ExamName, &s.CallbackURL, &s.Metadata, &s.IdempotencyKey,
		&s.SessionToken, &s.TokenExpiresAt, &s.Status,
		&s.Verdict, &s.VerdictDetails, &s.IntegrityScore, &s.ViolationCount,
		&s.StartedAt, &s.CompletedAt, &s.CreatedAt, &s.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: get external session by idempotency key: %w", err)
	}
	return s, nil
}

func (r *Repository) GetExternalSessionByID(ctx context.Context, sessionID string) (*entity.ExternalSession, error) {
	query := `
		SELECT id, session_id, org_id, exam_id, student_id,
			student_name, exam_name, callback_url, metadata, COALESCE(idempotency_key, ''),
			token_expires_at, status,
			verdict, verdict_details, integrity_score, violation_count,
			started_at, completed_at, created_at, updated_at
		FROM external_sessions
		WHERE session_id = $1`

	s := &entity.ExternalSession{}
	err := r.db.QueryRowContext(ctx, query, sessionID).Scan(
		&s.ID, &s.SessionID, &s.OrgID, &s.ExamID, &s.StudentID,
		&s.StudentName, &s.ExamName, &s.CallbackURL, &s.Metadata, &s.IdempotencyKey,
		&s.TokenExpiresAt, &s.Status,
		&s.Verdict, &s.VerdictDetails, &s.IntegrityScore, &s.ViolationCount,
		&s.StartedAt, &s.CompletedAt, &s.CreatedAt, &s.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: get external session: %w", err)
	}
	return s, nil
}

func (r *Repository) UpdateExternalSessionStatus(ctx context.Context, sessionID, status string) error {
	query := `UPDATE external_sessions SET status = $2, updated_at = NOW() WHERE session_id = $1`
	if status == "active" {
		query = `UPDATE external_sessions SET status = $2, started_at = NOW(), updated_at = NOW() WHERE session_id = $1`
	}
	_, err := r.db.ExecContext(ctx, query, sessionID, status)
	return err
}

func (r *Repository) TouchExternalSession(ctx context.Context, sessionID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE external_sessions SET updated_at = NOW() WHERE session_id = $1 AND status NOT IN ('completed','cancelled','expired')`,
		sessionID,
	)
	return err
}

func (r *Repository) ActivateExternalSession(ctx context.Context, sessionID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE external_sessions SET status = 'active', started_at = COALESCE(started_at, NOW()), updated_at = NOW() WHERE session_id = $1 AND status = 'created'`,
		sessionID,
	)
	return err
}

func (r *Repository) ListExternalSessionsByResult(ctx context.Context, examID, studentID string) ([]*entity.ExternalSession, error) {
	query := `
		SELECT id, session_id, org_id, exam_id, student_id, student_name, exam_name,
		       status, verdict, integrity_score, violation_count,
		       started_at, completed_at, created_at, updated_at
		FROM external_sessions
		WHERE exam_id = $1 AND student_id = $2 AND deleted_at IS NULL
		ORDER BY created_at DESC`
	rows, err := r.db.QueryContext(ctx, query, examID, studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sessions []*entity.ExternalSession
	for rows.Next() {
		s := &entity.ExternalSession{}
		if err := rows.Scan(
			&s.ID, &s.SessionID, &s.OrgID, &s.ExamID, &s.StudentID,
			&s.StudentName, &s.ExamName, &s.Status, &s.Verdict,
			&s.IntegrityScore, &s.ViolationCount,
			&s.StartedAt, &s.CompletedAt, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

func (r *Repository) CompleteExternalSession(ctx context.Context, sessionID, verdict string, details []byte, score float64, violations int) error {
	query := `
		UPDATE external_sessions
		SET status = 'completed', verdict = $2, verdict_details = $3,
			integrity_score = $4, violation_count = $5,
			completed_at = NOW(), updated_at = NOW()
		WHERE session_id = $1`
	var detailsVal interface{}
	if len(details) > 0 {
		detailsVal = details
	}
	_, err := r.db.ExecContext(ctx, query, sessionID, verdict, detailsVal, score, violations)
	return err
}

// ==========================================================================
// Webhook Endpoint CRUD
// ==========================================================================

func (r *Repository) CreateWebhookEndpoint(ctx context.Context, ep *entity.WebhookEndpoint) error {
	query := `
		INSERT INTO webhook_endpoints (org_id, name, url, secret, events, is_active)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at`
	return r.db.QueryRowContext(ctx, query,
		ep.OrgID, ep.Name, ep.URL, ep.Secret, pqStringArray(ep.Events), ep.IsActive,
	).Scan(&ep.ID, &ep.CreatedAt, &ep.UpdatedAt)
}

func (r *Repository) GetWebhookEndpointByID(ctx context.Context, endpointID string) (*entity.WebhookEndpoint, error) {
	query := `
		SELECT id, org_id, name, url, events, is_active,
			last_delivery_at, last_failure_at, consecutive_failures,
			created_at, updated_at
		FROM webhook_endpoints
		WHERE id = $1 AND deleted_at IS NULL`

	ep := &entity.WebhookEndpoint{}
	var events string
	err := r.db.QueryRowContext(ctx, query, endpointID).Scan(
		&ep.ID, &ep.OrgID, &ep.Name, &ep.URL, &events, &ep.IsActive,
		&ep.LastDeliveryAt, &ep.LastFailureAt, &ep.ConsecutiveFailures,
		&ep.CreatedAt, &ep.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: get webhook endpoint: %w", err)
	}
	ep.Events = parsePgArray(events)
	return ep, nil
}

func (r *Repository) GetWebhookEndpointsByOrg(ctx context.Context, orgID string) ([]*entity.WebhookEndpoint, error) {
	query := `
		SELECT id, org_id, name, url, events, is_active,
			last_delivery_at, last_failure_at, consecutive_failures,
			created_at, updated_at
		FROM webhook_endpoints
		WHERE org_id = $1 AND deleted_at IS NULL AND is_active = true
		ORDER BY created_at DESC`

	rows, err := r.db.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list webhook endpoints: %w", err)
	}
	defer rows.Close()

	var endpoints []*entity.WebhookEndpoint
	for rows.Next() {
		ep := &entity.WebhookEndpoint{}
		var events string
		if err := rows.Scan(
			&ep.ID, &ep.OrgID, &ep.Name, &ep.URL, &events, &ep.IsActive,
			&ep.LastDeliveryAt, &ep.LastFailureAt, &ep.ConsecutiveFailures,
			&ep.CreatedAt, &ep.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan webhook endpoint: %w", err)
		}
		ep.Events = parsePgArray(events)
		endpoints = append(endpoints, ep)
	}
	return endpoints, rows.Err()
}

func (r *Repository) GetWebhookEndpointsByOrgAndEvent(ctx context.Context, orgID, eventType string) ([]*entity.WebhookEndpoint, error) {
	query := `
		SELECT id, org_id, name, url, secret, events, is_active,
			last_delivery_at, last_failure_at, consecutive_failures,
			created_at, updated_at
		FROM webhook_endpoints
		WHERE org_id = $1 AND deleted_at IS NULL AND is_active = true
			AND $2 = ANY(events)
		ORDER BY created_at DESC`

	rows, err := r.db.QueryContext(ctx, query, orgID, eventType)
	if err != nil {
		return nil, fmt.Errorf("postgres: list webhook endpoints by event: %w", err)
	}
	defer rows.Close()

	var endpoints []*entity.WebhookEndpoint
	for rows.Next() {
		ep := &entity.WebhookEndpoint{}
		var events string
		if err := rows.Scan(
			&ep.ID, &ep.OrgID, &ep.Name, &ep.URL, &ep.Secret, &events, &ep.IsActive,
			&ep.LastDeliveryAt, &ep.LastFailureAt, &ep.ConsecutiveFailures,
			&ep.CreatedAt, &ep.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan webhook endpoint: %w", err)
		}
		ep.Events = parsePgArray(events)
		endpoints = append(endpoints, ep)
	}
	return endpoints, rows.Err()
}

func (r *Repository) DeleteWebhookEndpoint(ctx context.Context, endpointID string) error {
	query := `UPDATE webhook_endpoints SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`
	_, err := r.db.ExecContext(ctx, query, endpointID)
	return err
}

// ==========================================================================
// Webhook Delivery CRUD
// ==========================================================================

func (r *Repository) CreateWebhookDelivery(ctx context.Context, d *entity.WebhookDelivery) error {
	query := `
		INSERT INTO webhook_deliveries (endpoint_id, org_id, event_type, payload, status, max_attempts, next_retry_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at`
	return r.db.QueryRowContext(ctx, query,
		d.EndpointID, d.OrgID, d.EventType, d.Payload, d.Status, d.MaxAttempts, d.NextRetryAt,
	).Scan(&d.ID, &d.CreatedAt)
}

func (r *Repository) GetPendingWebhookDeliveries(ctx context.Context, limit int) ([]*entity.WebhookDelivery, error) {
	query := `
		SELECT d.id, d.endpoint_id, d.org_id, d.event_type, d.payload,
			d.status, d.attempt, d.max_attempts, d.next_retry_at, d.created_at,
			e.url, e.secret
		FROM webhook_deliveries d
		JOIN webhook_endpoints e ON e.id = d.endpoint_id
		WHERE d.status = 'pending' AND d.next_retry_at <= NOW()
		ORDER BY d.next_retry_at ASC
		LIMIT $1
		FOR UPDATE OF d SKIP LOCKED`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("postgres: get pending deliveries: %w", err)
	}
	defer rows.Close()

	var deliveries []*entity.WebhookDelivery
	for rows.Next() {
		d := &entity.WebhookDelivery{}
		var endpointURL, endpointSecret string
		if err := rows.Scan(
			&d.ID, &d.EndpointID, &d.OrgID, &d.EventType, &d.Payload,
			&d.Status, &d.Attempt, &d.MaxAttempts, &d.NextRetryAt, &d.CreatedAt,
			&endpointURL, &endpointSecret,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan delivery: %w", err)
		}
		// Store URL and secret in ResponseBody and ErrorMessage temporarily
		// (these fields are unused for pending deliveries).
		d.ResponseBody = endpointURL
		d.ErrorMessage = endpointSecret
		deliveries = append(deliveries, d)
	}
	return deliveries, rows.Err()
}

func (r *Repository) MarkWebhookDelivered(ctx context.Context, deliveryID int64, httpStatus int, responseBody string) error {
	query := `
		UPDATE webhook_deliveries
		SET status = 'delivered', http_status = $2, response_body = $3,
			attempt = attempt + 1, delivered_at = NOW()
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, deliveryID, httpStatus, responseBody)
	return err
}

func (r *Repository) MarkWebhookFailed(ctx context.Context, deliveryID int64, httpStatus int, errorMsg string, nextRetryAt *time.Time) error {
	if nextRetryAt != nil {
		query := `
			UPDATE webhook_deliveries
			SET http_status = $2, error_message = $3,
				attempt = attempt + 1, next_retry_at = $4
			WHERE id = $1`
		_, err := r.db.ExecContext(ctx, query, deliveryID, httpStatus, errorMsg, *nextRetryAt)
		return err
	}
	// Final failure — no more retries.
	query := `
		UPDATE webhook_deliveries
		SET status = 'failed', http_status = $2, error_message = $3,
			attempt = attempt + 1
		WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, deliveryID, httpStatus, errorMsg)
	return err
}

// ==========================================================================
// Student Enrollments (ArcFace reference embeddings)
// ==========================================================================

// StudentEnrollment holds a student's reference face embedding used for
// identity verification during AI deep scan.
type StudentEnrollment struct {
	ID           string
	StudentID    string
	OrgID        string
	Embedding    []float32
	PhotoURL     string
	EnrolledBy   string
	ModelVersion string
	EnrolledAt   time.Time
}

// SaveEnrollment inserts or replaces a student's reference embedding.
// Upserts on (student_id, org_id) so re-enrollment replaces the old record.
func (r *Repository) SaveEnrollment(ctx context.Context, e *StudentEnrollment) error {
	embJSON, err := json.Marshal(e.Embedding)
	if err != nil {
		return fmt.Errorf("marshal embedding: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO student_enrollments
			(student_id, org_id, embedding, photo_url, enrolled_by, model_version, enrolled_at)
		VALUES ($1, $2, $3::jsonb, $4, $5, $6, NOW())
		ON CONFLICT (student_id, org_id)
		DO UPDATE SET
			embedding     = EXCLUDED.embedding,
			photo_url     = EXCLUDED.photo_url,
			enrolled_by   = EXCLUDED.enrolled_by,
			model_version = EXCLUDED.model_version,
			enrolled_at   = NOW()`,
		e.StudentID, e.OrgID, string(embJSON), e.PhotoURL, e.EnrolledBy, e.ModelVersion,
	)
	return err
}

// GetEnrollment returns the stored embedding for a student, or nil if not enrolled.
func (r *Repository) GetEnrollment(ctx context.Context, studentID, orgID string) (*StudentEnrollment, error) {
	var e StudentEnrollment
	var embJSON string
	err := r.db.QueryRowContext(ctx, `
		SELECT id, student_id, org_id, embedding::text, photo_url,
		       enrolled_by, model_version, enrolled_at
		FROM student_enrollments
		WHERE student_id = $1 AND org_id = $2`,
		studentID, orgID,
	).Scan(&e.ID, &e.StudentID, &e.OrgID, &embJSON, &e.PhotoURL,
		&e.EnrolledBy, &e.ModelVersion, &e.EnrolledAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(embJSON), &e.Embedding); err != nil {
		return nil, fmt.Errorf("unmarshal embedding: %w", err)
	}
	return &e, nil
}

// DeleteEnrollment removes a student's enrollment record.
func (r *Repository) DeleteEnrollment(ctx context.Context, studentID, orgID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM student_enrollments WHERE student_id = $1 AND org_id = $2`,
		studentID, orgID,
	)
	return err
}
