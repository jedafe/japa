package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/valkey-io/valkey-go/valkeycompat"
	"golang.org/x/sync/singleflight"
)

const settingsCacheTTL = 5 * time.Minute

const settingsCachePrefix = "jc:settings:"

// Settings provides hot-reloadable, per-platform access to the system_settings
// table with a two-tier cache (in-memory L1 + Valkey L2).
type Settings struct {
	db      *db.DB
	q       *gen.Queries
	qWriter *gen.Queries
	vk      valkeycompat.Cmdable
	mu      sync.RWMutex
	cache   map[string]cachedSetting
	stopCh  chan struct{}
	sf      singleflight.Group
}

type cachedSetting struct {
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	Missing   bool      `json:"missing"`
}

// NewSettings creates a Settings backed by the given database and Valkey client.
// It starts a background cleanup goroutine that must be stopped with Close.
func NewSettings(database *db.DB, vkClient valkeycompat.Cmdable) *Settings {
	s := &Settings{
		db:      database,
		q:       db.NewQueries(database.Reader),
		qWriter: db.NewQueries(database.Writer),
		vk:      vkClient,
		cache:   make(map[string]cachedSetting),
		stopCh:  make(chan struct{}),
	}
	go s.cleanupLoop(60 * time.Second)

	return s
}

// Close stops the background cleanup goroutine. Callers should call this
// during application shutdown to avoid leaking the cleanup goroutine. It is
// safe to call multiple times — subsequent calls are no-ops. It is also safe
// to call on a Settings instance created without NewSettings (stopCh == nil).
func (s *Settings) Close() {
	if s.stopCh == nil {
		return
	}

	select {
	case <-s.stopCh:
		// already closed
	default:
		close(s.stopCh)
	}
}

// resolveCachedEntry translates an in-memory cache entry into a value or a
// not-found error. It returns ok=false when the entry is expired or missing.
func resolveCachedEntry(cached cachedSetting, key string) (string, bool, error) {
	if time.Since(cached.CreatedAt) >= settingsCacheTTL {
		return "", false, nil
	}

	if cached.Missing {
		return "", true, fmt.Errorf("setting %q not found: %w", key, pgx.ErrNoRows)
	}

	return cached.Value, true, nil
}

func (s *Settings) getFromL1(cacheKey, key string) (string, bool, error) {
	s.mu.RLock()
	cached := s.cache[cacheKey]
	s.mu.RUnlock()

	return resolveCachedEntry(cached, key)
}

func (s *Settings) getFromL2(ctx context.Context, cacheKey, key string) (string, bool, error) {
	data, ok := s.valkeyGet(ctx, cacheKey)
	if !ok {
		return "", false, nil
	}

	entry, ok := parseCachedSetting(data)
	if !ok {
		return "", false, nil
	}

	s.setCache(cacheKey, entry)

	return resolveCachedEntry(entry, key)
}

func (s *Settings) valkeyGet(ctx context.Context, cacheKey string) ([]byte, bool) {
	if s.vk == nil {
		return nil, false
	}

	data, err := s.vk.Get(ctx, settingsCachePrefix+cacheKey).Bytes()
	if err != nil {
		return nil, false
	}

	return data, true
}

func parseCachedSetting(data []byte) (cachedSetting, bool) {
	var entry cachedSetting
	if json.Unmarshal(data, &entry) != nil {
		return cachedSetting{}, false
	}

	return entry, true
}

func (s *Settings) loadFromDB(ctx context.Context, key, cacheKey string) (string, error) {
	v, err, _ := s.sf.Do(cacheKey, func() (any, error) {
		if value, ok, err := s.getFromL1(cacheKey, key); ok {
			return value, err
		}

		return s.queryAndCache(ctx, key, cacheKey)
	})
	if err != nil {
		return "", err
	}

	value, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("settings: unexpected singleflight result type %T", v)
	}

	return value, nil
}

func (s *Settings) queryAndCache(ctx context.Context, key, cacheKey string) (any, error) {
	valueBytes, err := s.q.GetSettingValue(ctx, key)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Debug("settings: setting not stored, using default", "key", key)

			entry := cachedSetting{Value: "", CreatedAt: time.Now(), Missing: true}
			s.setCache(cacheKey, entry)
			s.setValkeyCache(ctx, cacheKey, entry)

			return nil, fmt.Errorf("setting %q not found: %w", key, err)
		}

		slog.Warn("settings: DB read failed", "key", key, "error", err)

		return "", fmt.Errorf("setting %q not found: %w", key, err)
	}

	value := string(valueBytes)
	entry := cachedSetting{Value: value, CreatedAt: time.Now()}

	s.setCache(cacheKey, entry)
	s.setValkeyCache(ctx, cacheKey, entry)

	return value, nil
}

