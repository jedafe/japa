package media

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
)

// ImageProxyOpts controls how imgproxy resizes and formats the image.
type ImageProxyOpts struct {
	ResizingType string // "fill", "fit", "auto"
	Width        int
	Height       int
	Format       string // "webp", "png", "jpg"
}

// MediaURL generates an imgproxy HMAC-SHA256 signed URL for a media item.
// Called on every serialization — NEVER cached in DB.
// Falls back to a local serving endpoint when imgproxy is not configured
// or when the key/salt are invalid hex (security: never sign with an empty key).
func MediaURL(storageBucket, storagePath string, opts ImageProxyOpts) string { //nolint:revive // stuttering type name preserved for API compatibility
	key := os.Getenv("JC_IMGPROXY_KEY")
	salt := os.Getenv("JC_IMGPROXY_SALT")
	baseURL := os.Getenv("JC_IMGPROXY_BASE_URL")

	// Fallback to local serving endpoint when imgproxy is not configured.
	if key == "" || salt == "" || baseURL == "" {
		return "/api/v1/media/serve/" + storageBucket + "/" + storagePath
	}

	keyBytes, err := hex.DecodeString(key)
	if err != nil {
		slog.Error("imgproxy: JC_IMGPROXY_KEY is not valid hex — falling back to local serve",
			"error", err)
		return "/api/v1/media/serve/" + storageBucket + "/" + storagePath
	}
	saltBytes, err := hex.DecodeString(salt)
	if err != nil {
		slog.Error("imgproxy: JC_IMGPROXY_SALT is not valid hex — falling back to local serve",
			"error", err)
		return "/api/v1/media/serve/" + storageBucket + "/" + storagePath
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

	path := fmt.Sprintf("/rs:%s:%d:%d/plain/%s@%s",
		resizingType, width, height, encoded, format)

	mac := hmac.New(sha256.New, keyBytes)
	mac.Write(append(saltBytes, []byte(path)...))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return baseURL + "/" + sig + path
}
