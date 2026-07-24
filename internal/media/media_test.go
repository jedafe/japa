package media

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/httpx"
)

// mustUUID scans a UUID string into a pgtype.UUID, failing the test on error.
func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		t.Fatalf("scan uuid %q: %v", s, err)
	}

	return id
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

func TestMIMEPrefixConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		got, want string
	}{
		{MIMEPrefixImage, "image"},
		{MIMEPrefixVideo, "video"},
		{MIMEPrefixAudio, "audio"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("MIME prefix mismatch: got %q, want %q", tt.got, tt.want)
		}
	}
}

func TestStoragePathConstants(t *testing.T) {
	t.Parallel()

	if StoragePathTemp != "temp" {
		t.Errorf("StoragePathTemp = %q, want %q", StoragePathTemp, "temp")
	}

	if StoragePathMedia != "media" {
		t.Errorf("StoragePathMedia = %q, want %q", StoragePathMedia, "media")
	}
}

func TestDefaultBucket(t *testing.T) {
	t.Parallel()

	if DefaultBucket != "media" {
		t.Errorf("DefaultBucket = %q, want %q", DefaultBucket, "media")
	}
}

func TestPageSizeConstants(t *testing.T) {
	t.Parallel()

	if DefaultPageSize != 20 {
		t.Errorf("DefaultPageSize = %d, want 20", DefaultPageSize)
	}

	if MaxPageSize != 100 {
		t.Errorf("MaxPageSize = %d, want 100", MaxPageSize)
	}
}

func TestMaxFileSizeBytes(t *testing.T) {
	t.Parallel()

	if MaxFileSizeBytes != 50*1024*1024 {
		t.Errorf("MaxFileSizeBytes = %d, want %d", MaxFileSizeBytes, 50*1024*1024)
	}
}

// ---------------------------------------------------------------------------
// ImageProxyOpts
// ---------------------------------------------------------------------------

func TestImageProxyOptsDefaults(t *testing.T) {
	t.Parallel()

	opts := ImageProxyOpts{}
	if opts.ResizingType != "" {
		t.Errorf("default ResizingType = %q, want empty", opts.ResizingType)
	}

	if opts.Width != 0 {
		t.Errorf("default Width = %d, want 0", opts.Width)
	}

	if opts.Height != 0 {
		t.Errorf("default Height = %d, want 0", opts.Height)
	}

	if opts.Format != "" {
		t.Errorf("default Format = %q, want empty", opts.Format)
	}
}

// ---------------------------------------------------------------------------
// hexDecode
// ---------------------------------------------------------------------------

