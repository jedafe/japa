package media

import (
	"testing"
)

// ---------------------------------------------------------------------------
// Test Fixtures: Real file magic bytes
// ---------------------------------------------------------------------------

// JPEG file header: FF D8 FF E0 (JFIF) or FF D8 FF E1 (EXIF)
var jpegBytes = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01}

// PNG file header: 89 50 4E 47 0D 0A 1A 0A
var pngBytes = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D}

// GIF89a header: 47 49 46 38 39 61
var gifBytes = []byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00}

// PDF header: 25 50 44 46 2D (%PDF-)
var pdfBytes = []byte{0x25, 0x50, 0x44, 0x46, 0x2D, 0x31, 0x2E, 0x37}

// BMP header: 42 4D (BM)
var bmpBytes = []byte{0x42, 0x4D, 0x38, 0x00, 0x00, 0x00}

// EXE/PE header: 4D 5A (MZ) — Go's http.DetectContentType does NOT have
// a specific rule for MZ, so it returns "application/octet-stream".
// We include it for completeness but note that the real defense against
// EXE spoofing is the allowed-types check (EXE is never in the allowed set).
var exeBytes = []byte{0x4D, 0x5A, 0x90, 0x00, 0x03, 0x00, 0x00, 0x00}

// ELF header: 7F 45 4C 46 — same situation as MZ; not identified by
// http.DetectContentType, falls through to application/octet-stream.
var elfBytes = []byte{0x7F, 0x45, 0x4C, 0x46, 0x02, 0x01, 0x01, 0x00}

// TIFF header: 49 49 2A 00 (little-endian) — NOTE: Go's http.DetectContentType
// does NOT have a specific rule for TIFF, so it returns "application/octet-stream".
// We include it for completeness but note it falls through to the octet-stream path.
var tiffBytes = []byte{0x49, 0x49, 0x2A, 0x00, 0x08, 0x00, 0x00, 0x00}

// MP4 ftyp box (typical): 00 00 00 xx 66 74 79 70
var mp4Bytes = []byte{0x00, 0x00, 0x00, 0x18, 0x66, 0x74, 0x79, 0x70, 0x69, 0x73, 0x6F, 0x6D}

// WebP header: RIFF....WEBP
var webPBytes = []byte{0x52, 0x49, 0x46, 0x46, 0x24, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50}

// ---------------------------------------------------------------------------
// CanonicalMIMEType
// ---------------------------------------------------------------------------

