package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/argus-ai/event-collector/internal/infrastructure/postgres"
	"github.com/argus-ai/event-collector/internal/infrastructure/worker"
	"github.com/hibiken/asynq"
	"go.uber.org/zap"
)

// EnrollmentHandler handles student biometric enrollment endpoints.
//
//	POST /api/v1/external/students/{studentId}/enroll   — trigger enrollment from photo URL
//	GET  /api/v1/external/students/{studentId}/enrollment — check enrollment status
//	DELETE /api/v1/external/students/{studentId}/enrollment — remove enrollment
type EnrollmentHandler struct {
	repo        *postgres.Repository
	asynqClient *asynq.Client
	logger      *zap.Logger
	jwtSigningKey []byte
}

// NewEnrollmentHandler creates the REST enrollment handler.
func NewEnrollmentHandler(
	repo *postgres.Repository,
	asynqClient *asynq.Client,
	logger *zap.Logger,
	jwtSigningKey []byte,
) *EnrollmentHandler {
	return &EnrollmentHandler{
		repo:          repo,
		asynqClient:   asynqClient,
		logger:        logger.Named("enrollment_api"),
		jwtSigningKey: jwtSigningKey,
	}
}

// RegisterRoutes attaches enrollment endpoints to the mux.
func (h *EnrollmentHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/external/students/{studentId}/enroll",
		h.requireAPIKeyEnroll(h.handleEnroll))
	mux.HandleFunc("GET /api/v1/external/students/{studentId}/enrollment",
		h.requireAPIKeyEnroll(h.handleGetEnrollment))
	mux.HandleFunc("DELETE /api/v1/external/students/{studentId}/enrollment",
		h.requireAPIKeyEnroll(h.handleDeleteEnrollment))
}

// handleEnroll accepts a photo URL and enqueues a background enrollment task.
//
//	POST /api/v1/external/students/{studentId}/enroll
//	Body: { "photoUrl": "https://...", "enrolledBy": "lms" }
func (h *EnrollmentHandler) handleEnroll(w http.ResponseWriter, r *http.Request) {
	apiCtx := getAPIKeyFromContext(r.Context())
	if apiCtx == nil {
		h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	studentID := r.PathValue("studentId")
	if strings.TrimSpace(studentID) == "" {
		h.jsonError(w, "studentId is required", http.StatusBadRequest)
		return
	}

	var body struct {
		PhotoURL   string `json:"photoUrl"`
		EnrolledBy string `json:"enrolledBy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.PhotoURL) == "" {
		h.jsonError(w, "photoUrl is required", http.StatusBadRequest)
		return
	}

	enrolledBy := body.EnrolledBy
	if enrolledBy == "" {
		enrolledBy = "api"
	}

	task, err := worker.NewEnrollStudentTask(worker.EnrollStudentPayload{
		StudentID:  studentID,
		OrgID:      apiCtx.OrgID,
		PhotoURL:   body.PhotoURL,
		EnrolledBy: enrolledBy,
	})
	if err != nil {
		h.logger.Error("failed to build enrollment task", zap.Error(err))
		h.jsonError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if _, err := h.asynqClient.Enqueue(task); err != nil {
		h.logger.Error("failed to enqueue enrollment task",
			zap.String("student_id", studentID),
			zap.Error(err),
		)
		h.jsonError(w, "Failed to enqueue enrollment", http.StatusInternalServerError)
		return
	}

	h.logger.Info("enrollment task enqueued",
		zap.String("student_id", studentID),
		zap.String("org_id", apiCtx.OrgID),
	)

	h.jsonResponse(w, map[string]string{
		"status":    "queued",
		"studentId": studentID,
		"message":   "Enrollment queued. Check status via GET /enrollment after a few seconds.",
	}, http.StatusAccepted)
}

// handleGetEnrollment returns enrollment status for a student.
//
//	GET /api/v1/external/students/{studentId}/enrollment
func (h *EnrollmentHandler) handleGetEnrollment(w http.ResponseWriter, r *http.Request) {
	apiCtx := getAPIKeyFromContext(r.Context())
	if apiCtx == nil {
		h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	studentID := r.PathValue("studentId")
	if strings.TrimSpace(studentID) == "" {
		h.jsonError(w, "studentId is required", http.StatusBadRequest)
		return
	}

	enrollment, err := h.repo.GetEnrollment(r.Context(), studentID, apiCtx.OrgID)
	if err != nil {
		h.logger.Error("failed to look up enrollment", zap.Error(err))
		h.jsonError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if enrollment == nil {
		h.jsonResponse(w, map[string]interface{}{
			"studentId": studentID,
			"enrolled":  false,
		}, http.StatusOK)
		return
	}

	h.jsonResponse(w, map[string]interface{}{
		"studentId":    enrollment.StudentID,
		"enrolled":     true,
		"enrolledBy":   enrollment.EnrolledBy,
		"enrolledAt":   enrollment.EnrolledAt,
		"modelVersion": enrollment.ModelVersion,
		"photoUrl":     enrollment.PhotoURL,
		"embeddingDim": len(enrollment.Embedding),
	}, http.StatusOK)
}

// handleDeleteEnrollment removes a student's enrollment.
//
//	DELETE /api/v1/external/students/{studentId}/enrollment
func (h *EnrollmentHandler) handleDeleteEnrollment(w http.ResponseWriter, r *http.Request) {
	apiCtx := getAPIKeyFromContext(r.Context())
	if apiCtx == nil {
		h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	studentID := r.PathValue("studentId")
	if strings.TrimSpace(studentID) == "" {
		h.jsonError(w, "studentId is required", http.StatusBadRequest)
		return
	}

	if err := h.repo.DeleteEnrollment(r.Context(), studentID, apiCtx.OrgID); err != nil {
		h.logger.Error("failed to delete enrollment", zap.Error(err))
		h.jsonError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, map[string]string{
		"status":    "deleted",
		"studentId": studentID,
	}, http.StatusOK)
}

// requireAPIKeyEnroll reuses the existing requireAPIKey middleware from external_handler.
// It validates X-CLIENT-ID + X-API-KEY and injects APIKeyContext into the request.
func (h *EnrollmentHandler) requireAPIKeyEnroll(next http.HandlerFunc) http.HandlerFunc {
	// Delegate to the existing package-level helper by creating a minimal
	// ExternalHandler with the same repo and key; avoids duplicating auth logic.
	eh := &ExternalHandler{repo: h.repo, jwtSigningKey: h.jwtSigningKey}
	return eh.requireAPIKey("sessions:write", next)
}

func (h *EnrollmentHandler) jsonResponse(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data) //nolint:errcheck
}

func (h *EnrollmentHandler) jsonError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message}) //nolint:errcheck
}
