package worker

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"
)

type evidenceFrame struct {
	Data         []byte
	ContentType  string
	TimestampSec float64
}

type FrameExtractor interface {
	ExtractFrames(ctx context.Context, video io.Reader, contentType string) ([]evidenceFrame, error)
}

type FrameExtractionConfig struct {
	FFmpegPath     string
	IntervalSec    int
	MaxVideoDurSec int
	MaxFrames      int
	Timeout        time.Duration
}

type FFmpegFrameExtractor struct {
	cfg FrameExtractionConfig
}

func NewFFmpegFrameExtractor(cfg FrameExtractionConfig) *FFmpegFrameExtractor {
	return &FFmpegFrameExtractor{cfg: normalizeFrameExtractionConfig(cfg)}
}

func normalizeFrameExtractionConfig(cfg FrameExtractionConfig) FrameExtractionConfig {
	if cfg.FFmpegPath == "" {
		cfg.FFmpegPath = "ffmpeg"
	}
	if cfg.IntervalSec <= 0 {
		cfg.IntervalSec = 5
	}
	if cfg.MaxVideoDurSec <= 0 {
		cfg.MaxVideoDurSec = 300
	}
	if cfg.MaxFrames <= 0 {
		cfg.MaxFrames = cfg.MaxVideoDurSec / cfg.IntervalSec
		if cfg.MaxFrames < 1 {
			cfg.MaxFrames = 1
		}
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = time.Duration(cfg.MaxVideoDurSec+30) * time.Second
	}
	return cfg
}

func (e *FFmpegFrameExtractor) ExtractFrames(ctx context.Context, video io.Reader, _ string) ([]evidenceFrame, error) {
	dir, err := os.MkdirTemp("", "argus-frames-*")
	if err != nil {
		return nil, fmt.Errorf("create frame temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	inputPath := filepath.Join(dir, "input.video")
	input, err := os.Create(inputPath)
	if err != nil {
		return nil, fmt.Errorf("create temp video: %w", err)
	}
	if _, err := io.Copy(input, video); err != nil {
		input.Close()
		return nil, fmt.Errorf("copy video to temp file: %w", err)
	}
	if err := input.Close(); err != nil {
		return nil, fmt.Errorf("close temp video: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, e.cfg.Timeout)
	defer cancel()

	args := e.ffmpegArgs(inputPath, dir)
	cmd := exec.CommandContext(ctx, e.cfg.FFmpegPath, args...)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("ffmpeg timed out after %s", e.cfg.Timeout)
	}
	if err != nil {
		return nil, fmt.Errorf("ffmpeg extract frames: %w: %s", err, string(output))
	}

	paths, err := filepath.Glob(filepath.Join(dir, "frame-*.jpg"))
	if err != nil {
		return nil, fmt.Errorf("list extracted frames: %w", err)
	}
	sort.Strings(paths)

	frames := make([]evidenceFrame, 0, len(paths))
	for i, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read extracted frame %s: %w", path, err)
		}
		frames = append(frames, evidenceFrame{
			Data:         data,
			ContentType:  "image/jpeg",
			TimestampSec: float64(i * e.cfg.IntervalSec),
		})
	}
	return frames, nil
}

func (e *FFmpegFrameExtractor) ffmpegArgs(inputPath, outputDir string) []string {
	outputPattern := filepath.Join(outputDir, "frame-%06d.jpg")
	return []string{
		"-hide_banner",
		"-loglevel", "error",
		"-nostdin",
		"-i", inputPath,
		"-vf", fmt.Sprintf("fps=1/%d", e.cfg.IntervalSec),
		"-frames:v", fmt.Sprintf("%d", e.cfg.MaxFrames),
		"-q:v", "3",
		outputPattern,
	}
}
