package pagespeed

// Admin PageSpeed proxy (§26): GET returns the cached PSI report, POST /run
// refreshes it asynchronously. The Google API key stays server-side; without
// JC_PAGESPEED_API_KEY the handler reports itself unconfigured so the admin
// UI can show a setup hint instead of failing oddly.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jedafe/japa/internal/valkey"
)

const (
	reportKey  = "jc:pagespeed:report"
	runningKey = "jc:pagespeed:running"
	targetsKey = "jc:pagespeed:targets"
	reportTTL  = 30 * time.Minute
	runningTTL = 15 * time.Minute // covers worst case: MAX_TARGETS × 2 strategies × 60s PSI
	targetsTTL = time.Hour
	psiURL     = "https://www.googleapis.com/pagespeedonline/v5/runPagespeed"
)

type target struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type Handlers struct {
	rdb     *valkey.Client
	apiKey  string
	targets []target
	domain  string // scan endpoint is restricted to this host + its subdomains

	// In-memory fallbacks for when Valkey is unavailable (local/dev only).
	mu          sync.Mutex
	memReport   []byte
	memReportAt time.Time
	memRunning  bool
}

func NewHandlers(rdb *valkey.Client) *Handlers {
	// JC_PAGESPEED_TARGETS overrides the defaults:
	// "main=https://japa.to,docs=https://docs.japa.to/docs,academy=https://academy.japa.to"
	targets := []target{
		{Name: "main", URL: "https://japa.to"},
		{Name: "docs", URL: "https://docs.japa.to/docs"},
		{Name: "academy", URL: "https://academy.japa.to"},
	}
	if raw := os.Getenv("JC_PAGESPEED_TARGETS"); raw != "" {
		var parsed []target
		for _, pair := range strings.Split(raw, ",") {
			if n, u, ok := strings.Cut(strings.TrimSpace(pair), "="); ok && n != "" && u != "" {
				parsed = append(parsed, target{Name: n, URL: u})
			}
		}
		if len(parsed) > 0 {
			targets = parsed
		}
	}
	h := &Handlers{rdb: rdb, apiKey: os.Getenv("JC_PAGESPEED_API_KEY"), targets: targets}
	// The scan endpoint is restricted to the site's host (+subdomains) so it
	// can't be used as an open PSI proxy that burns quota on arbitrary URLs.
	site := os.Getenv("JC_SITE_URL")
	if site == "" {
		site = "https://japa.to"
	}
	if u, err := url.Parse(site); err == nil {
		h.domain = strings.TrimPrefix(u.Hostname(), "www.")
	}
	return h
}

func (h *Handlers) writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": msg})
}

func (h *Handlers) GetReport(w http.ResponseWriter, r *http.Request) {
	if h.apiKey == "" {
		h.writeErr(w, http.StatusServiceUnavailable, "JC_PAGESPEED_API_KEY is not configured")
		return
	}
	if h.rdb != nil {
		if cached, err := h.rdb.Redis().Get(r.Context(), reportKey).Result(); err == nil && cached != "" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write([]byte(cached))
			return
		}
	} else {
		h.mu.Lock()
		if time.Since(h.memReportAt) < reportTTL && len(h.memReport) > 0 {
			cached := h.memReport
			h.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(cached)
			return
		}
		h.mu.Unlock()
	}
	// Report status only — a GET must never trigger an audit: a failed run
	// that leaves nothing cached would turn every poll into a new full audit
	// and stream PSI calls nonstop.
	status := "idle"
	if h.running() {
		status = "running"
	}
	h.writeOK(w, map[string]any{"status": status})
}

func (h *Handlers) running() bool {
	if h.rdb != nil {
		if n, err := h.rdb.Redis().Exists(context.Background(), runningKey).Result(); err == nil {
			return n > 0
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.memRunning
}

// writeOK returns a 200 JSON body with no-store so heuristic caches cannot
// pin a transient status or stale report.
func (h *Handlers) writeOK(w http.ResponseWriter, body map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(body)
}

// trigger kicks off the async audit unless one is already running. Returns
// "started" or "running".
func (h *Handlers) trigger() string {
	if h.rdb != nil {
		if ok, _ := h.rdb.Redis().SetNX(context.Background(), runningKey, 1, runningTTL).Result(); !ok {
			return "running"
		}
	} else {
		h.mu.Lock()
		if h.memRunning {
			h.mu.Unlock()
			return "running"
		}
		h.memRunning = true
		h.mu.Unlock()
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), runningTTL)
		defer cancel()
		defer func() {
			if h.rdb != nil {
				h.rdb.Redis().Del(ctx, runningKey)
			} else {
				h.mu.Lock()
				h.memRunning = false
				h.mu.Unlock()
			}
		}()
		if rep, err := h.audit(ctx); err == nil {
			h.store(ctx, rep)
		}
	}()
	return "started"
}