func TestHexDecode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"48656c6c6f", "Hello"},
		{"", ""},
		{"invalid", ""},
	}
	for _, tt := range tests {
		got := string(hexDecode(tt.input))
		if got != tt.want {
			t.Errorf("hexDecode(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// IsAllowedMIMEType
// ---------------------------------------------------------------------------

func TestIsAllowedMIMEType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mimeType string
		want     bool
	}{
		{"image/jpeg", true},
		{"image/png", true},
		{"image/webp", true},
		{"image/gif", true},
		{"image/svg+xml", true},
		{"video/mp4", true},
		{"video/webm", true},
		{"audio/mpeg", true},
		{"audio/ogg", true},
		{"audio/wav", true},
		{"application/pdf", true},
		{"application/zip", false},
		{"text/html", false},
		{"", false},
		{"IMAGE/JPEG", false}, // case-sensitive
	}
	for _, tt := range tests {
		got := IsAllowedMIMEType(tt.mimeType)
		if got != tt.want {
			t.Errorf("IsAllowedMIMEType(%q) = %v, want %v", tt.mimeType, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// NormalizeMIMEType
// ---------------------------------------------------------------------------

func TestNormalizeMIMEType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input, want string
	}{
		{"Image/JPEG", "image/jpeg"},
		{"  image/png  ", "image/png"},
		{"VIDEO/MP4", "video/mp4"},
		{"", ""},
	}
	for _, tt := range tests {
		got := NormalizeMIMEType(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeMIMEType(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// MediaURL (unit test — no real env vars)
// ---------------------------------------------------------------------------

func TestMediaURLReturnsEmptyWhenEnvMissing(t *testing.T) {
	t.Parallel()
	// Ensure env vars are not set.
	_ = os.Unsetenv("JC_IMGPROXY_KEY")
	_ = os.Unsetenv("JC_IMGPROXY_SALT")
	_ = os.Unsetenv("JC_IMGPROXY_BASE_URL")

	got := MediaURL("media", "test/photo.jpg", ImageProxyOpts{
		ResizingType: "fill",
		Width:        640,
		Height:       480,
		Format:       "webp",
	})
	if got != "" {
		t.Errorf("MediaURL with missing env vars = %q, want empty", got)
	}
}

func TestMediaURLWithEnvVars(t *testing.T) {
	// Cannot use t.Parallel() because t.Setenv() is used (Go 1.26 restriction)
	t.Setenv("JC_IMGPROXY_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("JC_IMGPROXY_SALT", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("JC_IMGPROXY_BASE_URL", "https://imgproxy.example.com")

	got := MediaURL("media", "test/photo.jpg", ImageProxyOpts{
		ResizingType: "fill",
		Width:        640,
		Height:       480,
		Format:       "webp",
	})
	if got == "" {
		t.Fatal("MediaURL returned empty string with env vars set")
	}

	if got[:len("https://imgproxy.example.com/")] != "https://imgproxy.example.com/" {
		t.Errorf("MediaURL prefix = %q, want https://imgproxy.example.com/", got[:len("https://imgproxy.example.com/")])
	}
}

// ---------------------------------------------------------------------------
// writeJSON / writeError (copied from follows for handler tests)
// ---------------------------------------------------------------------------

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	httpx.WriteJSON(rr, http.StatusCreated, map[string]string{"status": "ok"})

	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusCreated)
	}

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	if got["status"] != "ok" {
		t.Errorf(`body["status"] = %q, want "ok"`, got["status"])
	}
}

func TestWriteError(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	httpx.WriteError(rr, http.StatusBadRequest, "invalid_body", "Invalid JSON body")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	errMap, ok := got["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error envelope, got %v", got)
	}

	if errMap["code"] != "invalid_body" {
		t.Errorf("error.code = %v, want invalid_body", errMap["code"])
	}

	if errMap["message"] != "Invalid JSON body" {
		t.Errorf("error.message = %v, want 'Invalid JSON body'", errMap["message"])
	}
}

// ---------------------------------------------------------------------------
// parsePageSize
// ---------------------------------------------------------------------------

func TestParsePageSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		query string
		want  int32
	}{
		{"", DefaultPageSize},
		{"0", DefaultPageSize},
		{"-1", DefaultPageSize},
		{"abc", DefaultPageSize},
		{"10", 10},
		{"50", 50},
		{"100", 100},
		{"101", MaxPageSize},
		{"999", MaxPageSize},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "/media?page_size="+tt.query, nil)

		got := parsePageSize(req)
		if got != tt.want {
			t.Errorf("parsePageSize(page_size=%q) = %d, want %d", tt.query, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// parsePageOffset
// ---------------------------------------------------------------------------

func TestParsePageOffset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		query string
		want  int32
	}{
		{"", 0},
		{"0", 0},
		{"-1", 0},
		{"abc", 0},
		{"10", 10},
		{"50", 50},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "/media?page_offset="+tt.query, nil)

		got := parsePageOffset(req)
		if got != tt.want {
			t.Errorf("parsePageOffset(page_offset=%q) = %d, want %d", tt.query, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// UploadResult struct
// ---------------------------------------------------------------------------

func TestUploadResultJSON(t *testing.T) {
	t.Parallel()

	ur := UploadResult{
		MediaID:      "abc-123",
		StoragePath:  "temp/tenant1/file.jpg",
		PresignedURL: "https://seaweedfs.example.com/upload",
		ExpiresAt:    "2026-01-01T00:00:00Z",
	}

	data, err := json.Marshal(ur)
	if err != nil {
		t.Fatalf("marshal UploadResult: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got["media_id"] != "abc-123" {
		t.Errorf("media_id = %v, want abc-123", got["media_id"])
	}

	if got["storage_path"] != "temp/tenant1/file.jpg" {
		t.Errorf("storage_path = %v, want temp/tenant1/file.jpg", got["storage_path"])
	}
}

// ---------------------------------------------------------------------------
// Task payload structs
// ---------------------------------------------------------------------------

func TestTaskPayloadsJSON(t *testing.T) {
	t.Parallel()

	msp := MediaScanPayload{MediaID: "abc"}

	data, err := json.Marshal(msp)
	if err != nil {
		t.Fatalf("marshal MediaScanPayload: %v", err)
	}

	var got MediaScanPayload
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.MediaID != "abc" {
		t.Errorf("MediaID = %q, want abc", got.MediaID)
	}

	ipp := ImageProcessingPayload{MediaID: "def"}

	data, err = json.Marshal(ipp)
	if err != nil {
		t.Fatalf("marshal ImageProcessingPayload: %v", err)
	}

	var gotIP ImageProcessingPayload
	if err := json.Unmarshal(data, &gotIP); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if gotIP.MediaID != "def" {
		t.Errorf("MediaID = %q, want def", gotIP.MediaID)
	}

	vpp := VideoProcessingPayload{MediaID: "ghi", Transcode: true}

	data, err = json.Marshal(vpp)
	if err != nil {
		t.Fatalf("marshal VideoProcessingPayload: %v", err)
	}

	var gotVP VideoProcessingPayload
	if err := json.Unmarshal(data, &gotVP); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if gotVP.MediaID != "ghi" {
		t.Errorf("MediaID = %q, want ghi", gotVP.MediaID)
	}

	if !gotVP.Transcode {
		t.Error("Transcode = false, want true")
	}
}

// ---------------------------------------------------------------------------
// QuarantinedMedia struct
// ---------------------------------------------------------------------------

func TestQuarantinedMediaJSON(t *testing.T) {
	t.Parallel()
	m := QuarantinedMedia{
		ID:               mustUUID(t, "550e8400-e29b-41d4-a716-446655440000"),
		TenantID:         mustUUID(t, "00000000-0000-0000-0000-000000000001"),
		UploaderUserID:   mustUUID(t, "660e8400-e29b-41d4-a716-446655440001"),
		OriginalFilename: "photo.jpg",
		MimeType:         "image/jpeg",
		FileSizeBytes:    1024,
		StoragePath:      "media/tenant1/photo.jpg",
		StorageBucket:    "media",
		Confirmed:        false,
	}
	reason := "EICAR test virus"
	m.QuarantineReason = &reason

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal QuarantinedMedia: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got["original_filename"] != "photo.jpg" {
		t.Errorf("original_filename = %v, want photo.jpg", got["original_filename"])
	}

	if got["mime_type"] != "image/jpeg" {
		t.Errorf("mime_type = %v, want image/jpeg", got["mime_type"])
	}
}

// ---------------------------------------------------------------------------
// NewService constructor
// ---------------------------------------------------------------------------

func TestNewServiceNotNil(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	if svc.store != nil {
		t.Error("expected nil store for zero-value Service")
	}
}
