package media

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// ImageProxyOpts controls how imgproxy resizes and formats the image.
type ImageProxyOpts struct {
	ResizingType string // "fill", "fit", "auto"
	Width        int
	Height       int
	Format       string // "webp", "png", "jpg"
}

// cdnBaseURLResolver returns the admin-configurable CDN base URL
// (system_settings key `media.cdn.base_url`). When non-empty it overrides
// JC_IMGPROXY_BASE_URL so an operator can front imgproxy with Cloudflare
// without a redeploy (Section 34 — CDN & S3-Compatible Object Storage).
// The hook is wired once at server startup via SetCDNBaseURLResolver.
var cdnBaseURLResolver func() string

// SetCDNBaseURLResolver wires the system_settings-backed resolver for
// `media.cdn.base_url`. Called once during server wiring. Passing nil
// restores the env-var-only behaviour.
func SetCDNBaseURLResolver(fn func() string) {
	cdnBaseURLResolver = fn
}

// mediaBaseURL resolves the imgproxy/CDN base URL per Section 34:
//  1. system_settings `media.cdn.base_url` (admin-configurable, hot-reloadable)
//  2. JC_IMGPROXY_BASE_URL env var (server-only fallback)
//
// Rule 3 — no hardcoded config. The settings hook is optional; when unset
// the env var is used, preserving backward compatibility.
func mediaBaseURL() string {
	if cdnBaseURLResolver != nil {
		if cdnURL := cdnBaseURLResolver(); cdnURL != "" {
			return cdnURL
		}
	}

	return os.Getenv("JC_IMGPROXY_BASE_URL")
}

// mediaURLKeySalt caches the hex-decoded imgproxy key and salt. Env vars
// are immutable at runtime so the decode is done once via sync.Once
// (BUG-008: avoids re-decoding on every MediaURL call in a batch).
var (
	keySaltOnce sync.Once
	keySalt     struct {
		keyBytes, saltBytes []byte
		valid               bool
	}
)

// loadKeySalt decodes JC_IMGPROXY_KEY and JC_IMGPROXY_SALT once and caches
// the result. Subsequent calls return the cached bytes without re-reading
// env vars or re-decoding hex.
func loadKeySalt() ([]byte, []byte, bool) {
	keySaltOnce.Do(func() {
		key := os.Getenv("JC_IMGPROXY_KEY")
		salt := os.Getenv("JC_IMGPROXY_SALT")

		if key == "" || salt == "" {
			keySalt.valid = false
			return
		}

		kb, err := hex.DecodeString(key)
		if err != nil {
			slog.Error("imgproxy: JC_IMGPROXY_KEY is not valid hex — falling back to local serve",
				"error", err)

			keySalt.valid = false

			return
		}

		sb, err := hex.DecodeString(salt)
		if err != nil {
			slog.Error("imgproxy: JC_IMGPROXY_SALT is not valid hex — falling back to local serve",
				"error", err)

			keySalt.valid = false

			return
		}

		keySalt.keyBytes = kb
		keySalt.saltBytes = sb
		keySalt.valid = true
	})

	return keySalt.keyBytes, keySalt.saltBytes, keySalt.valid
}

// mediaURLConfig is the resolved imgproxy configuration for a batch of
// MediaURL calls. The key/salt are decoded once (env vars are immutable);
// the baseURL is resolved fresh per batch because it may be hot-reloaded
// from system_settings (media.cdn.base_url).
type mediaURLConfig struct {
	keyBytes  []byte
	saltBytes []byte
	baseURL   string
	valid     bool
}

// loadMediaURLConfig resolves the imgproxy config once for a batch of
// MediaURL calls (BUG-008: avoids re-reading env vars and re-decoding hex
// on every item in a list response).
func loadMediaURLConfig() mediaURLConfig {
	kb, sb, valid := loadKeySalt()

	return mediaURLConfig{
		keyBytes:  kb,
		saltBytes: sb,
		baseURL:   mediaBaseURL(),
		valid:     valid,
	}
}