func (h *Handlers) RunAudit(w http.ResponseWriter, r *http.Request) {
	if h.apiKey == "" {
		h.writeErr(w, http.StatusServiceUnavailable, "JC_PAGESPEED_API_KEY is not configured")
		return
	}
	status := h.trigger()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": status})
}

func (h *Handlers) store(ctx context.Context, rep map[string]any) {
	raw, err := json.Marshal(rep)
	if err != nil {
		return
	}
	if h.rdb != nil {
		_ = h.rdb.Redis().Set(ctx, reportKey, raw, reportTTL).Err()
		return
	}
	h.mu.Lock()
	h.memReport, h.memReportAt = raw, time.Now()
	h.mu.Unlock()
}

func (h *Handlers) audit(ctx context.Context) (map[string]any, error) {
	results := make([]map[string]any, 0, len(h.targets)*2)
	for _, t := range h.allTargets(ctx) {
		for _, strategy := range []string{"mobile", "desktop"} {
			out, err := h.runPsi(ctx, t.URL, strategy)
			if err != nil {
				results = append(results, map[string]any{"target": t.Name, "url": t.URL, "strategy": strategy, "error": err.Error()})
				continue
			}
			results = append(results, map[string]any{"target": t.Name, "url": t.URL, "strategy": strategy, "scores": out})
		}
	}
	return map[string]any{"generated_at": time.Now().UTC().Format(time.RFC3339), "results": results}, nil
}

