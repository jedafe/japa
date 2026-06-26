// Package renderer provides server-side rendering of ProseMirror JSON
// to HTML via TipTap (Section 15).
//
// The renderer shells out to Bun to run a TipTap SSR script. Results
// are cached in Valkey keyed by content_id:updated_at to avoid
// re-rendering unchanged content.
package renderer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/valkey"
)

// Renderer converts ProseMirror JSON to HTML via TipTap SSR.
type Renderer struct {
	scriptPath string
	cache      *valkey.Client
	cacheTTL   time.Duration
}

// New creates a Renderer that shells out to the given Bun script.
// The cache is used to store rendered HTML keyed by content_id:updated_at.
// Pass nil for cache to disable caching.
func New(scriptPath string, cache *valkey.Client) *Renderer {
	return &Renderer{
		scriptPath: scriptPath,
		cache:      cache,
		cacheTTL:   24 * time.Hour,
	}
}

// Render converts ProseMirror JSON to HTML. If the result is cached,
// it returns the cached version. Otherwise, it shells out to Bun.
func (r *Renderer) Render(ctx context.Context, contentID pgtype.UUID, updatedAt pgtype.Timestamptz, json []byte) (string, error) {
	// Try cache first
	if r.cache != nil {
		cacheKey := r.cacheKey(contentID, updatedAt)
		if cached, err := r.cache.Redis().Get(ctx, cacheKey).Result(); err == nil {
			return cached, nil
		}
	}

	// Shell out to Bun
	html, err := r.renderViaBun(ctx, json)
	if err != nil {
		return "", fmt.Errorf("rendering via Bun: %w", err)
	}

	// Cache the result (best-effort — failure does not break the response)
	if r.cache != nil {
		cacheKey := r.cacheKey(contentID, updatedAt)
		_ = r.cache.Redis().Set(ctx, cacheKey, html, r.cacheTTL).Err()
	}

	return html, nil
}

// renderViaBun shells out to Bun to run the TipTap SSR script.
func (r *Renderer) renderViaBun(ctx context.Context, json []byte) (string, error) {
	cmd := exec.CommandContext(ctx, "bun", "run", r.scriptPath)
	cmd.Stdin = bytes.NewReader(json)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("bun script failed: %w, stderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// cacheKey generates a Valkey cache key for rendered HTML.
// Format: jc:render:{content_id}:{updated_at_hash}
func (r *Renderer) cacheKey(contentID pgtype.UUID, updatedAt pgtype.Timestamptz) string {
	ts := updatedAt.Time.Unix()
	hash := sha256.Sum256([]byte(fmt.Sprintf("%d", ts)))
	tsHash := hex.EncodeToString(hash[:8])

	return fmt.Sprintf("jc:render:%s:%s", contentID.String(), tsHash)
}
