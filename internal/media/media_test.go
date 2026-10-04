package media

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
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

func TestMediaURLReturnsLocalServeWhenEnvMissing(t *testing.T) {
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
	want := "/api/v1/media/serve/media/test/photo.jpg"

	if got != want {
		t.Errorf("MediaURL with missing env vars = %q, want %q", got, want)
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

func TestServiceGetMediaURLNonImage(t *testing.T) {
	t.Parallel()

	// Non-image binaries (videos, audio, PDFs) must get the same-origin
	// serve URL — imgproxy cannot transform them, so a signed imgproxy URL
	// would never resolve.
	svc := &Service{}
	m := &gen.Medium{
		StorageBucket: "media",
		StoragePath:   "media/clip.mp4",
		MimeType:      "video/mp4",
	}

	got := svc.GetMediaURL(m, ImageProxyOpts{})
	want := "/api/v1/media/serve/media/media/clip.mp4"

	if got != want {
		t.Errorf("GetMediaURL for video = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// writeJSON / writeError (copied from follows for handler tests)
// ---------------------------------------------------------------------------

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	errpkg.WriteJSON(rr, http.StatusCreated, map[string]string{"status": "ok"})

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
	errpkg.WriteError(rr, http.StatusBadRequest, "invalid_body", "Invalid JSON body")

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
		StoragePath:  "temp/platform1/file.jpg",
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

	if got["storage_path"] != "temp/platform1/file.jpg" {
		t.Errorf("storage_path = %v, want temp/platform1/file.jpg", got["storage_path"])
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
		ID: mustUUID(t, "550e8400-e29b-41d4-a716-446655440000"),

		UploaderUserID:   mustUUID(t, "660e8400-e29b-41d4-a716-446655440001"),
		OriginalFilename: "photo.jpg",
		MimeType:         "image/jpeg",
		FileSizeBytes:    1024,
		StoragePath:      "media/platform1/photo.jpg",
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
// storeLocalMediaFile / extensionFromMIME (university:fetch_logo local
// fallback — regression guard: a media record must never be
// created/confirmed for bytes that were never actually stored anywhere.)
// ---------------------------------------------------------------------------

func TestExtensionFromMIME(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mimeType string
		want     string
	}{
		{"image/png", ".png"},
		{"image/jpeg", ".jpg"},
		{"image/webp", ".webp"},
		{"image/gif", ".gif"},
		{"image/svg+xml", ".svg"},
		{"application/octet-stream", ".bin"},
		{"", ".bin"},
	}
	for _, tt := range tests {
		if got := extensionFromMIME(tt.mimeType); got != tt.want {
			t.Errorf("extensionFromMIME(%q) = %q, want %q", tt.mimeType, got, tt.want)
		}
	}
}

func TestStoreLocalMediaFile_WritesRetrievableBytes(t *testing.T) {
	// Not parallel: writes to the shared data/media/ directory relative to
	// the package's working directory.
	bucket := "media"
	storagePath := "test-fetch-logo/" + t.Name() + "/logo.png"
	body := []byte("fake-png-bytes")

	t.Cleanup(func() {
		_ = os.RemoveAll(filepath.Dir(mediaRoot)) // "data" — test-only local storage under CWD, safe to remove entirely
	})

	if err := storeLocalMediaFile(bucket, storagePath, body); err != nil {
		t.Fatalf("storeLocalMediaFile: %v", err)
	}

	localPath, err := resolveMediaPath(bucket, storagePath)
	if err != nil {
		t.Fatalf("resolveMediaPath: %v", err)
	}

	got, err := os.ReadFile(filepath.Clean(localPath))
	if err != nil {
		t.Fatalf("reading stored file: %v", err)
	}

	if string(got) != string(body) {
		t.Errorf("stored content = %q, want %q", got, body)
	}
}

func TestStoreLocalMediaFile_RejectsPathEscape(t *testing.T) {
	t.Parallel()

	if err := storeLocalMediaFile("media", "../../etc/passwd", []byte("x")); err == nil {
		t.Fatal("expected error for path traversal, got nil")
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

// ---------------------------------------------------------------------------
// SettingsProvider fake + settings-driven gates
// ---------------------------------------------------------------------------

// fakeSettings implements SettingsProvider over a plain map.
type fakeSettings map[string]string

func (f fakeSettings) GetIntOr(_ context.Context, key string, fallback int) int {
	if v, ok := f[key]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func (f fakeSettings) GetStringOr(_ context.Context, key, fallback string) string {
	if v, ok := f[key]; ok {
		return v
	}
	return fallback
}

func TestStorageBucket(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	if got := (&Service{}).storageBucket(ctx); got != DefaultBucket {
		t.Errorf("no config: storageBucket = %q, want %q", got, DefaultBucket)
	}

	svc := &Service{settings: fakeSettings{"media.storage.bucket": "custom-bucket"}}
	if got := svc.storageBucket(ctx); got != "custom-bucket" {
		t.Errorf("settings bucket: got %q, want custom-bucket", got)
	}

	// Blank/whitespace setting falls back to the default.
	svc = &Service{settings: fakeSettings{"media.storage.bucket": "  "}}
	if got := svc.storageBucket(ctx); got != DefaultBucket {
		t.Errorf("blank bucket setting: got %q, want %q", got, DefaultBucket)
	}

	// A configured S3 client wins — it already resolved media.storage.bucket.
	svc = &Service{
		settings: fakeSettings{"media.storage.bucket": "custom-bucket"},
		s3:       &S3Client{bucket: "s3bucket"},
	}
	if got := svc.storageBucket(ctx); got != "s3bucket" {
		t.Errorf("s3 client bucket: got %q, want s3bucket", got)
	}
}

func TestIsExtAllowed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mime string
		csv  string
		want bool
	}{
		{"image/jpeg", "jpg", true},
		{"image/jpeg", "jpeg,jpg", true},
		{"image/jpeg", " .jpg ", true},     // whitespace + leading dot
		{"image/jpeg", "image/jpg", false}, // MIME tokens don't match extensions
		{"image/webp", "png,jpg", false},
		{"video/quicktime", "mov", true},
		{"application/pdf", "pdf", true},
		{"application/pdf", "", false},
		{"image/png", ",,", false},
	}
	for _, tt := range tests {
		if got := isExtAllowed(tt.mime, tt.csv); got != tt.want {
			t.Errorf("isExtAllowed(%q, %q) = %v, want %v", tt.mime, tt.csv, got, tt.want)
		}
	}
}

func TestIsStorableMediaMIME(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mime string
		want bool
	}{
		{"image/jpeg", true},
		{"image/svg+xml", true},
		{"image/bmp", true}, // ingestion accepts any image/* — read path decides rendering
		{"video/mp4", true},
		{"video/x-matroska", true},
		{"audio/mpeg", true},
		{"application/pdf", true},
		{"text/html", false},
		{"text/plain", false},
		{"application/zip", false},
		{"application/octet-stream", false},
		{"application/x-msdownload", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isStorableMediaMIME(tt.mime); got != tt.want {
			t.Errorf("isStorableMediaMIME(%q) = %v, want %v", tt.mime, got, tt.want)
		}
	}
}

func TestDetectStorageMIME(t *testing.T) {
	t.Parallel()

	// Minimal PNG header — DetectContentType identifies it from magic bytes.
	png := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D}
	if got := DetectStorageMIME(png, "logo.bin"); got != "image/png" {
		t.Errorf("png bytes named .bin = %q, want image/png (content wins over ext)", got)
	}

	// SVG has no magic bytes — sniffs as text, resolved via the .svg ext.
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)
	if got := DetectStorageMIME(svg, "logo.svg"); got != "image/svg+xml" {
		t.Errorf("svg bytes = %q, want image/svg+xml", got)
	}

	// Unknown binary keeps octet-stream when the extension is unrecognized.
	if got := DetectStorageMIME([]byte{0x00, 0xDE, 0xAD, 0xBE, 0xEF}, "blob.xyz"); got != "application/octet-stream" {
		t.Errorf("unknown binary = %q, want application/octet-stream", got)
	}
}

func TestIsAllowedMIMEContextSettingsGate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Unset setting — base context allowlist alone applies.
	svc := &Service{settings: fakeSettings{}}
	for _, mt := range []string{"image/webp", "image/gif", "application/pdf", "audio/mpeg"} {
		if !svc.isAllowedMIME(ctx, mt, UploadContextMedia) {
			t.Errorf("unset content.allowed_upload_types rejected %s", mt)
		}
	}

	// Configured CSV narrows the base allowlist. "jpg" alias resolves to
	// image/jpeg; types not in the CSV are rejected.
	svc = &Service{settings: fakeSettings{"content.allowed_upload_types": "png,jpg"}}
	tests := []struct {
		mime string
		want bool
	}{
		{"image/png", true},
		{"image/jpeg", true},  // jpg alias
		{"image/jpg", true},   // non-canonical input, canonicalized first
		{"image/webp", false}, // not in the configured CSV
		{"image/gif", false},
		{"application/pdf", false},
		{"video/mp4", false}, // also fails the extension list
	}
	for _, tt := range tests {
		if got := svc.isAllowedMIME(ctx, tt.mime, UploadContextMedia); got != tt.want {
			t.Errorf("isAllowedMIME(%q, media) with csv=%q = %v, want %v",
				tt.mime, "png,jpg", got, tt.want)
		}
	}
	// Types outside the base allowlist stay rejected even if the CSV lists
	// an extension for them (the gate narrows, never widens).
	svc = &Service{settings: fakeSettings{"content.allowed_upload_types": "docx,png"}}
	if svc.isAllowedMIME(ctx, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", UploadContextMedia) {
		t.Error("docx in CSV must not widen the media-context allowlist")
	}

	// Avatar context consults users.allowed_avatar_types, not the content key.
	svc = &Service{settings: fakeSettings{
		"content.allowed_upload_types": "png,jpg",
		"users.allowed_avatar_types":   "webp",
	}}
	if svc.isAllowedMIME(ctx, "image/png", UploadContextAvatar) {
		t.Error("avatar context must use users.allowed_avatar_types, not content.allowed_upload_types")
	}
	if !svc.isAllowedMIME(ctx, "image/webp", UploadContextAvatar) {
		t.Error("webp avatar should be allowed when users.allowed_avatar_types=webp")
	}
	// gif is in the avatar base allowlist; an admin-removal CSV rejects it.
	if svc.isAllowedMIME(ctx, "image/gif", UploadContextAvatar) {
		t.Error("gif avatar should be rejected when not in users.allowed_avatar_types")
	}

	// Document context is intentionally ungated by content.allowed_upload_types.
	svc = &Service{settings: fakeSettings{"content.allowed_upload_types": "png"}}
	if !svc.isAllowedMIME(ctx, "application/pdf", UploadContextDocument) {
		t.Error("content.allowed_upload_types must not gate document uploads")
	}

	// Videos must satisfy both media.allowed_video_types and the context ext list.
	svc = &Service{settings: fakeSettings{
		"content.allowed_upload_types": "mp4,webm",
		"media.allowed_video_types":    "webm",
	}}
	if svc.isAllowedMIME(ctx, "video/mp4", UploadContextMedia) {
		t.Error("mp4 allowed by ext list but blocked by media.allowed_video_types")
	}
	if !svc.isAllowedMIME(ctx, "video/webm", UploadContextMedia) {
		t.Error("webm allowed by both lists should pass")
	}
}

// TestOptimizeImage covers the ingest/upload compression path: oversized
// rasters downscale to OptimizeMaxDimension and re-encode to WebP (alpha
// preserved natively), and pass-through cases (small files, gif, svg,
// non-images) return byte-identical data.
func TestOptimizeImage(t *testing.T) {
	// noiseImage returns a PNG with pseudo-random pixels — incompressible
	// enough to exceed OptimizeSkipBytes at modest dimensions.
	noiseImage := func(w, h int, opaque bool) []byte {
		img := image.NewNRGBA(image.Rect(0, 0, w, h))
		rnd := rand.New(rand.NewSource(42))
		if _, err := rnd.Read(img.Pix); err != nil {
			t.Fatalf("rand read: %v", err)
		}
		if opaque {
			for i := 3; i < len(img.Pix); i += 4 {
				img.Pix[i] = 255
			}
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			t.Fatalf("png encode: %v", err)
		}
		return buf.Bytes()
	}

	decodeBounds := func(t *testing.T, data []byte) (string, int, int) {
		t.Helper()
		img, format, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("decode output: %v", err)
		}
		b := img.Bounds()
		return format, b.Dx(), b.Dy()
	}

	t.Run("raster re-encodes to webp and shrinks", func(t *testing.T) {
		orig := noiseImage(800, 600, true)
		if len(orig) < OptimizeSkipBytes {
			t.Fatalf("fixture too small: %d bytes", len(orig))
		}
		out, outMIME := optimizeImage(orig, "image/png")
		if outMIME != OptimizeMIME {
			t.Fatalf("expected %q output, got %q", OptimizeMIME, outMIME)
		}
		format, w, h := decodeBounds(t, out)
		if format != "webp" || w != 800 || h != 600 {
			t.Fatalf("unexpected output %s %dx%d", format, w, h)
		}
		if len(out) >= len(orig) {
			t.Fatalf("expected smaller output: %d >= %d", len(out), len(orig))
		}
	})

	t.Run("oversized dimensions downscale to OptimizeMaxDimension", func(t *testing.T) {
		orig := noiseImage(3000, 200, true)
		out, _ := optimizeImage(orig, "image/png")
		_, w, h := decodeBounds(t, out)
		if w != OptimizeMaxDimension {
			t.Fatalf("expected width %d, got %d", OptimizeMaxDimension, w)
		}
		if h >= 200 {
			t.Fatalf("expected proportional height < 200, got %d", h)
		}
	})

	t.Run("alpha output is webp and keeps transparency", func(t *testing.T) {
		orig := noiseImage(800, 600, false)
		out, outMIME := optimizeImage(orig, "image/png")
		if outMIME != OptimizeMIME {
			t.Fatalf("expected %q preserving alpha, got %q", OptimizeMIME, outMIME)
		}
		img, format, err := image.Decode(bytes.NewReader(out))
		if err != nil {
			t.Fatalf("decode output: %v", err)
		}
		if format != "webp" {
			t.Fatalf("expected webp output, got %s", format)
		}
		// Verify the alpha channel survived lossy WebP encoding — scan a
		// corner block; random alphas must produce non-opaque pixels.
		nonOpaque := false
		for y := 0; y < 10 && !nonOpaque; y++ {
			for x := 0; x < 10 && !nonOpaque; x++ {
				if _, _, _, a := img.At(x, y).RGBA(); a < 0xf000 {
					nonOpaque = true
				}
			}
		}
		if !nonOpaque {
			t.Fatal("expected non-opaque alpha to be preserved")
		}
	})

	t.Run("small files pass through byte-identical", func(t *testing.T) {
		small := noiseImage(10, 10, true)
		if len(small) >= OptimizeSkipBytes {
			t.Fatalf("fixture unexpectedly large: %d", len(small))
		}
		out, outMIME := optimizeImage(small, "image/png")
		if outMIME != "image/png" || !bytes.Equal(out, small) {
			t.Fatal("small image must pass through unchanged")
		}
	})

	t.Run("gif and non-image pass through", func(t *testing.T) {
		blob := bytes.Repeat([]byte{0xAB}, OptimizeSkipBytes+1)
		for _, mime := range []string{"image/gif", "image/svg+xml", "video/mp4", "application/pdf"} {
			out, outMIME := optimizeImage(blob, mime)
			if outMIME != mime || !bytes.Equal(out, blob) {
				t.Fatalf("%s must pass through unchanged", mime)
			}
		}
	})
}
