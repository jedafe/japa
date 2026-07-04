package media

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNewVideoProcessor(t *testing.T) {
	t.Parallel()

	vp := NewVideoProcessor()
	if vp == nil {
		t.Fatal("NewVideoProcessor returned nil")
	}
}

func TestTranscodeRequiresInputPath(t *testing.T) {
	t.Parallel()

	vp := NewVideoProcessor()

	err := vp.Transcode(t.Context(), "", "/tmp/out.mp4")
	if err == nil {
		t.Fatal("expected error for empty input path")
	}

	if err.Error() != "transcode: input path is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTranscodeRequiresOutputPath(t *testing.T) {
	t.Parallel()

	vp := NewVideoProcessor()

	err := vp.Transcode(t.Context(), "/tmp/in.mp4", "")
	if err == nil {
		t.Fatal("expected error for empty output path")
	}

	if err.Error() != "transcode: output path is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGenerateThumbnailRequiresInputPath(t *testing.T) {
	t.Parallel()

	vp := NewVideoProcessor()

	err := vp.GenerateThumbnail(t.Context(), "", "/tmp/out.jpg", "00:00:01")
	if err == nil {
		t.Fatal("expected error for empty input path")
	}

	if err.Error() != "thumbnail: input path is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGenerateThumbnailRequiresOutputPath(t *testing.T) {
	t.Parallel()

	vp := NewVideoProcessor()

	err := vp.GenerateThumbnail(t.Context(), "/tmp/in.mp4", "", "00:00:01")
	if err == nil {
		t.Fatal("expected error for empty output path")
	}

	if err.Error() != "thumbnail: output path is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCompressRequiresInputPath(t *testing.T) {
	t.Parallel()

	vp := NewVideoProcessor()

	err := vp.Compress(t.Context(), "", "/tmp/out.mp4")
	if err == nil {
		t.Fatal("expected error for empty input path")
	}

	if err.Error() != "compress: input path is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCompressRequiresOutputPath(t *testing.T) {
	t.Parallel()

	vp := NewVideoProcessor()

	err := vp.Compress(t.Context(), "/tmp/in.mp4", "")
	if err == nil {
		t.Fatal("expected error for empty output path")
	}

	if err.Error() != "compress: output path is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSanitizeFFmpegOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty",
			input: "",
			want:  "",
		},
		{
			name:  "single line",
			input: "error",
			want:  "error",
		},
		{
			name:  "short output",
			input: "line1\nline2\nline3",
			want:  "line1\nline2\nline3",
		},
		{
			name:  "long output truncated to last 5",
			input: "line1\nline2\nline3\nline4\nline5\nline6\nline7",
			want:  "line3\nline4\nline5\nline6\nline7",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := sanitizeFFmpegOutput(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeFFmpegOutput() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestTranscodeIntegration runs against real ffmpeg if available.
func TestTranscodeIntegration(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed, skipping integration test")
	}

	vp := NewVideoProcessor()
	dir := t.TempDir()

	// Create a minimal valid MP4 input (1-second black video).
	inputPath := filepath.Join(dir, "input.mp4")

	cmd := exec.Command(
		"ffmpeg",
		"-y", "-f", "lavfi", "-i", "color=c=black:s=320x240:d=1",
		"-c:v", "libx264", "-preset", "ultrafast",
		inputPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create test video: %v\n%s", err, out)
	}

	outputPath := filepath.Join(dir, "output.mp4")
	if err := vp.Transcode(t.Context(), inputPath, outputPath); err != nil {
		t.Fatalf("Transcode failed: %v", err)
	}

	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatal("output file was not created")
	}
}

func TestThumbnailIntegration(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed, skipping integration test")
	}

	vp := NewVideoProcessor()
	dir := t.TempDir()

	inputPath := filepath.Join(dir, "input.mp4")

	cmd := exec.Command(
		"ffmpeg",
		"-y", "-f", "lavfi", "-i", "color=c=black:s=320x240:d=2",
		"-c:v", "libx264", "-preset", "ultrafast",
		inputPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create test video: %v\n%s", err, out)
	}

	outputPath := filepath.Join(dir, "thumb.jpg")
	if err := vp.GenerateThumbnail(t.Context(), inputPath, outputPath, "00:00:01"); err != nil {
		t.Fatalf("GenerateThumbnail failed: %v", err)
	}

	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatal("thumbnail file was not created")
	}
}

func TestCompressIntegration(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed, skipping integration test")
	}

	vp := NewVideoProcessor()
	dir := t.TempDir()

	inputPath := filepath.Join(dir, "input.mp4")

	cmd := exec.Command(
		"ffmpeg",
		"-y", "-f", "lavfi", "-i", "color=c=black:s=320x240:d=1",
		"-c:v", "libx264", "-preset", "ultrafast",
		inputPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create test video: %v\n%s", err, out)
	}

	outputPath := filepath.Join(dir, "compressed.mp4")
	if err := vp.Compress(t.Context(), inputPath, outputPath); err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatal("compressed file was not created")
	}
}
