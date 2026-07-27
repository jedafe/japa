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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/valkey"
)

// MaxRenderInputSize is the maximum size in bytes of ProseMirror JSON
// accepted by the renderer. Larger payloads are rejected.
const MaxRenderInputSize = 512 * 1024 // 512 KiB

// ScriptRootEnv is the env var that overrides the allowlist root directory
// for the Bun SSR script. Defaults to the repo root ("./scripts/tiptap-ssr").
const ScriptRootEnv = "JC_RENDERER_SCRIPT_ROOT"

// defaultScriptRoot is the allowlist root used when JC_RENDERER_SCRIPT_ROOT
// is unset. Resolved against the process working directory.
const defaultScriptRoot = "scripts/tiptap-ssr"

// ErrInvalidScriptPath is returned when the configured script path escapes
// the allowlist root or is not a regular .js/.ts file.
var ErrInvalidScriptPath = errors.New("renderer: script path must be a .js/.ts file within the allowlist root")

// Renderer converts ProseMirror JSON to HTML via TipTap SSR.
type Renderer struct {
	scriptPath string
	cache      *valkey.Client
	cacheTTL   time.Duration
}

// New creates a Renderer that shells out to the given Bun script.
// The cache is used to store rendered HTML keyed by content_id:updated_at.
// Pass nil for cache to disable caching.
//
// The scriptPath is validated against an allowlist root (JC_RENDERER_SCRIPT_ROOT
// or "./scripts/tiptap-ssr" by default): it must be relative, must not contain
// ".." traversal, and must resolve inside the root. Absolute paths are rejected.
// This prevents path traversal even if the admin config is compromised.
func New(scriptPath string, cache *valkey.Client) (*Renderer, error) {
	cleaned, err := validateScriptPath(scriptPath)
	if err != nil {
		return nil, err
	}
	return &Renderer{
		scriptPath: cleaned,
		cache:      cache,
		cacheTTL:   24 * time.Hour,
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
// The input is validated before being piped to the external process.
func (r *Renderer) renderViaBun(ctx context.Context, json []byte) (string, error) {
	if err := validateRenderInput(json); err != nil {
		return "", err
	}

	// scriptPath was validated against the allowlist root in New(); no
	// gosec suppression needed here.
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
// Format: jc:render:{content_id}:{updated_at_hash}.
func (r *Renderer) cacheKey(contentID pgtype.UUID, updatedAt pgtype.Timestamptz) string {
	ts := updatedAt.Time.Unix()
	hash := sha256.Sum256([]byte(strconv.FormatInt(ts, 10)))
	tsHash := hex.EncodeToString(hash[:8])

	return fmt.Sprintf("jc:render:%s:%s", contentID.String(), tsHash)
}
