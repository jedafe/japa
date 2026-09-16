// Package renderer provides server-side rendering of ProseMirror JSON
// to HTML via TipTap (Section 15).
// // The renderer shells out to Bun to run a TipTap SSR script. Results
// are cached in Valkey keyed by content_id:updated_at to avoid
// re-rendering unchanged content.
package renderer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/valkey"
)

// MaxRenderInputSize is the maximum size in bytes of ProseMirror JSON
// accepted by the renderer. Larger payloads are rejected.
const MaxRenderInputSize = 512 * 1024 // 512 KiB

// ScriptRootEnv is the env var that overrides the allowlist root directory
// for the Bun SSR script. Defaults to the repo root ("./scripts/tiptap-ssr").
const ScriptRootEnv = "JC_RENDERER_SCRIPT_ROOT"

// CacheTTLEnv is the env var that overrides the render cache TTL (Go duration
// string, e.g. "12h", "30m"). Defaults to 24h per Section 15 / valkey catalog.
const CacheTTLEnv = "JC_RENDERER_CACHE_TTL"

// TimeoutEnv is the env var that overrides the per-render Bun subprocess
// timeout (Go duration string, e.g. "10s", "30s"). Defaults to 10s. Prevents
// a hanging SSR script from tying up a request goroutine indefinitely.
const TimeoutEnv = "JC_RENDERER_TIMEOUT"

// MaxConcurrentEnv is the env var that overrides the maximum number of
// concurrent Bun subprocesses. Defaults to 8. Prevents resource exhaustion
// under burst load.
const MaxConcurrentEnv = "JC_RENDERER_MAX_CONCURRENT"

// defaultCacheTTL is the render cache TTL used when JC_RENDERER_CACHE_TTL is
// unset. Matches the valkey-keys-catalog entry (24hr).
const defaultCacheTTL = 24 * time.Hour

// defaultTimeout is the per-render Bun subprocess timeout used when
// JC_RENDERER_TIMEOUT is unset.
const defaultTimeout = 10 * time.Second

// defaultMaxConcurrent is the maximum number of concurrent Bun subprocesses
// used when JC_RENDERER_MAX_CONCURRENT is unset.
const defaultMaxConcurrent = 8

// defaultScriptRoot is the allowlist root used when JC_RENDERER_SCRIPT_ROOT
// is unset. Resolved against the process working directory.
const defaultScriptRoot = "scripts/tiptap-ssr"

// ErrInvalidScriptPath is returned when the configured script path escapes
// the allowlist root or is not a regular .js/.ts file.
var ErrInvalidScriptPath = errors.New("renderer: script path must be a .js/.ts file within the allowlist root")

// Renderer converts ProseMirror JSON to HTML via TipTap SSR.
type Renderer struct {
	scriptPath        string
	cache             *valkey.Client
	cacheTTL          time.Duration
	timeout           time.Duration
	sem               chan struct{}
	couponResolver    CouponResolver
	affiliateResolver AffiliateLinkResolver
}

// SetCouponResolver injects the coupon resolver for render-time coupon_embed
// resolution (Section 55 P4). When nil, coupon_embed placeholders are left
// as-is.
func (r *Renderer) SetCouponResolver(cr CouponResolver) { r.couponResolver = cr }

// SetAffiliateLinkResolver injects the affiliate link resolver for render-time
// href rewriting (Section 55 P4). When nil, affiliate link hrefs are left
// as-is.
func (r *Renderer) SetAffiliateLinkResolver(alr AffiliateLinkResolver) { r.affiliateResolver = alr }

// New creates a Renderer that shells out to the given Bun script.
// The cache is used to store rendered HTML keyed by content_id:updated_at.
// Pass nil for cache to disable caching.
// // The scriptPath is validated against an allowlist root (JC_RENDERER_SCRIPT_ROOT
// or "./scripts/tiptap-ssr" by default): it must be relative, must not contain
// ".." traversal, and must resolve inside the root. Absolute paths are rejected.
// This prevents path traversal even if the admin config is compromised.
func New(scriptPath string, cache *valkey.Client) (*Renderer, error) {
	cleaned, err := validateScriptPath(scriptPath)
	if err != nil {
		return nil, err
	}

	ttl := defaultCacheTTL

	if env := strings.TrimSpace(os.Getenv(CacheTTLEnv)); env != "" {
		if parsed, err := time.ParseDuration(env); err == nil && parsed > 0 {
			ttl = parsed
		} else {
			slog.Warn("invalid JC_RENDERER_CACHE_TTL, using default", "value", env, "default", defaultCacheTTL)
		}
	}

	timeout := defaultTimeout

	if env := strings.TrimSpace(os.Getenv(TimeoutEnv)); env != "" {
		if parsed, err := time.ParseDuration(env); err == nil && parsed > 0 {
			timeout = parsed
		} else {
			slog.Warn("invalid JC_RENDERER_TIMEOUT, using default", "value", env, "default", defaultTimeout)
		}
	}

	maxConcurrent := defaultMaxConcurrent

	if env := strings.TrimSpace(os.Getenv(MaxConcurrentEnv)); env != "" {
		if parsed, err := strconv.Atoi(env); err == nil && parsed > 0 {
			maxConcurrent = parsed
		} else {
			slog.Warn("invalid JC_RENDERER_MAX_CONCURRENT, using default", "value", env, "default", defaultMaxConcurrent)
		}
	}

	return &Renderer{
		scriptPath: cleaned,
		cache:      cache,
		cacheTTL:   ttl,
		timeout:    timeout,
		sem:        make(chan struct{}, maxConcurrent),
	}, nil
}

