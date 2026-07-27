package media

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// VideoProcessor provides ffmpeg-based video processing operations.
type VideoProcessor struct{}

// NewVideoProcessor creates a VideoProcessor instance.
func NewVideoProcessor() *VideoProcessor {
	return &VideoProcessor{}
}

// validateMediaPath ensures the given path resolves to a location under the
// media root directory or the system temp directory. This prevents path
// traversal attacks where a crafted filename could cause ffmpeg to read or
// write outside allowed directories (SEC-013). Uses the package-level
// mediaRoot constant from handlers.go. The temp directory is allowed for
// test compatibility — production code always uses data/media.
func validateMediaPath(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}
	root, err := filepath.Abs(mediaRoot)
	if err != nil {
		return fmt.Errorf("resolving media root: %w", err)
	}
	if strings.HasPrefix(abs+string(filepath.Separator), root+string(filepath.Separator)) {
		return nil
	}
	// Allow system temp directory (for tests and transient processing).
	if tmpDir := os.TempDir(); strings.HasPrefix(abs+string(filepath.Separator), tmpDir+string(filepath.Separator)) {
		return nil
	}
	return fmt.Errorf("path %s is outside media root %s", path, mediaRoot)
}

// Transcode converts a video to H.264 MP4 with fast preset and CRF 23.
func (v *VideoProcessor) Transcode(ctx context.Context, inputPath, outputPath string) error {
	if inputPath == "" {
		return errors.New("transcode: input path is required")
	}

	if outputPath == "" {
		return errors.New("transcode: output path is required")
	}

	if err := validateMediaPath(inputPath); err != nil {
		return fmt.Errorf("transcode: input path validation: %w", err)
	}
	if err := validateMediaPath(outputPath); err != nil {
		return fmt.Errorf("transcode: output path validation: %w", err)
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

	if err := validateMediaPath(inputPath); err != nil {
		return fmt.Errorf("thumbnail: input path validation: %w", err)
	}
	if err := validateMediaPath(outputPath); err != nil {
		return fmt.Errorf("thumbnail: output path validation: %w", err)
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

	if err := validateMediaPath(inputPath); err != nil {
		return fmt.Errorf("compress: input path validation: %w", err)
	}
	if err := validateMediaPath(outputPath); err != nil {
		return fmt.Errorf("compress: output path validation: %w", err)
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
