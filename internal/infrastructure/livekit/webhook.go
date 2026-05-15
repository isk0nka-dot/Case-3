package livekit

import (
	"context"
	"net/http"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/webhook"
	"go.uber.org/zap"
)

// EvidenceStorePort — интерфейс для обновления базы данных после завершения Egress
type EvidenceStorePort interface {
	UpdateEvidenceURL(ctx context.Context, sessionID, userID, url string) error
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

		// Достаем результаты: нам нужен путь к файлу в S3
		if fileRes := egressInfo.GetFile(); fileRes != nil {
			filepath := fileRes.GetFilename()
			
			// ВАЖНО: Мы сохраняли файл как content/recordings/{sessionID}/{userID}.mp4
			// Если база данных подключена, мы вызываем обновление URL
			if h.evidenceRepo != nil {
				// В реальной системе нужно распарсить sessionID и userID из filepath
				// или передавать их через egressInfo.RoomName, но сейчас для примера:
				h.logger.Info("файл успешно сохранен, обновляем БД", zap.String("filepath", filepath))
				
				// Заглушка:
				// h.evidenceRepo.UpdateEvidenceURL(r.Context(), sessionID, userID, filepath)
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}