// validateScriptPath ensures the script path is a relative .js/.ts file that
// resolves inside the allowlist root (JC_RENDERER_SCRIPT_ROOT or
// defaultScriptRoot). Returns the cleaned, root-anchored absolute path.
func validateScriptPath(scriptPath string) (string, error) {
	if scriptPath == "" {
		return "", ErrInvalidScriptPath
	}
	// Reject absolute paths — the script must live under the allowlist root.
	if filepath.IsAbs(scriptPath) {
		return "", ErrInvalidScriptPath
	}
	// Reject any ".." segment — no traversal above the root.
	for _, seg := range strings.Split(filepath.ToSlash(scriptPath), "/") {
		if seg == ".." {
			return "", ErrInvalidScriptPath
		}
	}
	// Must be a .js or .ts file.
	ext := strings.ToLower(filepath.Ext(scriptPath))
	if ext != ".js" && ext != ".ts" {
		return "", ErrInvalidScriptPath
	}

	root := defaultScriptRoot
	if env := strings.TrimSpace(os.Getenv(ScriptRootEnv)); env != "" {
		root = env
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("renderer: resolving script root: %w", err)
	}

	absScript := filepath.Join(absRoot, filepath.Clean(scriptPath))

	rel, err := filepath.Rel(absRoot, absScript)
	if err != nil || strings.HasPrefix(filepath.ToSlash(rel), "../") {
		return "", ErrInvalidScriptPath
	}

	return absScript, nil
}

// Render converts ProseMirror JSON to HTML. If the result is cached,
// it returns the cached version. Otherwise, it shells out to Bun.
// // The ID scopes the Valkey cache key so one platform cannot read
// another's rendered fragment (§4.2). The output is sanitized through
// the shared content.SanitizeHTML allowlist before caching/returning to
// prevent stored XSS from raw embed HTML or crafted node attributes
// (§5.1, §14 — Renderer must allowlist tags).
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

	// Sanitize through the shared allowlist-based sanitizer (§5.1/§14).
	// This strips <script>, event handlers, javascript: URIs, and any
	// tags/attributes not in the content.SanitizeHTML allowlist — including
	// raw html injected by the platformEmbed node.
	htmlOut := content.SanitizeHTML(html)

	// Render-time resolution of coupon_embed placeholders and affiliate link
	// href rewriting (Section 55 P4). This runs AFTER sanitization so the
	// injected coupon widget HTML and rewritten hrefs are safe. Best-effort:
	// resolution failures are logged and the original placeholder is left.
	htmlOut = resolveCouponEmbedsAndAffiliateLinks(ctx, htmlOut, r.couponResolver, r.affiliateResolver)

	// Cache the result (best-effort — failure does not break the response)
	if r.cache != nil {
		cacheKey := r.cacheKey(contentID, updatedAt)
		_ = r.cache.Redis().Set(ctx, cacheKey, htmlOut, r.cacheTTL).Err()
	}

	return htmlOut, nil
}

// renderViaBun shells out to Bun to run the TipTap SSR script.
// The input is validated before being piped to the external process.
// A dedicated timeout (JC_RENDERER_TIMEOUT, default 10s) bounds the
// subprocess so a hanging SSR script cannot tie up the request goroutine.
// A concurrency semaphore (JC_RENDERER_MAX_CONCURRENT, default 8) caps
// the number of simultaneous Bun processes to prevent resource exhaustion.
func (r *Renderer) renderViaBun(ctx context.Context, json []byte) (string, error) {
	if err := validateRenderInput(json); err != nil {
		return "", err
	}

	// Acquire a concurrency slot. On context cancellation, return early.
	select {
	case r.sem <- struct{}{}:
		defer func() { <-r.sem }()
	case <-ctx.Done():
		return "", ctx.Err()
	}

	// Apply a dedicated render timeout so a hanging Bun process is killed
	// even if the request context has no deadline.
	renderCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	// scriptPath was validated against the allowlist root in New(); no
	// gosec suppression needed here.
	cmd := exec.CommandContext(renderCtx, "bun", "run", r.scriptPath) // #nosec
	cmd.Stdin = bytes.NewReader(json)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("bun script failed: %w, stderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// validateRenderInput checks that the supplied ProseMirror JSON is well-formed
// and within size limits before passing it to an external process.
func validateRenderInput(data []byte) error {
	if len(data) > MaxRenderInputSize {
		return fmt.Errorf("render input exceeds maximum size of %d bytes", MaxRenderInputSize)
	}

	if len(data) == 0 {
		return errors.New("render input is empty")
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("render input is not valid JSON: %w", err)
	}

	docType, ok := doc["type"].(string)
	if !ok || docType != "doc" {
		return fmt.Errorf("render input must have root \"type\": \"doc\", got %q", docType)
	}

	if _, ok := doc["content"]; !ok {
		return errors.New("render input is missing required \"content\" field")
	}

	return nil
}

// cacheKey generates a Valkey cache key for rendered HTML.
// Format: jc:render:{}:{content_id}:{updated_at_hash}. The
// scope prevents cross-platform cache bleed (§4.2). Uses the
// full SHA-256 hash to avoid collision risk from truncation (STD-003).
func (r *Renderer) cacheKey(contentID pgtype.UUID, updatedAt pgtype.Timestamptz) string {
	ts := updatedAt.Time.Unix()
	hash := sha256.Sum256([]byte(strconv.FormatInt(ts, 10)))
	tsHash := hex.EncodeToString(hash[:])

	return valkey.MustKey("render::%s:%s", contentID.String(), tsHash)
}