func (s *Settings) setCache(cacheKey string, entry cachedSetting) {
	s.mu.Lock()
	s.cache[cacheKey] = entry
	s.mu.Unlock()
}

func (s *Settings) setValkeyCache(ctx context.Context, cacheKey string, entry cachedSetting) {
	if s.vk == nil {
		return
	}

	data, _ := json.Marshal(entry)
	_ = s.vk.Set(ctx, settingsCachePrefix+cacheKey, data, settingsCacheTTL).Err()
}

// GetSetting returns the raw stored value for a platform-scoped system setting.
// Missing settings are cached with a sentinel so repeated lookups do not hit
// the database. The cache is invalidated by Set/Update calls and by the TTL.
func (s *Settings) GetSetting(ctx context.Context, key string) (string, error) {
	// Ref: .jeos/sections/04-multi-tenancy.md (historical; settings are global).
	// ID is kept in the public signature for Phase 1 compatibility and is ignored.
	cacheKey := key

	if value, ok, err := s.getFromL1(cacheKey, key); ok {
		return value, err
	}

	if value, ok, err := s.getFromL2(ctx, cacheKey, key); ok {
		return value, err
	}

	return s.loadFromDB(ctx, key, cacheKey)
}

// GetSettingBool returns the boolean value for a system setting.
func (s *Settings) GetSettingBool(ctx context.Context, key string) (bool, error) {
	val, err := s.GetSetting(ctx, key)
	if err != nil {
		return false, err
	}

	return parseBool(val), nil
}

// parseBool decodes a JSON-encoded system_setting value into a bool.
// Accepts JSON bool (true), JSON string ("true"/"1"), or bare token.
func parseBool(sv string) bool {
	sv = strings.TrimSpace(sv)

	var b bool

	if json.Unmarshal([]byte(sv), &b) == nil {
		return b
	}

	var str string
	if json.Unmarshal([]byte(sv), &str) == nil {
		str = strings.TrimSpace(str)
		return str == "true" || str == "1"
	}

	return sv == "true" || sv == "1"
}

// GetSettingInt returns the integer value for a system setting.
func (s *Settings) GetSettingInt(ctx context.Context, key string) (int, error) {
	val, err := s.GetSetting(ctx, key)
	if err != nil {
		return 0, err
	}

	return parseInt(val)
}

// parseInt decodes a JSON-encoded system_setting value into an int.
// Accepts JSON number (5000), JSON string ("5000"), or bare token.
func parseInt(sv string) (int, error) {
	sv = strings.TrimSpace(sv)

	var n int

	if json.Unmarshal([]byte(sv), &n) == nil {
		return n, nil
	}

	var str string
	if json.Unmarshal([]byte(sv), &str) == nil {
		if m, convErr := strconv.Atoi(strings.TrimSpace(str)); convErr == nil {
			return m, nil
		}
	}

	if m, convErr := strconv.Atoi(sv); convErr == nil {
		return m, nil
	}

	return 0, fmt.Errorf("value is not an integer: %q", sv)
}

// InvalidateSettings clears the L1 and L2 cache for all settings.
// Callers should invoke this after any system_settings write.
func (s *Settings) InvalidateSettings(ctx context.Context) {
	// Ref: .jeos/sections/04-multi-tenancy.md (historical; settings are global).
	// ID is kept in the public signature for Phase 1 compatibility and is ignored.
	s.mu.Lock()
	s.cache = make(map[string]cachedSetting)
	s.mu.Unlock()

	if s.vk != nil {
		var cursor uint64

		for {
			keys, nextCursor, err := s.vk.Scan(ctx, cursor, settingsCachePrefix+"*", 100).Result()
			if err != nil {
				slog.Warn("settings: failed to scan valkey for invalidation", "error", err)
				return
			}

			if len(keys) > 0 {
				_ = s.vk.Del(ctx, keys...).Err()
			}

			cursor = nextCursor
			if cursor == 0 {
				break
			}
		}
	}
}

// GetIntOr returns the integer value for key, or fallback when the key is
// unset, missing, or not parseable as an integer. It never returns an error,
// so call sites read a hot-reloadable operational limit with a safe default
// and unchanged behaviour when the setting has not been configured.
func (s *Settings) GetIntOr(ctx context.Context, key string, fallback int) int {
	v, err := s.GetSettingInt(ctx, key)
	if err != nil {
		return fallback
	}

	return v
}

