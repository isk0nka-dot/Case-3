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
		db.Close()
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

// ==========================================================================
// Organization Repository Implementation
// ==========================================================================

func (r *Repository) CreateOrg(ctx context.Context, org *entity.Organization) error {
	query := `
		INSERT INTO organizations (org_id, name, slug, org_type, contact_email, contact_phone,
			city, region, plan, max_sessions, max_events_rps, retention_days, is_active, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $14)
		RETURNING id, created_at, updated_at`

	return r.db.QueryRowContext(ctx, query,
		org.OrgID, org.Name, org.Slug, org.OrgType,
		org.ContactEmail, org.ContactPhone, org.City, org.Region,
		string(org.Plan), org.MaxSessions, org.MaxEventsRPS, org.RetentionDays,
		org.IsActive, org.CreatedBy,
	).Scan(&org.ID, &org.CreatedAt, &org.UpdatedAt)
}

func (r *Repository) GetOrgByOrgID(ctx context.Context, orgID string) (*entity.Organization, error) {
	query := `
		SELECT id, org_id, name, slug, org_type,
			COALESCE(contact_email, ''), COALESCE(contact_phone, ''),
			COALESCE(city, ''), COALESCE(region, ''),
			plan, max_sessions, max_events_rps, retention_days,
			is_active, created_at, updated_at, deleted_at, created_by, updated_by
		FROM organizations
		WHERE org_id = $1 AND deleted_at IS NULL`

	org := &entity.Organization{}
	var plan string
	err := r.db.QueryRowContext(ctx, query, orgID).Scan(
		&org.ID, &org.OrgID, &org.Name, &org.Slug, &org.OrgType,
		&org.ContactEmail, &org.ContactPhone, &org.City, &org.Region,
		&plan, &org.MaxSessions, &org.MaxEventsRPS, &org.RetentionDays,
		&org.IsActive, &org.CreatedAt, &org.UpdatedAt, &org.DeletedAt,
		&org.CreatedBy, &org.UpdatedBy,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: get org: %w", err)
	}
	org.Plan = entity.Plan(plan)
	return org, nil
}

func (r *Repository) GetOrgBySlug(ctx context.Context, slug string) (*entity.Organization, error) {
	query := `
		SELECT id, org_id, name, slug, org_type,
			COALESCE(contact_email, ''), COALESCE(contact_phone, ''),
			COALESCE(city, ''), COALESCE(region, ''),
			plan, max_sessions, max_events_rps, retention_days,
			is_active, created_at, updated_at, deleted_at, created_by, updated_by
		FROM organizations
		WHERE slug = $1 AND deleted_at IS NULL`

	org := &entity.Organization{}
	var plan string
	err := r.db.QueryRowContext(ctx, query, slug).Scan(
		&org.ID, &org.OrgID, &org.Name, &org.Slug, &org.OrgType,
		&org.ContactEmail, &org.ContactPhone, &org.City, &org.Region,
		&plan, &org.MaxSessions, &org.MaxEventsRPS, &org.RetentionDays,
		&org.IsActive, &org.CreatedAt, &org.UpdatedAt, &org.DeletedAt,
		&org.CreatedBy, &org.UpdatedBy,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: get org by slug: %w", err)
	}
	org.Plan = entity.Plan(plan)
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
		argIndex++
	}

	// Exclude the internal Super Admin organization from listings.
	conditions = append(conditions, "org_id != '*'")

	query := fmt.Sprintf(`
		SELECT id, org_id, name, slug, org_type,
			COALESCE(contact_email, ''), COALESCE(contact_phone, ''),
			COALESCE(city, ''), COALESCE(region, ''),
			plan, max_sessions, max_events_rps, retention_days,
			is_active, created_at, updated_at, created_by, updated_by
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
	defer rows.Close()

	var orgs []*entity.Organization
	for rows.Next() {
		org := &entity.Organization{}
		var plan string
		if err := rows.Scan(
			&org.ID, &org.OrgID, &org.Name, &org.Slug, &org.OrgType,
			&org.ContactEmail, &org.ContactPhone, &org.City, &org.Region,
			&plan, &org.MaxSessions, &org.MaxEventsRPS, &org.RetentionDays,
			&org.IsActive, &org.CreatedAt, &org.UpdatedAt,
			&org.CreatedBy, &org.UpdatedBy,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan org: %w", err)
		}
		org.Plan = entity.Plan(plan)
		orgs = append(orgs, org)
	}

	return orgs, rows.Err()
}

func (r *Repository) UpdateOrg(ctx context.Context, org *entity.Organization) error {
	query := `
		UPDATE organizations
		SET name = $2, slug = $3, org_type = $4, contact_email = $5, contact_phone = $6,
			city = $7, region = $8, plan = $9, max_sessions = $10, max_events_rps = $11,
			retention_days = $12, is_active = $13, updated_at = NOW(), updated_by = $14
		WHERE org_id = $1 AND deleted_at IS NULL`

	_, err := r.db.ExecContext(ctx, query,
		org.OrgID, org.Name, org.Slug, org.OrgType,
		org.ContactEmail, org.ContactPhone, org.City, org.Region,
		string(org.Plan), org.MaxSessions, org.MaxEventsRPS, org.RetentionDays,
		org.IsActive, org.UpdatedBy,
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
	defer rows.Close()

	var users []*entity.User
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
		argIndex++
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

	var users []*entity.User
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

	var keys []*entity.APIKey
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

	var keys []*entity.APIKey
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
		argIndex++
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

	var entries []*entity.AuditEntry
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
func (r *Repository) EnsureSessionTerminationsTable(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS session_terminations (
			id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			session_id      VARCHAR(255) NOT NULL UNIQUE,
			terminated_by   UUID NOT NULL,
			reason          TEXT NOT NULL DEFAULT '',
			terminated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_session_terminations_session
			ON session_terminations (session_id);
	`
	_, err := r.db.ExecContext(ctx, query)
	return err
}

