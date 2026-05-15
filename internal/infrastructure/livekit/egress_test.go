package livekit

import (
	"testing"

	livekitpb "github.com/livekit/protocol/livekit"
)

func TestBuildTrackCompositeRequestUsesStableRecordingPathAndEncoding(t *testing.T) {
	cfg := EgressConfig{
		S3Endpoint:  "minio:9000",
		S3AccessKey: "access",
		S3SecretKey: "secret",
		S3Bucket:    "argus-evidence",
	}

	req := buildTrackCompositeRequest(cfg, "argus-session-session-1", "video-track", "audio-track", "student-1", "session-1")

	if req.RoomName != "argus-session-session-1" {
		t.Fatalf("RoomName = %q, want argus-session-session-1", req.RoomName)
	}
	if req.VideoTrackId != "video-track" {
		t.Fatalf("VideoTrackId = %q, want video-track", req.VideoTrackId)
	}
	if req.AudioTrackId != "audio-track" {
		t.Fatalf("AudioTrackId = %q, want audio-track", req.AudioTrackId)
	}

	advanced := req.GetAdvanced()
	if advanced == nil {
		t.Fatal("advanced encoding options are nil")
	}
	if advanced.Width != 1280 || advanced.Height != 720 || advanced.Framerate != 15 {
		t.Fatalf("encoding = %dx%d@%d, want 1280x720@15", advanced.Width, advanced.Height, advanced.Framerate)
	}
	if advanced.VideoCodec != livekitpb.VideoCodec_H264_MAIN {
		t.Fatalf("VideoCodec = %s, want H264_MAIN", advanced.VideoCodec)
	}

	file := req.GetFile()
	if file == nil {
		t.Fatal("file output is nil")
	}
	if file.Filepath != "content/recordings/argus-session-session-1/session-1-student-1.mp4" {
		t.Fatalf("Filepath = %q", file.Filepath)
	}
	s3 := file.GetS3()
	if s3 == nil {
		t.Fatal("s3 output is nil")
	}
	if s3.Endpoint != cfg.S3Endpoint || s3.AccessKey != cfg.S3AccessKey || s3.Secret != cfg.S3SecretKey || s3.Bucket != cfg.S3Bucket {
		t.Fatalf("s3 config not copied into request: %+v", s3)
	}
}