// GetStringOr returns the string value for key, or fallback when the key is
// unset or missing. It never returns an error. The value is unquoted if it
// is a JSON-encoded string (e.g. "\"https://...\"" → "https://...").
func (s *Settings) GetStringOr(ctx context.Context, key, fallback string) string {
	val, err := s.GetSetting(ctx, key)
	if err != nil || val == "" {
		return fallback
	}

	// system_settings values are stored as JSON-encoded strings —
	// unquote if the raw value is a JSON string (starts and ends with `"`).
	if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
		var unquoted string
		if json.Unmarshal([]byte(val), &unquoted) == nil {
			return unquoted
		}
	}

	return val
}

// GetFloatOr returns the float64 value for key, or fallback when the key is
// unset, missing, or not parseable as a float. It never returns an error.
func (s *Settings) GetFloatOr(ctx context.Context, key string, fallback float64) float64 {
	val, err := s.GetSetting(ctx, key)
	if err != nil {
		return fallback
	}

	f, err := parseFloat(val)
	if err != nil {
		return fallback
	}

	return f
}

// parseFloat decodes a JSON-encoded system_setting value into a float64.
// Accepts JSON number (1.8), JSON string ("1.8"), or bare token.
func parseFloat(sv string) (float64, error) {
	sv = strings.TrimSpace(sv)

	var f float64

	if json.Unmarshal([]byte(sv), &f) == nil {
		return f, nil
	}

	var str string
	if json.Unmarshal([]byte(sv), &str) == nil {
		if v, convErr := strconv.ParseFloat(strings.TrimSpace(str), 64); convErr == nil {
			return v, nil
		}
	}

	if v, convErr := strconv.ParseFloat(sv, 64); convErr == nil {
		return v, nil
	}

	return 0, fmt.Errorf("value is not a float: %q", sv)
}

// GetBoolOr returns the boolean value for key, or fallback when the key is
// unset or missing. It never returns an error.
func (s *Settings) GetBoolOr(ctx context.Context, key string, fallback bool) bool {
	v, err := s.GetSettingBool(ctx, key)
	if err != nil {
		return fallback
	}

	return v
}

// SetSetting upserts a setting value for the given key.
// The is_public flag is preserved on conflict (not reset to false).
func (s *Settings) SetSetting(ctx context.Context, key, value string) error {
	if err := s.qWriter.UpsertSystemSettingPreservePublic(ctx, gen.UpsertSystemSettingPreservePublicParams{
		Key:   key,
		Value: []byte(value),
	}); err != nil {
		return fmt.Errorf("upserting setting %q: %w", key, err)
	}

	s.resolveLogoDims(ctx, key, value)
	s.InvalidateSettings(ctx)

	return nil
}

// logoDimKeys maps a logo URL setting to its dimension siblings — resolved
// from the media row when the URL points at a stored media object.
var logoDimKeys = map[string][2]string{
	"appearance.logo_url":      {"appearance.logo_width", "appearance.logo_height"},
	"appearance.logo_url_dark": {"appearance.logo_dark_width", "appearance.logo_dark_height"},
}

