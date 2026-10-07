package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hibiken/asynq"
	inferencepb "github.com/argus-ai/event-collector/api/proto/v1/inferencepb"
	"github.com/argus-ai/event-collector/internal/infrastructure/postgres"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// EnrollStudentHandler processes TypeEnrollStudent asynq tasks.
//
// Flow:
//  1. Fetch reference photo bytes from PhotoURL (HTTP GET, 5 MB cap)
//  2. Call AnalyzeFrame on the inference gateway
//  3. Extract the first face embedding from the response
//  4. Upsert into student_enrollments via the postgres repo
type EnrollStudentHandler struct {
	inferenceAddr string
	pgRepo        *postgres.Repository
	httpClient    *http.Client
	logger        *zap.Logger
}

// NewEnrollStudentHandler creates the asynq task handler for student enrollment.
func NewEnrollStudentHandler(
	inferenceAddr string,
	pgRepo *postgres.Repository,
	logger *zap.Logger,
) *EnrollStudentHandler {
	return &EnrollStudentHandler{
		inferenceAddr: inferenceAddr,
		pgRepo:        pgRepo,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger.Named("asynq_enroll"),
	}
}

// ProcessTask implements asynq.Handler.
func (h *EnrollStudentHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var payload EnrollStudentPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal EnrollStudentPayload: %w", err)
	}

	h.logger.Info("processing enrollment",
		zap.String("student_id", payload.StudentID),
		zap.String("org_id", payload.OrgID),
		zap.String("photo_url", payload.PhotoURL),
	)

	// ── Step 1: Fetch the reference photo ────────────────────────────────────
	frameData, contentType, err := h.fetchPhoto(ctx, payload.PhotoURL)
	if err != nil {
		return fmt.Errorf("fetch reference photo for %s: %w", payload.StudentID, err)
	}

	// ── Step 2: Connect to inference gateway ─────────────────────────────────
	conn, err := grpc.NewClient(
		h.inferenceAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("connect to inference gateway: %w", err)
	}
	defer conn.Close()

	client := inferencepb.NewInferenceServiceClient(conn)

	// ── Step 3: Run face detection + embedding extraction ────────────────────
	resp, err := client.AnalyzeFrame(ctx, &inferencepb.AnalyzeFrameRequest{
		SessionId:   "enrollment",
		StudentId:   payload.StudentID,
		OrgId:       payload.OrgID,
		ExamId:      "enrollment",
		FrameData:   frameData,
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("inference AnalyzeFrame for %s: %w", payload.StudentID, err)
	}

	if len(resp.GetFaces()) == 0 {
		return fmt.Errorf("no face detected in reference photo for student %s (url=%s)",
			payload.StudentID, payload.PhotoURL)
	}

	embedding := resp.GetFaces()[0].GetEmbedding()
	if len(embedding) == 0 {
		return fmt.Errorf("inference returned empty embedding for student %s", payload.StudentID)
	}

	// ── Step 4: Persist enrollment ───────────────────────────────────────────
	enrolledBy := payload.EnrolledBy
	if enrolledBy == "" {
		enrolledBy = "lms"
	}

	if err := h.pgRepo.SaveEnrollment(ctx, &postgres.StudentEnrollment{
		StudentID:    payload.StudentID,
		OrgID:        payload.OrgID,
		Embedding:    embedding,
		PhotoURL:     payload.PhotoURL,
		EnrolledBy:   enrolledBy,
		ModelVersion: "arcface_r50_w600k",
	}); err != nil {
		return fmt.Errorf("save enrollment for %s: %w", payload.StudentID, err)
	}

	h.logger.Info("enrollment complete",
		zap.String("student_id", payload.StudentID),
		zap.String("org_id", payload.OrgID),
		zap.Int("embedding_dim", len(embedding)),
	)

	return nil
}

// fetchPhoto downloads the image at url and returns (bytes, content-type, error).
// Enforces a 5 MB cap to prevent OOM from adversarial URLs.
func (h *EnrollStudentHandler) fetchPhoto(ctx context.Context, url string) ([]byte, string, error) {
	const maxBytes = 5 * 1024 * 1024 // 5 MB

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("photo URL returned HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > maxBytes {
		return nil, "", fmt.Errorf("photo exceeds 5 MB limit")
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "image/jpeg"
	}

	return data, ct, nil
}
