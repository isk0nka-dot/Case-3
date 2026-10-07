// =============================================================================
// Argus AI — Chunked Evidence Upload REST API Handler
// =============================================================================
//
// Provides HTTP endpoints for receiving chunked binary evidence uploads from
// browser clients operating in degraded (Tier B/C) network conditions.
//
// The browser splits large binary evidence (JPEG snapshots, audio clips) into
// 256KB chunks and uploads them sequentially. Each chunk includes a per-chunk
// SHA-256 hash for integrity verification. When the last chunk arrives, the
// assembler concatenates all chunks, verifies the total SHA-256, and uploads
// the result to MinIO.
//
// Endpoints:
//   POST /api/v1/ingest/chunk                      — Upload a single chunk
//   GET  /api/v1/ingest/chunk/{fragmentId}/status   — Check upload progress
//
// Authentication: Same JWT middleware as all other admin endpoints.
// Content-Type: multipart/form-data (for chunk uploads).
// =============================================================================
package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/argus-ai/event-collector/internal/application/port"
	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/infrastructure/postgres"
)

// ChunkHandler serves the chunked upload REST API for binary evidence ingestion.
type ChunkHandler struct {
	assembler port.ChunkAssembler
	pgRepo    *postgres.Repository
	logger    *zap.Logger

	jwtSigningKey []byte
	repo          adminRepo // for requireAuth user lookup
}

// NewChunkHandler creates a new chunk upload API handler.
func NewChunkHandler(
	assembler port.ChunkAssembler,
	pgRepo *postgres.Repository,
	logger *zap.Logger,
	jwtSigningKey []byte,
) *ChunkHandler {
	return &ChunkHandler{
		assembler:     assembler,
		pgRepo:        pgRepo,
		logger:        logger.Named("chunk_api"),
		jwtSigningKey: jwtSigningKey,
		repo:          pgRepo,
	}
}

// RegisterRoutes registers chunk upload API endpoints on the given mux.
// These routes use the /api/v1/ingest/ prefix to avoid conflicts with
// existing /api/v1/evidence/ routes (which handle evidence retrieval).
func (h *ChunkHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/ingest/chunk", h.requireAuth(h.handleUploadChunk))
	mux.HandleFunc("GET /api/v1/ingest/chunk/{fragmentId}/status", h.requireAuth(h.handleChunkStatus))
}

// ==========================================================================
// Upload Chunk Handler
// ==========================================================================

// maxChunkUploadSize is the max request body size for chunk uploads (1MB).
// This is generous — individual chunks are typically 256KB.
const maxChunkUploadSize = 1 * 1024 * 1024