func TestCanonicalMIMEType(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		// Canonical types pass through unchanged.
		{"image/jpeg", "image/jpeg"},
		{"image/png", "image/png"},
		{"video/mp4", "video/mp4"},
		{"application/pdf", "application/pdf"},

		// Alias mapping.
		{"image/jpg", "image/jpeg"},
		{"image/pjpeg", "image/jpeg"},
		{"image/x-jpeg", "image/jpeg"},
		{"image/x-webp", "image/webp"},
		{"image/x-png", "image/png"},
		{"audio/x-wav", "audio/wav"},
		{"audio/wave", "audio/wav"},
		{"application/x-pdf", "application/pdf"},

		// Case insensitivity.
		{"IMAGE/JPEG", "image/jpeg"},
		{"Image/PNG", "image/png"},
		{"VIDEO/MP4", "video/mp4"},

		// Whitespace trimming.
		{"  image/jpeg  ", "image/jpeg"},
		{" image/png ", "image/png"},

		// Unknown types return lowercased input.
		{"custom/type", "custom/type"},
		{"", ""},
	}
	for _, tt := range tests {
		got := CanonicalMIMEType(tt.input)
		if got != tt.want {
			t.Errorf("CanonicalMIMEType(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// VerifyMIMEType
// ---------------------------------------------------------------------------

func TestVerifyMIMEType_Matches(t *testing.T) {
	tests := []struct {
		name     string
		fileData []byte
		declared string
		wantOK   bool
	}{
		{"JPEG matches image/jpeg", jpegBytes, "image/jpeg", true},
		{"JPEG matches alias image/jpg", jpegBytes, "image/jpg", true},
		{"JPEG matches alias image/pjpeg", jpegBytes, "image/pjpeg", true},
		{"PNG matches image/png", pngBytes, "image/png", true},
		{"GIF matches image/gif", gifBytes, "image/gif", true},
		{"PDF matches application/pdf", pdfBytes, "application/pdf", true},
		{"MP4 matches video/mp4", mp4Bytes, "video/mp4", true},
		{"WebP matches image/webp", webPBytes, "image/webp", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detected, ok, err := VerifyMIMEType(tt.fileData, tt.declared)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !ok != !tt.wantOK {
				t.Errorf("matchesExpected = %v, want %v (detected=%s, declared=%s)",
					ok, tt.wantOK, detected, tt.declared)
			}
		})
	}
}

func TestVerifyMIMEType_Mismatches(t *testing.T) {
	tests := []struct {
		name     string
		fileData []byte
		declared string
	}{
		// These are all formats http.DetectContentType CAN identify.
		// When the detected type conflicts with declared, it's a mismatch.
		{"BMP claimed as JPEG", bmpBytes, "image/jpeg"},
		{"GIF claimed as JPEG", gifBytes, "image/jpeg"},
		{"PNG claimed as GIF", pngBytes, "image/gif"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detected, ok, err := VerifyMIMEType(tt.fileData, tt.declared)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ok {
				t.Errorf("expected mismatch but got matchesExpected=true (detected=%s, declared=%s)",
					detected, tt.declared)
			}
		})
	}
}

func TestVerifyMIMEType_OctetStreamNotRejected(t *testing.T) {
	// Formats that http.DetectContentType returns "application/octet-stream"
	// for (MZ, ELF, TIFF) should NOT be rejected — the detector simply can't
	// fingerprint them from the header alone. The defense against these
	// is the allowed-types check, not content inspection.
	tests := []struct {
		name     string
		fileData []byte
		declared string
	}{
		{"MZ/EXE as JPEG", exeBytes, "image/jpeg"},
		{"MZ/EXE as PNG", exeBytes, "image/png"},
		{"ELF as PDF", elfBytes, "application/pdf"},
		{"TIFF as JPEG", tiffBytes, "image/jpeg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detected, ok, err := VerifyMIMEType(tt.fileData, tt.declared)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !ok {
				t.Errorf("octet-stream should pass through (detected=%s, declared=%s)", detected, tt.declared)
			}
		})
	}
}

func TestVerifyMIMEType_EmptyBytes(t *testing.T) {
	_, _, err := VerifyMIMEType([]byte{}, "image/jpeg")
	if err == nil {
		t.Error("expected error for empty file bytes, got nil")
	}
}

// ---------------------------------------------------------------------------
// MIME Spoofing Detection
// ---------------------------------------------------------------------------

func TestSpoofingDetection(t *testing.T) {
	// Scenario 1: A BMP file renamed to photo.jpg and claimed as image/jpeg.
	// http.DetectContentType identifies BMP and rejects the mismatch.
	detected, ok, err := VerifyMIMEType(bmpBytes, "image/jpeg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Errorf("spoofed BMP claimed as JPEG should be rejected, detected=%s", detected)
	}

	// Scenario 2: A GIF file renamed to document.png and claimed as image/png.
	detected, ok, err = VerifyMIMEType(gifBytes, "image/png")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Errorf("spoofed GIF claimed as PNG should be rejected, detected=%s", detected)
	}

	// Scenario 3: EXE/MZ — http.DetectContentType returns octet-stream,
	// so content inspection alone can't catch it. This is why the
	// allowed-types check in InitiateUpload is the primary defense:
	// "application/x-msdownload" is never in AllowedMIMETypes.
	detected, ok, err = VerifyMIMEType(exeBytes, "image/jpeg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Errorf("MZ as octet-stream should pass through, detected=%s", detected)
	}
	// But the allowed-types layer catches it:
	if IsAllowedMIMETypeForContext("application/x-msdownload", UploadContextMedia) {
		t.Error("application/x-msdownload should NOT be in media allowed types")
	}
}

// ---------------------------------------------------------------------------
// ValidateFileType
// ---------------------------------------------------------------------------