// TerminateSession inserts a termination record for the given session.
func (r *Repository) TerminateSession(ctx context.Context, sessionID, terminatedBy, reason string) error {
	query := `
		INSERT INTO session_terminations (session_id, terminated_by, reason)
		VALUES ($1, $2, $3)
		ON CONFLICT (session_id) DO UPDATE SET
			terminated_by = EXCLUDED.terminated_by,
			reason = EXCLUDED.reason,
			terminated_at = NOW()`

	_, err := r.db.ExecContext(ctx, query, sessionID, terminatedBy, reason)
	if err != nil {
		return fmt.Errorf("postgres: terminate session: %w", err)
	}
	return nil
}

// GetTerminatedSessions returns all terminated session IDs.
func (r *Repository) GetTerminatedSessions(ctx context.Context) ([]string, error) {
	query := `SELECT session_id FROM session_terminations ORDER BY terminated_at DESC LIMIT 1000`
	rows, err := r.db.QueryContext(ctx, query)
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
func (r *Repository) IsSessionTerminated(ctx context.Context, sessionID string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM session_terminations WHERE session_id = $1)`
	err := r.db.QueryRowContext(ctx, query, sessionID).Scan(&exists)
	return exists, err
}

// ---------------------------------------------------------------------------
// Review Decisions
// ---------------------------------------------------------------------------

// EnsureReviewDecisionsTable creates the review_decisions table if it doesn't exist.
func (r *Repository) EnsureReviewDecisionsTable(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS review_decisions (
			session_id      VARCHAR(255) PRIMARY KEY,
			reviewer_id     UUID NOT NULL,
			reviewer_name   VARCHAR(255) NOT NULL,
			org_id          VARCHAR(64) NOT NULL,
			decision        VARCHAR(32) NOT NULL CHECK (decision IN ('confirmed', 'dismissed', 'escalated')),
			notes           TEXT NOT NULL DEFAULT '',
			evidence_ids    TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
			integrity_score DOUBLE PRECISION NOT NULL DEFAULT 0,
			reviewed_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
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
		INSERT INTO review_decisions (session_id, reviewer_id, reviewer_name, org_id, decision, notes, evidence_ids, integrity_score)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (session_id) DO UPDATE SET
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
		review.SessionID,
		review.ReviewerID,
		review.ReviewerName,
		review.OrgID,
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