// resolveLogoDims fills the appearance.logo_*_width/height siblings after a
// logo URL write. The URL may be a raw storage path or an absolute URL whose
// path is the storage path — both resolve through GetMediaByStoragePath.
// Best-effort: unresolvable URLs leave the dims untouched.
func (s *Settings) resolveLogoDims(ctx context.Context, key, logoURL string) {
	dims, ok := logoDimKeys[key]
	if !ok {
		return
	}
	// URL cleared or unresolvable → write 0 so stale dims never attribute the
	// previous logo's aspect to a different image (ThemeLogo ignores 0).
	clearDims := func() {
		for _, k := range dims {
			_ = s.qWriter.UpsertSystemSetting(ctx, gen.UpsertSystemSettingParams{
				Key: k, Value: []byte("0"), IsPublic: true,
			})
		}
	}
	if strings.TrimSpace(logoURL) == "" {
		clearDims()
		return
	}
	// value arrives JSON-serialized — string settings carry literal quotes;
	// unwrap so the path lookup sees the bare URL.
	var unquoted string
	if err := json.Unmarshal([]byte(logoURL), &unquoted); err == nil && unquoted != "" {
		logoURL = unquoted
	}
	// Try the raw value, then the URL path, then known storage prefixes —
	// covers raw storage paths, absolute URLs, and CDN/imgproxy URLs.
	candidates := []string{logoURL}
	if u, err := url.Parse(logoURL); err == nil && u.Path != "" {
		candidates = append(candidates, strings.TrimPrefix(u.Path, "/"))
	}
	for _, prefix := range []string{"/uploads/", "/media/"} {
		if idx := strings.Index(logoURL, prefix); idx >= 0 {
			candidates = append(candidates, logoURL[idx+1:])
		}
	}
	// Serve-path values (/api/v1/media/serve/{bucket}/{path}) embed the raw
	// storage path after the bucket segment — strip both so the lookup can
	// match the media row directly.
	if idx := strings.Index(logoURL, "/api/v1/media/serve/"); idx >= 0 {
		if rest := logoURL[idx+len("/api/v1/media/serve/"):]; rest != "" {
			if slash := strings.Index(rest, "/"); slash >= 0 {
				candidates = append(candidates, rest[slash+1:])
			}
		}
	}
	var row gen.Medium
	var err error
	for _, c := range candidates {
		if c == "" {
			continue
		}
		row, err = s.q.GetMediaByStoragePath(ctx, c)
		if err == nil {
			break
		}
	}
	if err != nil {
		// Not a platform media object — try fetching dims directly off the
		// external URL (SSRF-bounded); only clear when that fails too.
		if w, h, ok := fetchExternalLogoDims(logoURL); ok {
			for i, v := range []int{w, h} {
				_ = s.qWriter.UpsertSystemSetting(ctx, gen.UpsertSystemSettingParams{
					Key: dims[i], Value: []byte(fmt.Sprintf("%d", v)), IsPublic: true,
				})
			}
			return
		}
		clearDims()
		return
	}
	if row.WidthPx == nil || row.HeightPx == nil || *row.WidthPx <= 0 || *row.HeightPx <= 0 {
		clearDims()
		return
	}
	for i, v := range []int{int(*row.WidthPx), int(*row.HeightPx)} {
		_ = s.qWriter.UpsertSystemSetting(ctx, gen.UpsertSystemSettingParams{
			Key: dims[i], Value: []byte(fmt.Sprintf("%d", v)), IsPublic: true,
		})
	}
}

// externalLogoHTTPClient fetches logo headers off admin-supplied external
// URLs — every dial is IP-validated so the fetch can't reach private ranges
// (SSRF). Same pattern as the media remote proxy; kept local because
// internal/media already imports this package.
var externalLogoHTTPClient = &http.Client{
	Timeout: 5 * time.Second,
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("logo dims: split host: %w", err)
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("logo dims: resolving %s: %w", host, err)
			}
			for _, ip := range ips {
				if !isPublicIP(ip.IP) {
					return nil, fmt.Errorf("logo dims: %s resolves to non-public IP %s", host, ip)
				}
			}
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		},
	},
}

// isPublicIP returns false for loopback, private, link-local, unspecified,
// and multicast addresses — every dial above is guarded by it.
func isPublicIP(ip net.IP) bool {
	return ip.IsGlobalUnicast() && !ip.IsLoopback() && !ip.IsPrivate() &&
		!ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsUnspecified()
}

// fetchExternalLogoDims reads a logo image's intrinsic dims via
// image.DecodeConfig — headers only, body bounded at 512KB.
func fetchExternalLogoDims(rawURL string) (w, h int, ok bool) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil) // #nosec G704 -- every dial is IP-validated above
	if err != nil {
		return 0, 0, false
	}
	req.Header.Set("User-Agent", "JapaConnect-LogoDims/1.0")
	req.Header.Set("Accept", "image/*")
	resp, err := externalLogoHTTPClient.Do(req)
	if err != nil {
		return 0, 0, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, 0, false
	}
	cfg, _, err := image.DecodeConfig(io.LimitReader(resp.Body, 512*1024))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0, false
	}
	return cfg.Width, cfg.Height, true
}