func TestValidateFileType(t *testing.T) {
	mediaTypes := AllowedMIMETypesByContext[UploadContextMedia]
	avatarTypes := AllowedMIMETypesByContext[UploadContextAvatar]

	tests := []struct {
		name    string
		detected string
		allowed  []string
		want     bool
	}{
		{"JPEG in media types", "image/jpeg", mediaTypes, true},
		{"PNG in media types", "image/png", mediaTypes, true},
		{"WebP in media types", "image/webp", mediaTypes, true},
		{"GIF in media types", "image/gif", mediaTypes, true},
		{"SVG in media types", "image/svg+xml", mediaTypes, true},
		{"MP4 in media types", "video/mp4", mediaTypes, true},
		{"WebM in media types", "video/webm", mediaTypes, true},
		{"MP3 in media types", "audio/mpeg", mediaTypes, true},
		{"OGG in media types", "audio/ogg", mediaTypes, true},
		{"WAV in media types", "audio/wav", mediaTypes, true},
		{"PDF in media types", "application/pdf", mediaTypes, true},
		{"EXE not in media types", "application/x-msdownload", mediaTypes, false},
		{"BMP not in media types", "image/bmp", mediaTypes, false},
		{"JPEG in avatar types (allowed)", "image/jpeg", avatarTypes, true},
		{"PNG in avatar types (allowed)", "image/png", avatarTypes, true},
		{"WebP in avatar types (allowed)", "image/webp", avatarTypes, true},
		{"GIF in avatar types (denied)", "image/gif", avatarTypes, false},
		{"PDF in avatar types (denied)", "application/pdf", avatarTypes, false},

		// Alias acceptance in ValidateFileType.
		{"image/jpg alias in media types", "image/jpg", mediaTypes, true},
		{"image/pjpeg alias in media types", "image/pjpeg", mediaTypes, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateFileType(tt.detected, tt.allowed)
			if got != tt.want {
				t.Errorf("ValidateFileType(%q, ...) = %v, want %v", tt.detected, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MIMETypeFromExtension
// ---------------------------------------------------------------------------

func TestMIMETypeFromExtension(t *testing.T) {
	tests := []struct {
		filename, want string
	}{
		{"photo.jpg", "image/jpeg"},
		{"photo.jpeg", "image/jpeg"},
		{"image.png", "image/png"},
		{"animation.gif", "image/gif"},
		{"modern.webp", "image/webp"},
		{"icon.svg", "image/svg+xml"},
		{"video.mp4", "video/mp4"},
		{"clip.webm", "video/webm"},
		{"music.mp3", "audio/mpeg"},
		{"sound.ogg", "audio/ogg"},
		{"recording.wav", "audio/wav"},
		{"document.pdf", "application/pdf"},
		{"unknown.xyz", ""},
		{"noext", ""},
	}
	for _, tt := range tests {
		got := MIMETypeFromExtension(tt.filename)
		if got != tt.want {
			t.Errorf("MIMETypeFromExtension(%q) = %q, want %q", tt.filename, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// IsAllowedMIMETypeForContext
// ---------------------------------------------------------------------------

func TestIsAllowedMIMETypeForContext(t *testing.T) {
	tests := []struct {
		name    string
		mime    string
		ctx     UploadContext
		want    bool
	}{
		{"JPEG allowed for media", "image/jpeg", UploadContextMedia, true},
		{"image/jpg alias allowed for media", "image/jpg", UploadContextMedia, true},
		{"EXE denied for media", "application/x-msdownload", UploadContextMedia, false},
		{"JPEG allowed for avatar", "image/jpeg", UploadContextAvatar, true},
		{"GIF denied for avatar", "image/gif", UploadContextAvatar, false},
		{"PDF allowed for document", "application/pdf", UploadContextDocument, true},
		{"EXE denied for document", "application/x-msdownload", UploadContextDocument, false},
		{"Unknown context denied", "image/jpeg", UploadContext("bogus"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsAllowedMIMETypeForContext(tt.mime, tt.ctx)
			if got != tt.want {
				t.Errorf("IsAllowedMIMETypeForContext(%q, %q) = %v, want %v",
					tt.mime, tt.ctx, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MIMETypeForContext
// ---------------------------------------------------------------------------

func TestMIMETypeForContext(t *testing.T) {
	media := MIMETypeForContext(UploadContextMedia)
	if len(media) == 0 {
		t.Fatal("MIMETypeForContext(UploadContextMedia) returned nil")
	}

	bogus := MIMETypeForContext(UploadContext("bogus"))
	if bogus != nil {
		t.Errorf("MIMETypeForContext(bogus) = %v, want nil", bogus)
	}
}
