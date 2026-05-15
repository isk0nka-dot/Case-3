package livekit

import (
	"context"
	"net/http"

	"github.com/livekit/protocol/auth"
	livekitpb "github.com/livekit/protocol/livekit"
	"github.com/livekit/protocol/webhook"
	"go.uber.org/zap"
)

// EvidenceStorePort — интерфейс для обновления базы данных после завершения Egress
type EvidenceStorePort interface {
	UpdateRecordingEnded(ctx context.Context, egressID, status, fileURL, errorMessage string) error
}

// WebhookHandler обрабатывает входящие хуки от LiveKit
type WebhookHandler struct {
	logger       *zap.Logger
	apiKey       string
	apiSecret    string
	evidenceRepo EvidenceStorePort
}

func NewWebhookHandler(apiKey, apiSecret string, repo EvidenceStorePort, logger *zap.Logger) *WebhookHandler {
	return &WebhookHandler{
		logger:       logger.Named("livekit_webhook"),
		apiKey:       apiKey,
		apiSecret:    apiSecret,
		evidenceRepo: repo,
	}
}

// ServeHTTP implements http.Handler.
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Создаем AuthProvider для валидации цифровой подписи хука
	authProvider := auth.NewSimpleKeyProvider(
		h.apiKey,
		h.apiSecret,
	)

	// Парсим и валидируем событие
	event, err := webhook.ReceiveWebhookEvent(r, authProvider)
	if err != nil {
		h.logger.Error("ошибка валидации webhook", zap.Error(err))
		http.Error(w, "invalid webhook signature", http.StatusUnauthorized)
		return
	}

	// Нам интересен только хук об окончании записи (Egress)
	if event.GetEvent() == "egress_ended" {
		egressInfo := event.GetEgressInfo()
		if egressInfo == nil {
			w.WriteHeader(http.StatusOK)
			return
		}

		h.logger.Info("получено событие egress_ended",
			zap.String("egress_id", egressInfo.EgressId),
			zap.String("room", egressInfo.RoomName),
			zap.String("status", egressInfo.Status.String()),
		)

		if err := handleEgressEnded(r.Context(), h.evidenceRepo, egressInfo); err != nil {
			h.logger.Error("ошибка обновления записи LiveKit Egress",
				zap.String("egress_id", egressInfo.EgressId),
				zap.Error(err),
			)
			http.Error(w, "failed to update recording", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

func handleEgressEnded(ctx context.Context, repo EvidenceStorePort, egressInfo *livekitpb.EgressInfo) error {
	if repo == nil || egressInfo == nil {
		return nil
	}

	fileURL := ""
	if fileRes := egressInfo.GetFile(); fileRes != nil {
		fileURL = fileRes.GetFilename()
	}

	return repo.UpdateRecordingEnded(
		ctx,
		egressInfo.GetEgressId(),
		egressInfo.GetStatus().String(),
		fileURL,
		egressInfo.GetError(),
	)
}
