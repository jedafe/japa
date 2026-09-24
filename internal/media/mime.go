package media

import (
	"errors"
	"net/http"
	"path/filepath"
	"strings"
)

// ---------------------------------------------------------------------------
// MIME Alias Mapping
// ---------------------------------------------------------------------------

// mimeAlias maps non-canonical or legacy MIME types to their canonical forms.
// This prevents false rejections when clients send common variants like
// "image/jpg" instead of the IANA-registered "image/jpeg".
var mimeAlias = map[string]string{
	"image/jpg":         "image/jpeg",
	"image/pjpeg":       "image/jpeg",
	"image/x-jpeg":      "image/jpeg",
	"image/x-webp":      "image/webp",
	"image/x-png":       "image/png",
	"video/mpeg":        "video/mpeg",
	"video/mpg":         "video/mpeg",
	"video/mpe":         "video/mpeg",
	"video/x-mpeg":      "video/mpeg",
	"video/x-msvideo":   "video/avi",
	"audio/x-wav":       "audio/wav",
	"audio/wave":        "audio/wav",
	"audio/x-pn-wav":    "audio/wav",
	"application/x-pdf": "application/pdf",
	"text/x-markdown":   "text/markdown",
	"application/x-rtf": "application/rtf",
	"text/rtf":          "application/rtf",
}

// ---------------------------------------------------------------------------
// Allowed MIME Types Per Upload Context
// ---------------------------------------------------------------------------

// UploadContext identifies the purpose of an upload, which determines
// which MIME types are permitted.
type UploadContext string

const (
	// UploadContextMedia is the upload context media constant.
	UploadContextMedia UploadContext = "media"
	// UploadContextAvatar is the upload context avatar.
	UploadContextAvatar UploadContext = "avatar"
	// UploadContextDocument is the upload context document.
	UploadContextDocument UploadContext = "document"
)

// AllowedMIMETypesByContext defines which MIME types are permitted for each
// upload context. Each entry is an allowed MIME type in its canonical form.
var AllowedMIMETypesByContext = map[UploadContext][]string{
	UploadContextMedia: {
		"image/jpeg",
		"image/png",
		"image/webp",
		"image/gif",
		"image/svg+xml",
		"video/mp4",
		"video/webm",
		"audio/mpeg",
		"audio/ogg",
		"audio/wav",
		"application/pdf",
	},
	UploadContextAvatar: {
		"image/jpeg",
		"image/png",
		"image/webp",
	},
	// UploadContextDocument permits CV/letter-style attachments (e.g. job
	// application CVs per §63) alongside PDFs and the image fallbacks used
	// for scanned documents.
	UploadContextDocument: {
		"application/pdf",
		"application/msword",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.oasis.opendocument.text",
		"application/rtf",
		"text/plain",
		"text/markdown",
		"image/jpeg",
		"image/png",
	},
}

// ExtensionToMIME maps common file extensions to their canonical MIME types.
// Used as a secondary signal when http.DetectContentType returns
// application/octet-stream for ambiguous binary content.
var ExtensionToMIME = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
	".mp4":  "video/mp4",
	".webm": "video/webm",
	".mp3":  "audio/mpeg",
	".ogg":  "audio/ogg",
	".wav":  "audio/wav",
	".pdf":  "application/pdf",
	// Document formats (UploadContextDocument — §63 job applications).
	".doc":      "application/msword",
	".docx":     "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".odt":      "application/vnd.oasis.opendocument.text",
	".rtf":      "application/rtf",
	".txt":      "text/plain",
	".md":       "text/markdown",
	".markdown": "text/markdown",
}

// containerCompat maps types reported by http.DetectContentType for
// container-format files to the declared types they may legitimately hold.
// Office documents are containers: OOXML files (.docx/.xlsx/.pptx) are ZIP
// archives, legacy Office files (.doc/.xls/.ppt) are OLE2 compound storage.
var containerCompat = map[string][]string{
	"application/zip": {
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation",
		"application/epub+zip",
	},
	"application/x-ole-storage": {
		"application/msword",
		"application/vnd.ms-excel",
		"application/vnd.ms-powerpoint",
	},
}

// ---------------------------------------------------------------------------
// Core Functions
// ---------------------------------------------------------------------------

// CanonicalMIMEType normalizes a MIME type through alias mapping and
// lowercasing, and strips any parameters (e.g. "; charset=utf-8" as
// returned by http.DetectContentType for text content). Returns the
// IANA-registered canonical form when one exists, or the normalized
// input otherwise.
func CanonicalMIMEType(mimeType string) string {
	normalized := strings.ToLower(strings.TrimSpace(mimeType))
	if i := strings.IndexByte(normalized, ';'); i >= 0 {
		normalized = strings.TrimSpace(normalized[:i])
	}
	if canonical, ok := mimeAlias[normalized]; ok {
		return canonical
	}

	return normalized
}

// VerifyMIMEType detects the actual content type from file bytes using
// http.DetectContentType (content-inspection, never trusting client headers)
// and compares it against the declared MIME type.
//
// Returns:
//   - detectedType: the type identified by http.DetectContentType
//   - matchesExpected: true if detected matches declared (after alias resolution)
//   - error: non-nil only for truly invalid input (empty bytes)
//
// When http.DetectContentType returns "application/octet-stream" (unidentifiable
// binary), matchesExpected is true — the content may be valid but not
// identifiable from the first 512 bytes alone (e.g. encrypted files, large
// video containers). Rejection should only happen when a concrete type is
// detected that conflicts with the declared type.
func VerifyMIMEType(fileBytes []byte, declaredMimeType string) (detectedType string, matchesExpected bool, err error) {
	// Guard: empty input is a hard error.
	if len(fileBytes) == 0 {
		return "", false, errors.New("mime verification: empty file bytes")
	}

	detected := http.DetectContentType(fileBytes)
	canonical := CanonicalMIMEType(detected)
	declared := CanonicalMIMEType(declaredMimeType)

	// "application/octet-stream" means the content is binary but not
	// identifiable from the header alone. Allow through — it's valid
	// content that simply can't be fingerprinted.
	if canonical == "application/octet-stream" {
		return detected, true, nil
	}

	if canonical == declared {
		return detected, true, nil
	}

	// Container formats: DetectContentType only sees the outer container
	// (zip / OLE2), not the document type inside. Accept when the declared
	// type is a document known to live in that container.
	for _, inner := range containerCompat[canonical] {
		if inner == declared {
			return detected, true, nil
		}
	}

	return detected, false, nil
}

// ValidateFileType checks whether the detected MIME type is in the allowed
// list for a given upload context. Uses canonical comparison so alias types
// are accepted.
func ValidateFileType(detected string, allowed []string) bool {
	canonical := CanonicalMIMEType(detected)
	for _, a := range allowed {
		if CanonicalMIMEType(a) == canonical {
			return true
		}
	}

	return false
}

// MIMETypeForContext returns the allowed MIME types for the given upload
// context, or nil if the context is unknown.
func MIMETypeForContext(ctx UploadContext) []string {
	return AllowedMIMETypesByContext[ctx]
}

// MIMETypeFromExtension infers a MIME type from a file extension. Returns
// empty string if the extension is not recognized.
func MIMETypeFromExtension(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	return ExtensionToMIME[ext]
}

// IsAllowedMIMETypeForContext checks if a declared MIME type (possibly
// non-canonical) is allowed for the given upload context.
func IsAllowedMIMETypeForContext(mimeType string, ctx UploadContext) bool {
	allowed := AllowedMIMETypesByContext[ctx]
	if allowed == nil {
		return false
	}

	return ValidateFileType(mimeType, allowed)
}
