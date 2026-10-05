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
	"audio/x-m4a":       "audio/mp4",
	"audio/m4a":         "audio/mp4",
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
		"video/quicktime",
		"audio/mpeg",
		"audio/ogg",
		"audio/wav",
		"audio/mp4",
		"application/pdf",
	},
	// Avatar allowlist must cover every option exposed by the
	// users.allowed_avatar_types setting (jpeg, jpg, png, gif, webp, svg) so
	// the configured choices are actually reachable — the setting gate can
	// only narrow this list. SVG is already hardened on serve (nosniff+CSP)
	// and permitted in the media context.
	UploadContextAvatar: {
		"image/jpeg",
		"image/png",
		"image/webp",
		"image/gif",
		"image/svg+xml",
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
	".mov":  "video/quicktime",
	".mp3":  "audio/mpeg",
	".ogg":  "audio/ogg",
	".wav":  "audio/wav",
	".m4a":  "audio/mp4",
	".pdf":  "application/pdf",
	// Image types the allowlists reject but detection still labels honestly
	// (e.g. ingestion, future enablement) rather than as .bin blobs.
	".avif": "image/avif",
	".heic": "image/heic",
	".heif": "image/heif",
	".bmp":  "image/bmp",
	".tiff": "image/tiff",
	".tif":  "image/tiff",
	// Video containers outside the upload allowlist — classified correctly
	// for ingestion; user uploads are still allowlist-gated.
	".m4v": "video/x-m4v",
	".mkv": "video/x-matroska",
	".avi": "video/x-msvideo",
	".3gp": "video/3gpp",
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

// textCompat lists declared types whose content is plain text or XML and is
// therefore indistinguishable to http.DetectContentType — it reports
// "text/plain" or "text/xml" for all of them. Markdown, RTF, and SVG have no
// magic bytes.
var textCompat = map[string]bool{
	"text/plain":      true,
	"text/markdown":   true,
	"text/csv":        true,
	"text/xml":        true,
	"application/rtf": true,
	"image/svg+xml":   true,
}

// bmffBrandMIME maps ISO base-media file format (ISO-BMFF) ftyp major
// brands to the MIME type the file actually carries. http.DetectContentType
// fingerprints only the literal "mp4 " brand — every other brand (isom,
// heic, avif…) reports application/octet-stream even though the container
// is identified by the 4-byte major brand at offset 8. isom is the ffmpeg
// default and the most common MP4 brand in the wild.
var bmffBrandMIME = map[string]string{
	// Still-image brands: HEIC/HEIF/AVIF/JXL/JP2 photos. None of these
	// types are in the upload allowlists — they exist here so mislabeled
	// declarations (e.g. HEIC uploaded as video/mp4) are caught instead of
	// slipping through the octet-stream free-pass.
	"heic": "image/heic",
	"heix": "image/heic",
	"mif1": "image/heif",
	"msf1": "image/heif",
	"avif": "image/avif",
	"avis": "image/avif",
	"jxl ": "image/jxl",
	"jp2 ": "image/jp2",
	// Audio-only MP4 brands (m4a/m4b/m4p).
	"M4A ": "audio/mp4",
	"M4B ": "audio/mp4",
	"M4P ": "audio/mp4",
	// Video brands with their own registered types.
	"qt  ": "video/quicktime",
	"M4V ": "video/x-m4v",
	"3gp4": "video/3gpp",
	"3gp5": "video/3gpp",
	"3gp6": "video/3gpp",
	"3g2a": "video/3gpp2",
	"3g2b": "video/3gpp2",
	"3g2c": "video/3gpp2",
	// The remaining registered video brands all resolve to video/mp4.
	"isom": "video/mp4",
	"iso2": "video/mp4",
	"iso4": "video/mp4",
	"iso5": "video/mp4",
	"iso6": "video/mp4",
	"iso8": "video/mp4",
	"avc1": "video/mp4",
	"mp41": "video/mp4",
	"mp42": "video/mp4",
	"mp71": "video/mp4",
	"mmp4": "video/mp4",
	"dash": "video/mp4",
	"msdh": "video/mp4",
	"msix": "video/mp4",
	"cmfc": "video/mp4",
	"cmfs": "video/mp4",
	"hevc": "video/mp4",
	"hevx": "video/mp4",
	"hevm": "video/mp4",
	"hevs": "video/mp4",
	"f4v ": "video/mp4",
}

// isoBMFFBrandMIME returns the MIME type implied by the ISO-BMFF ftyp major
// brand — the 4-byte brand at offset 8, after the box size and "ftyp"
// signature at offset 4. Returns "" when the data isn't an ISO-BMFF file or
// the brand isn't registered here (unknown brands keep the octet-stream
// free-pass so newer formats aren't rejected on sight).
func isoBMFFBrandMIME(data []byte) string {
	if len(data) < 12 || string(data[4:8]) != "ftyp" {
		return ""
	}
	return bmffBrandMIME[string(data[8:12])]
}

// declaredMatchesBMFF checks a declared MIME against an ftyp-identified
// ISO-BMFF type. Still-image brands (HEIC/HEIF/AVIF/JXL/JP2) match only an
// honest declaration of that same type — HEIC labeled video/mp4 or
// image/jpeg is caught. Video and audio brands accept any declared type in
// the same family, consistent with the container-compat rules in
// VerifyMIMEType.
func declaredMatchesBMFF(declared, brandMIME string) bool {
	switch {
	case strings.HasPrefix(brandMIME, "image/"):
		// heic and heif are the same HEIF still-image family; tools tag
		// either way.
		return declared == brandMIME ||
			(declared == "image/heic" && brandMIME == "image/heif") ||
			(declared == "image/heif" && brandMIME == "image/heic")
	case strings.HasPrefix(brandMIME, "audio/"):
		return strings.HasPrefix(declared, "audio/")
	default:
		return strings.HasPrefix(declared, "video/")
	}
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
	// identifiable from the header alone — unless the ISO-BMFF ftyp brand
	// identifies it, in which case the brand's real type governs: a HEIC
	// mislabeled video/mp4 is rejected here, while an isom-branded MP4
	// (ffmpeg's default) verifies correctly instead of relying on the
	// free-pass. Truly unidentified binary keeps the free-pass — it may
	// be valid content that can't be fingerprinted (e.g. encrypted).
	if canonical == "application/octet-stream" {
		if brandMIME := isoBMFFBrandMIME(fileBytes); brandMIME != "" {
			return detected, declaredMatchesBMFF(declared, brandMIME), nil
		}
		return detected, true, nil
	}

	if canonical == declared {
		return detected, true, nil
	}

	// ISO base-media family: MP4 and QuickTime share the same container and
	// DetectContentType reports whichever ftyp brand it sees, so a
	// quicktime-declared .mov may sniff as video/mp4 (and vice versa).
	if (canonical == "video/mp4" && declared == "video/quicktime") ||
		(canonical == "video/quicktime" && declared == "video/mp4") {
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

	// Text-based formats (markdown, RTF, SVG…) are indistinguishable to
	// DetectContentType — it reports "text/plain" or "text/xml" for all of
	// them. Accept a declared type from the text-compat set.
	if (canonical == "text/plain" || canonical == "text/xml") && textCompat[declared] {
		return detected, true, nil
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

// contextUploadTypeSetting maps an upload context to the system_settings key
// holding its admin-configured allowed extension list (CSV). Only contexts
// whose setting is declared in the control settings catalog are gated here;
// document uploads (§63 job-application CVs) intentionally are not — an admin
// trimming media-library types must not silently break applications.
var contextUploadTypeSetting = map[UploadContext]string{
	UploadContextMedia:  "content.allowed_upload_types",
	UploadContextAvatar: "users.allowed_avatar_types",
}

// isExtAllowed reports whether canonicalMIME is permitted by a CSV of allowed
// extensions (e.g. "jpg,jpeg,png,webp"). Extension tokens resolve through
// ExtensionToMIME, so "jpg"/"jpeg" both match image/jpeg. Unknown tokens in
// the CSV simply never match anything.
func isExtAllowed(canonicalMIME, allowedCSV string) bool {
	for _, ext := range strings.Split(allowedCSV, ",") {
		ext = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")
		if ext == "" {
			continue
		}
		if m, ok := ExtensionToMIME["."+ext]; ok && CanonicalMIMEType(m) == canonicalMIME {
			return true
		}
	}

	return false
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