// mediaURLWithConfig generates a signed imgproxy URL using a pre-resolved
// config. Used by batch helpers (toMediaResponses) to amortize env var
// reads and hex decoding across all items in a list (BUG-008).
func mediaURLWithConfig(storageBucket, storagePath string, opts ImageProxyOpts, cfg mediaURLConfig) string {
	if !cfg.valid || cfg.baseURL == "" {
		return ServeURL(storageBucket, storagePath)
	}

	src := "s3://" + storageBucket + "/" + storagePath
	encoded := base64.RawURLEncoding.EncodeToString([]byte(src))

	resizingType := opts.ResizingType
	if resizingType == "" {
		resizingType = "fill"
	}

	width := opts.Width
	if width == 0 {
		width = 640
	}

	height := opts.Height
	if height == 0 {
		resizingType = "auto"
		height = 0
	}

	format := opts.Format
	if format == "" {
		format = "webp"
	}

	// Base64 source form: /{opts}/{base64url}.{ext} — the "plain/" prefix
	// takes a percent-escaped URL, not base64, so wrapping base64 in
	// plain/ makes imgproxy parse a schemeless URL ("unknown scheme").
	path := fmt.Sprintf("/rs:%s:%d:%d/%s.%s",
		resizingType, width, height, encoded, format)

	mac := hmac.New(sha256.New, cfg.keyBytes)
	mac.Write(append(cfg.saltBytes, []byte(path)...))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return cfg.baseURL + "/" + sig + path
}

// MediaURL generates an imgproxy HMAC-SHA256 signed URL for a media item.
// Called on every serialization — NEVER cached in DB.
// Falls back to a local serving endpoint when imgproxy is not configured
// or when the key/salt are invalid hex (security: never sign with an empty key).
func MediaURL(storageBucket, storagePath string, opts ImageProxyOpts) string { //nolint:revive // stuttering type name preserved for API compatibility
	return mediaURLWithConfig(storageBucket, storagePath, opts, loadMediaURLConfig())
}

// ServeURL returns the same-origin serving endpoint for a media item
// (GET /api/v1/media/serve/{bucket}/{path}, Section 34). Used for binaries
// imgproxy cannot transform — videos, audio, PDFs — where the imgproxy
// pipeline would corrupt or reject the payload.
func ServeURL(storageBucket, storagePath string) string {
	// JC_API_PUBLIC_URL makes emitted URLs absolute so cross-origin frontends
	// (admin/control/academy on separate hosts) resolve media on the API host
	// instead of their own origin. Empty keeps dev's relative same-origin path.
	base := strings.TrimRight(os.Getenv("JC_API_PUBLIC_URL"), "/")
	return base + "/api/v1/media/serve/" + storageBucket + "/" + storagePath
}

// UploadURL returns the API-side upload endpoint for a pending media object
// (PUT /seaweedfs/upload/{bucket}/{path}). The client always PUTs raw bytes
// to this same-origin path — SeaweedFSUpload then streams them to S3 when
// configured or writes local disk otherwise. Emitting the object-store
// endpoint directly would hand browsers a Docker-internal hostname they
// cannot resolve; the API proxy keeps uploads working in every deployment.
// JC_API_PUBLIC_URL makes the URL absolute for cross-origin frontends;
// empty keeps dev's relative same-origin path.
func UploadURL(storageBucket, storagePath string) string {
	base := strings.TrimRight(os.Getenv("JC_API_PUBLIC_URL"), "/")
	return base + "/seaweedfs/upload/" + storageBucket + "/" + storagePath
}

// URLFromParts resolves a displayable URL from a nullable storage bucket/path
// pair (typically the right side of a LEFT JOIN to media). Returns nil when
// either part is absent so consumers serialize the field as omitted.
func URLFromParts(storageBucket, storagePath *string, opts ImageProxyOpts) *string {
	if storageBucket == nil || storagePath == nil || *storageBucket == "" || *storagePath == "" {
		return nil
	}

	u := MediaURL(*storageBucket, *storagePath, opts)

	return &u
}

// ServeURLFromParts is URLFromParts for non-image media — it always returns
// the local serve endpoint, never an imgproxy URL.
func ServeURLFromParts(storageBucket, storagePath *string) *string {
	if storageBucket == nil || storagePath == nil || *storageBucket == "" || *storagePath == "" {
		return nil
	}

	u := ServeURL(*storageBucket, *storagePath)

	return &u
}