func (h *Handlers) runPsi(ctx context.Context, pageURL, strategy string) (map[string]any, error) {
	qs := url.Values{"url": {pageURL}, "strategy": {strategy}, "key": {h.apiKey}}
	for _, c := range []string{"performance", "accessibility", "best-practices", "seo"} {
		qs.Add("category", c)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, psiURL+"?"+qs.Encode(), nil)
	if err != nil {
		return nil, err
	}
	res, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("psi: http %d", res.StatusCode)
	}
	var body struct {
		LighthouseResult struct {
			Categories map[string]struct {
				Score *float64 `json:"score"`
			} `json:"categories"`
			Audits map[string]struct {
				DisplayValue string `json:"displayValue"`
			} `json:"audits"`
		} `json:"lighthouseResult"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, err
	}
	if body.Error != nil {
		return nil, fmt.Errorf("psi: %s", body.Error.Message)
	}
	scores := map[string]any{}
	for id, c := range body.LighthouseResult.Categories {
		if c.Score != nil {
			scores[id] = int(*c.Score * 100)
		}
	}
	for _, m := range []string{"first-contentful-paint", "largest-contentful-paint", "total-blocking-time", "cumulative-layout-shift", "speed-index"} {
		if a, ok := body.LighthouseResult.Audits[m]; ok {
			scores[m] = a.DisplayValue
		}
	}
	return scores, nil
}

// allTargets returns the configured entry pages merged with top pages
// discovered from each host's sitemap.xml, deduplicated and capped by
// JC_PAGESPEED_MAX_TARGETS (default 10 — every target costs 2 PSI calls).
// The merged list caches for 1h so audits don't re-fetch every sitemap.
func (h *Handlers) allTargets(ctx context.Context) []target {
	if h.rdb != nil {
		if cached, err := h.rdb.Redis().Get(ctx, targetsKey).Result(); err == nil && cached != "" {
			var out []target
			if json.Unmarshal([]byte(cached), &out) == nil && len(out) > 0 {
				return out
			}
		}
	}
	max := 10
	if v := os.Getenv("JC_PAGESPEED_MAX_TARGETS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			max = n
		}
	}
	seen := map[string]bool{}
	var out []target
	add := func(name, u string) {
		if !seen[u] && len(out) < max {
			seen[u] = true
			out = append(out, target{Name: name, URL: u})
		}
	}
	for _, t := range h.targets {
		add(t.Name, t.URL)
	}
	// Top pages: first URLs of each host's sitemap (sitemaps are ordered by
	// priority/lastmod). One index level of recursion, fail-silent — a sitemap
	// outage must not break the audit.
	for _, t := range h.targets {
		if u, err := url.Parse(t.URL); err == nil {
			for _, page := range sitemapTopPages(ctx, u.Scheme+"://"+u.Host, max-len(out)) {
				add(t.Name, page)
			}
		}
	}
	if h.rdb != nil {
		if raw, err := json.Marshal(out); err == nil {
			_ = h.rdb.Redis().Set(ctx, targetsKey, raw, targetsTTL).Err()
		}
	}
	return out
}

var locRe = regexp.MustCompile(`<loc>\s*([^<]+?)\s*</loc>`)

// sitemapTopPages fetches sitemapBase/sitemap.xml and returns up to limit
// same-host page URLs, recursing one level into sitemap index files.
func sitemapTopPages(ctx context.Context, sitemapBase string, limit int) []string {
	return sitemapLocs(ctx, sitemapBase+"/sitemap.xml", limit, 0)
}

func sitemapLocs(ctx context.Context, smURL string, limit int, depth int) []string {
	if limit <= 0 {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, smURL, nil)
	if err != nil {
		return nil
	}
	res, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil || res.StatusCode != http.StatusOK {
		return nil
	}
	base, _ := url.Parse(sitemapBaseFrom(smURL))
	var out []string
	for _, m := range locRe.FindAllStringSubmatch(string(body), -1) {
		loc := m[1]
		if depth == 0 && strings.HasSuffix(loc, ".xml") {
			for _, s := range sitemapLocs(ctx, loc, limit-len(out), depth+1) {
				out = append(out, s)
				if len(out) >= limit {
					return out
				}
			}
			continue
		}
		u, err := url.Parse(loc)
		if err != nil || base == nil || u.Host != base.Host {
			continue
		}
		p := u.Path
		if strings.Contains(p, "/api/") || strings.Contains(p, "/admin") || strings.Contains(p, "/auth") || strings.HasSuffix(p, ".xml") {
			continue
		}
		out = append(out, loc)
		if len(out) >= limit {
			return out
		}
	}
	return out
}

func sitemapBaseFrom(smURL string) string {
	u, err := url.Parse(smURL)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// Scan handles POST /admin/pagespeed/scan — audits one page URL on demand.
// Restricted to the site's registered domain so the endpoint can't be used
// as an open PSI proxy.
func (h *Handlers) Scan(w http.ResponseWriter, r *http.Request) {
	if h.apiKey == "" {
		h.writeErr(w, http.StatusServiceUnavailable, "JC_PAGESPEED_API_KEY is not configured")
		return
	}
	var body struct {
		URL      string `json:"url"`
		Strategy string `json:"strategy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		h.writeErr(w, http.StatusBadRequest, "url is required")
		return
	}
	u, err := url.Parse(body.URL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		h.writeErr(w, http.StatusBadRequest, "invalid url")
		return
	}
	if h.domain == "" {
		h.writeErr(w, http.StatusServiceUnavailable, "scan domain is not configured")
		return
	}
	d := strings.ToLower(u.Hostname())
	if d != h.domain && !strings.HasSuffix(d, "."+h.domain) {
		h.writeErr(w, http.StatusForbidden, "url must be on "+h.domain+" or a subdomain")
		return
	}
	strategies := []string{"mobile", "desktop"}
	if body.Strategy == "mobile" || body.Strategy == "desktop" {
		strategies = []string{body.Strategy}
	}
	ck := "jc:pagespeed:scan:" + strings.Join(strategies, "+") + ":" + u.Hostname() + ":" + u.EscapedPath() + u.RawQuery
	if h.rdb != nil {
		if cached, err := h.rdb.Redis().Get(r.Context(), ck).Result(); err == nil && cached != "" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(cached))
			return
		}
	}
	// Without a cache there is nowhere to park the result — run synchronously
	// so the caller still gets scores (local/dev path only).
	if h.rdb == nil {
		out := map[string]any{"url": body.URL, "results": map[string]any{}}
		resMap := out["results"].(map[string]any)
		for _, strategy := range strategies {
			if scores, err := h.runPsi(r.Context(), body.URL, strategy); err != nil {
				resMap[strategy] = map[string]any{"error": err.Error()}
			} else {
				resMap[strategy] = scores
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
		return
	}
	// PSI calls take 20-60s each — run async so the request doesn't hold.
	// The endpoint is idempotent: callers re-POST to poll for the result.
	rk := ck + ":running"
	if ok, _ := h.rdb.Redis().SetNX(r.Context(), rk, 1, runningTTL).Result(); !ok {
		h.writeAccepted(w, "running")
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), runningTTL)
		defer cancel()
		defer func() {
			if h.rdb != nil {
				h.rdb.Redis().Del(ctx, rk)
			}
		}()
		out := map[string]any{"url": body.URL, "results": map[string]any{}}
		resMap := out["results"].(map[string]any)
		for _, strategy := range strategies {
			if scores, err := h.runPsi(ctx, body.URL, strategy); err != nil {
				resMap[strategy] = map[string]any{"error": err.Error()}
			} else {
				resMap[strategy] = scores
			}
		}
		if h.rdb != nil {
			if raw, err := json.Marshal(out); err == nil {
				_ = h.rdb.Redis().Set(ctx, ck, raw, reportTTL).Err()
			}
		}
	}()
	h.writeAccepted(w, "started")
}

func (h *Handlers) writeAccepted(w http.ResponseWriter, status string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": status})
}