// UpdateSettingsBatch atomically upserts multiple settings in a
// single transaction. Each entry in settings maps a full dot-notation key
// (e.g. "site.name") to its already-serialized JSON value. The cache is
// invalidated once after commit so callers do not need to invalidate per key.
// is_public is preserved on conflict (not overwritten).
func (s *Settings) UpdateSettingsBatch(ctx context.Context, settings map[string][]byte) error {
	err := db.WithTx(ctx, s.db.Writer, func(q *gen.Queries) error {
		for key, valueJSON := range settings {
			if err := q.UpsertSystemSettingPreservePublic(ctx, gen.UpsertSystemSettingPreservePublicParams{
				Key:   key,
				Value: valueJSON,
			}); err != nil {
				return fmt.Errorf("upserting setting %q: %w", key, err)
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	for key, valueJSON := range settings {
		s.resolveLogoDims(ctx, key, string(valueJSON))
	}
	s.InvalidateSettings(ctx)

	return nil
}

// UpdateSettingsBatchWithPublic is like UpdateSettingsBatch but also sets the
// is_public flag on each upserted row (used by the admin handler so appearance
// settings are exposed via GET /site/settings/public). On conflict, is_public
// is updated to the new value so toggling publicity takes effect immediately.
func (s *Settings) UpdateSettingsBatchWithPublic(ctx context.Context, settings map[string][]byte, isPublic map[string]bool) error {
	err := db.WithTx(ctx, s.db.Writer, func(q *gen.Queries) error {
		for key, valueJSON := range settings {
			pub := isPublic[key]
			if err := q.UpsertSystemSetting(ctx, gen.UpsertSystemSettingParams{
				Key:      key,
				Value:    valueJSON,
				IsPublic: pub,
			}); err != nil {
				return fmt.Errorf("upserting setting %q: %w", key, err)
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	for key, valueJSON := range settings {
		s.resolveLogoDims(ctx, key, string(valueJSON))
	}
	s.InvalidateSettings(ctx)

	return nil
}

// DeleteSettingsBatch atomically removes multiple settings rows in a single
// transaction. Keys are full dot-notation (e.g. "appearance.primary_color").
// Removing a row restores the catalog default at read time — this is how a
// settings-category reset is implemented. The cache is invalidated once after
// commit so callers do not need to invalidate per key.
func (s *Settings) DeleteSettingsBatch(ctx context.Context, keys []string) error {
	err := db.WithTx(ctx, s.db.Writer, func(q *gen.Queries) error {
		for _, key := range keys {
			if err := q.DeleteSystemSetting(ctx, key); err != nil {
				return fmt.Errorf("deleting setting %q: %w", key, err)
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	// Deleting a logo URL clears its dimension siblings (empty URL → clearDims).
	for _, key := range keys {
		s.resolveLogoDims(ctx, key, "")
	}
	s.InvalidateSettings(ctx)

	return nil
}

// GetPublicSettings returns all settings marked as public.
func (s *Settings) GetPublicSettings(ctx context.Context) (map[string]any, error) {
	rows, err := s.q.GetPublicSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("querying public settings: %w", err)
	}

	settings := make(map[string]any, len(rows))

	// JC_API_PUBLIC_URL rewrites relative /api/v1/... values to absolute so
	// cross-origin frontends (admin/control/academy on separate hosts, which
	// don't proxy /api) resolve stored media paths on the API host. Empty
	// keeps dev's same-origin relative paths. Mirrors media.ServeURL.
	apiBase := strings.TrimRight(os.Getenv("JC_API_PUBLIC_URL"), "/")

	for _, row := range rows {
		var parsed any
		if json.Unmarshal(row.Value, &parsed) == nil {
			settings[row.Key] = parsed
		} else {
			settings[row.Key] = string(row.Value)
		}
		if apiBase != "" {
			settings[row.Key] = absolutizeAPIPaths(settings[row.Key], apiBase)
		}
	}

	return settings, nil
}

// absolutizeAPIPaths prefixes relative /api/v1/... string values with the
// API origin, recursing into arrays and objects so nested values (e.g.
// auth brand-slide image URLs) are rewritten too. Non-path strings and
// already-absolute URLs pass through unchanged.
func absolutizeAPIPaths(v any, base string) any {
	switch t := v.(type) {
	case string:
		if strings.HasPrefix(t, "/api/v1/") {
			return base + t
		}
		return t
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = absolutizeAPIPaths(item, base)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, item := range t {
			out[k] = absolutizeAPIPaths(item, base)
		}
		return out
	default:
		return v
	}
}

// cleanupLoop periodically evicts expired L1 cache entries to prevent memory leaks.
// It exits when Close() is called so the goroutine does not leak.
func (s *Settings) cleanupLoop(maxAge time.Duration) {
	ticker := time.NewTicker(maxAge)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			now := time.Now()

			s.mu.Lock()
			for key, entry := range s.cache {
				if now.Sub(entry.CreatedAt) > settingsCacheTTL {
					delete(s.cache, key)
				}
			}
			s.mu.Unlock()
		}
	}
}
