package livekit

import (
	"context"
	"fmt"

	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"go.uber.org/zap"
)

// EgressConfig содержит настройки для подключения к LiveKit и S3 (Minio)
type EgressConfig struct {
	Host        string
	APIKey      string
	APISecret   string
	S3Endpoint  string
	S3AccessKey string
	S3SecretKey string
	S3Bucket    string
}

// EgressService управляет записью видеопотоков через LiveKit Egress
type EgressService struct {
	client *lksdk.EgressClient
	cfg    EgressConfig
	logger *zap.Logger
}

// NewEgressService создает новый сервис Egress
func NewEgressService(cfg EgressConfig, logger *zap.Logger) *EgressService {
	client := lksdk.NewEgressClient(cfg.Host, cfg.APIKey, cfg.APISecret)
	return &EgressService{
		client: client,
		cfg:    cfg,
		logger: logger.Named("egress_service"),
	}
}

// StartRecording запускает запись видео трека студента.
// Использует TrackCompositeEgress для применения жестких лимитов (720p, 15fps)
// с минимальной нагрузкой на CPU (так как нет рендеринга всего UI браузера).
func (s *EgressService) StartRecording(ctx context.Context, roomName, videoTrackID, audioTrackID, userID, sessionID string) (string, error) {
	req := buildTrackCompositeRequest(s.cfg, roomName, videoTrackID, audioTrackID, userID, sessionID)
	filepath := ""
	if fileOutputs := req.GetFileOutputs(); len(fileOutputs) > 0 && fileOutputs[0] != nil {
		filepath = fileOutputs[0].GetFilepath()
	}

	s.logger.Info("запуск записи LiveKit Egress",
		zap.String("room", roomName),
		zap.String("session_id", sessionID),
		zap.String("filepath", filepath),
	)

	info, err := s.client.StartTrackCompositeEgress(ctx, req)
	if err != nil {
		s.logger.Error("ошибка запуска Egress", zap.Error(err))
		return "", fmt.Errorf("failed to start track composite egress: %w", err)
	}

	return info.EgressId, nil
}

func buildTrackCompositeRequest(cfg EgressConfig, roomName, videoTrackID, audioTrackID, userID, sessionID string) *livekit.TrackCompositeEgressRequest {
	filepath := fmt.Sprintf("content/recordings/%s/%s-%s.mp4", roomName, sessionID, userID)

	encodingOptions := &livekit.EncodingOptions{
		Width:        1280,
		Height:       720,
		Depth:        24,
		Framerate:    15,
		VideoCodec:   livekit.VideoCodec_H264_MAIN,
		VideoBitrate: 1500,
	}

	fileOutput := &livekit.EncodedFileOutput{
		FileType: livekit.EncodedFileType_MP4,
		Filepath: filepath,
		Output: &livekit.EncodedFileOutput_S3{
			S3: &livekit.S3Upload{
				AccessKey: cfg.S3AccessKey,
				Secret:    cfg.S3SecretKey,
				Endpoint:  cfg.S3Endpoint,
				Bucket:    cfg.S3Bucket,
			},
		},
	}

	return &livekit.TrackCompositeEgressRequest{
		RoomName:     roomName,
		VideoTrackId: videoTrackID,
		AudioTrackId: audioTrackID,
		Options: &livekit.TrackCompositeEgressRequest_Advanced{
			Advanced: encodingOptions,
		},
		FileOutputs: []*livekit.EncodedFileOutput{fileOutput},
	}
}

// StopRecording останавливает процесс записи
func (s *EgressService) StopRecording(ctx context.Context, egressID string) error {
	s.logger.Info("остановка записи LiveKit Egress", zap.String("egress_id", egressID))

	_, err := s.client.StopEgress(ctx, &livekit.StopEgressRequest{
		EgressId: egressID,
	})

	if err != nil {
		s.logger.Error("ошибка остановки Egress", zap.Error(err))
		return fmt.Errorf("failed to stop egress: %w", err)
	}

	return nil
}