// handleUploadChunk processes a single chunk upload via multipart/form-data.
//
// Request fields (form data):
//   - session_id:    proctoring session identifier
//   - fragment_id:   unique identifier for the assembled evidence file
//   - chunk_index:   zero-based index of this chunk
//   - total_chunks:  total number of chunks for this fragment
//   - sha256_chunk:  hex-encoded SHA-256 hash of this chunk's data
//   - sha256_total:  hex-encoded SHA-256 hash of the complete file
//   - content_type:  MIME type of the evidence (e.g., "image/jpeg")
//   - org_id:        organization identifier
//   - exam_id:       exam identifier
//   - student_id:    student identifier
//   - data:          the chunk binary data (file upload)
//
// Response: JSON with chunk status and assembly result (if complete).
func (h *ChunkHandler) handleUploadChunk(w http.ResponseWriter, r *http.Request) {
	// Limit request body size.
	r.Body = http.MaxBytesReader(w, r.Body, maxChunkUploadSize)

	// Parse multipart form.
	if err := r.ParseMultipartForm(maxChunkUploadSize); err != nil {
		h.chunkJSONError(w, fmt.Sprintf("Failed to parse multipart form: %v", err), http.StatusBadRequest)
		return
	}

	// Extract form fields.
	sessionID := r.FormValue("session_id")
	fragmentID := r.FormValue("fragment_id")
	chunkIndexStr := r.FormValue("chunk_index")
	totalChunksStr := r.FormValue("total_chunks")
	sha256Chunk := r.FormValue("sha256_chunk")
	sha256Total := r.FormValue("sha256_total")
	contentType := r.FormValue("content_type")
	orgID := r.FormValue("org_id")
	examID := r.FormValue("exam_id")
	studentID := r.FormValue("student_id")

	// Validate required fields.
	if sessionID == "" || fragmentID == "" || chunkIndexStr == "" || totalChunksStr == "" {
		h.chunkJSONError(w, "Missing required fields: session_id, fragment_id, chunk_index, total_chunks", http.StatusBadRequest)
		return
	}

	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil {
		h.chunkJSONError(w, fmt.Sprintf("Invalid chunk_index: %v", err), http.StatusBadRequest)
		return
	}

	totalChunks, err := strconv.Atoi(totalChunksStr)
	if err != nil {
		h.chunkJSONError(w, fmt.Sprintf("Invalid total_chunks: %v", err), http.StatusBadRequest)
		return
	}

	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Get the chunk file data.
	file, _, err := r.FormFile("data")
	if err != nil {
		h.chunkJSONError(w, fmt.Sprintf("Missing 'data' file field: %v", err), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Build chunk metadata.
	meta := port.ChunkMeta{
		SessionID:   sessionID,
		FragmentID:  fragmentID,
		ChunkIndex:  chunkIndex,
		TotalChunks: totalChunks,
		SHA256Chunk: sha256Chunk,
		SHA256Total: sha256Total,
		ContentType: contentType,
		OrgID:       orgID,
		ExamID:      examID,
		StudentID:   studentID,
	}

	// Receive the chunk.
	complete, err := h.assembler.ReceiveChunk(r.Context(), meta, file)
	if err != nil {
		h.logger.Warn("chunk receive failed",
			zap.String("fragment_id", fragmentID),
			zap.Int("chunk_index", chunkIndex),
			zap.Error(err),
		)
		h.chunkJSONError(w, fmt.Sprintf("Chunk receive failed: %v", err), http.StatusBadRequest)
		return
	}

	// If all chunks received, automatically assemble.
	if complete {
		h.logger.Info("all chunks received, assembling",
			zap.String("fragment_id", fragmentID),
			zap.Int("total_chunks", totalChunks),
		)

		uri, sha256Hash, sizeBytes, err := h.assembler.Assemble(r.Context(), fragmentID)
		if err != nil {
			h.logger.Error("fragment assembly failed",
				zap.String("fragment_id", fragmentID),
				zap.Error(err),
			)
			h.chunkJSONError(w, fmt.Sprintf("Assembly failed: %v", err), http.StatusInternalServerError)
			return
		}

		// Log audit trail.
		h.logChunkAccess(r, "chunk_upload_complete", fragmentID)

		h.chunkJSONResponse(w, chunkUploadResponse{
			FragmentID:     fragmentID,
			ChunkIndex:     chunkIndex,
			TotalChunks:    totalChunks,
			ReceivedChunks: totalChunks,
			Complete:       true,
			URI:            uri,
			SHA256:         sha256Hash,
			SizeBytes:      sizeBytes,
		}, http.StatusOK)
		return
	}

	// Partial upload — return status.
	status, _ := h.assembler.GetStatus(r.Context(), fragmentID)
	received := 0
	if status != nil {
		received = status.ReceivedChunks
	}

	h.chunkJSONResponse(w, chunkUploadResponse{
		FragmentID:     fragmentID,
		ChunkIndex:     chunkIndex,
		TotalChunks:    totalChunks,
		ReceivedChunks: received,
		Complete:       false,
	}, http.StatusAccepted)
}

// ==========================================================================
// Chunk Status Handler
// ==========================================================================

// handleChunkStatus returns the current upload status for a fragment.
func (h *ChunkHandler) handleChunkStatus(w http.ResponseWriter, r *http.Request) {
	fragmentID := r.PathValue("fragmentId")
	if fragmentID == "" {
		h.chunkJSONError(w, "Missing fragmentId path parameter", http.StatusBadRequest)
		return
	}

	status, err := h.assembler.GetStatus(r.Context(), fragmentID)
	if err != nil {
		h.chunkJSONError(w, fmt.Sprintf("Fragment not found: %v", err), http.StatusNotFound)
		return
	}

	h.chunkJSONResponse(w, status, http.StatusOK)
}

// ==========================================================================
// Response Types
// ==========================================================================

// chunkUploadResponse is the JSON response for chunk upload operations.
type chunkUploadResponse struct {
	FragmentID     string `json:"fragmentId"`
	ChunkIndex     int    `json:"chunkIndex"`
	TotalChunks    int    `json:"totalChunks"`
	ReceivedChunks int    `json:"receivedChunks"`
	Complete       bool   `json:"complete"`
	URI            string `json:"uri,omitempty"`
	SHA256         string `json:"sha256,omitempty"`
	SizeBytes      int64  `json:"sizeBytes,omitempty"`
}

// ==========================================================================
// Auth & Helpers (same pattern as EvidenceHandler)
// ==========================================================================

func (h *ChunkHandler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := ""
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		}

		if token == "" {
			h.chunkJSONError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		user, err := h.verifyToken(r.Context(), token)
		if err != nil {
			h.chunkJSONError(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

func (h *ChunkHandler) verifyToken(ctx context.Context, tokenStr string) (*entity.User, error) {
	parts := splitToken(tokenStr)
	if parts == nil {
		return nil, errInvalidToken
	}

	signingInput := parts[0] + "." + parts[1]
	expectedSig := hmacSHA256([]byte(signingInput), h.jwtSigningKey)
	actualSig, err := base64URLDecode(parts[2])
	if err != nil {
		return nil, errInvalidToken
	}
	if !hmacEqual(expectedSig, actualSig) {
		return nil, errInvalidToken
	}

	payloadBytes, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, errInvalidToken
	}

	var claims struct {
		Sub string `json:"sub"`
		Exp int64  `json:"exp"`
	}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, errInvalidToken
	}

	if time.Now().Unix() > claims.Exp {
		return nil, errInvalidToken
	}

	user, err := h.repo.GetUserByID(ctx, claims.Sub)
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (h *ChunkHandler) logChunkAccess(r *http.Request, action string, resourceID string) {
	caller := getUserFromContext(r.Context())
	if caller == nil {
		return
	}

	h.logger.Info("chunk access",
		zap.String("action", action),
		zap.String("resource_id", resourceID),
		zap.String("user_id", caller.ID),
		zap.String("user_role", string(caller.Role)),
		zap.String("org_id", caller.OrgID),
		zap.String("ip", r.RemoteAddr),
	)

	// Write to audit log if PostgreSQL is available.
	if h.pgRepo != nil {
		_ = h.pgRepo.CreateAuditEntry(r.Context(), &entity.AuditEntry{
			UserID:       caller.ID,
			UserPhone:    caller.Phone,
			UserRole:     string(caller.Role),
			OrgID:        caller.OrgID,
			Action:       action,
			ResourceType: "chunk_evidence",
			ResourceID:   resourceID,
			IPAddress:    r.RemoteAddr,
			UserAgent:    r.Header.Get("User-Agent"),
		})
	}
}

func (h *ChunkHandler) chunkJSONResponse(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *ChunkHandler) chunkJSONError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
