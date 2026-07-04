package media

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// VideoProcessor provides ffmpeg-based video processing operations.
type VideoProcessor struct{}

// NewVideoProcessor creates a VideoProcessor instance.
func NewVideoProcessor() *VideoProcessor {
	return &VideoProcessor{}
}

// Transcode converts a video to H.264 MP4 with fast preset and CRF 23.
func (v *VideoProcessor) Transcode(ctx context.Context, inputPath, outputPath string) error {
	if inputPath == "" {
		return errors.New("transcode: input path is required")
	}

	if outputPath == "" {
		return errors.New("transcode: output path is required")
	}

	cmd := exec.CommandContext(
		ctx, "ffmpeg",
		"-y",            // overwrite output
		"-i", inputPath, // input
		"-c:v", "libx264", // H.264 codec
		"-preset", "fast", // fast encoding
		"-crf", "23", // quality level
		"-c:a", "aac", // audio codec
		"-b:a", "128k", // audio bitrate
		outputPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("transcode: ffmpeg failed: %w — %s", err, sanitizeFFmpegOutput(string(output)))
	}

	return nil
}

// GenerateThumbnail extracts a single frame at the given timestamp.
func (v *VideoProcessor) GenerateThumbnail(ctx context.Context, inputPath, outputPath, timestamp string) error {
	if inputPath == "" {
		return errors.New("thumbnail: input path is required")
	}

	if outputPath == "" {
		return errors.New("thumbnail: output path is required")
	}

	if timestamp == "" {
		timestamp = "00:00:01"
	}

	cmd := exec.CommandContext(
		ctx, "ffmpeg",
		"-y",            // overwrite output
		"-i", inputPath, // input
		"-ss", timestamp, // seek to timestamp
		"-vframes", "1", // extract single frame
		"-q:v", "2", // quality
		outputPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("thumbnail: ffmpeg failed: %w — %s", err, sanitizeFFmpegOutput(string(output)))
	}

	return nil
}

// Compress reduces video bitrate with slow preset and CRF 28 for smaller files.
func (v *VideoProcessor) Compress(ctx context.Context, inputPath, outputPath string) error {
	if inputPath == "" {
		return errors.New("compress: input path is required")
	}

	if outputPath == "" {
		return errors.New("compress: output path is required")
	}

	cmd := exec.CommandContext(
		ctx, "ffmpeg",
		"-y",            // overwrite output
		"-i", inputPath, // input
		"-c:v", "libx264", // H.264 codec
		"-preset", "slow", // slow encoding, better compression
		"-crf", "28", // lower quality, smaller file
		"-c:a", "aac", // audio codec
		"-b:a", "96k", // lower audio bitrate
		outputPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("compress: ffmpeg failed: %w — %s", err, sanitizeFFmpegOutput(string(output)))
	}

	return nil
}

// sanitizeFFmpegOutput truncates verbose ffmpeg output to the last few lines
// for use in error messages while keeping the error actionable.
func sanitizeFFmpegOutput(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	// Return last 5 lines max to avoid massive error messages.
	if len(lines) > 5 {
		lines = lines[len(lines)-5:]
	}

	return strings.Join(lines, "\n")
}
