package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"

	"github.com/jedafe/japa/internal/intelligence/fetcher"
	"github.com/jedafe/japa/internal/intelligence/sources"
)

// ---------------------------------------------------------------------------
// intel:xhr_sniff — Phase-2 headless XHR/fetch capture (channel design §3).
// POSTs {intel.render.service_url}/har with the target URL; the render
// service drives a headless pass and returns a HAR of the page's network
// activity. Entries whose response is JSON become `api` channel proposals
// materialized on the source (origin='probe' — the admin reviews them in
// the channels editor like any probe proposal).
//
// Enqueued by intel:probe_verify when the report flags requires_js (a SPA
// whose API only appears under a browser) and by admin crawl-now on an
// xhr_sniff channel. Disabled cleanly when no render service is configured
// — the task is a no-op, not a failure.
// ---------------------------------------------------------------------------

type intelXHRSniffPayload struct {
	Version   string `json:"version"`
	SourceID  string `json:"source_id"`
	ChannelID string `json:"channel_id,omitempty"`
}

// sniffClient is the safetransport client for render-service calls —
// lazily built once (a fresh transport per task would leak pools).
var (
	sniffClientOnce sync.Once
	sniffClient     *http.Client
)

func intelSniffHTTPClient() *http.Client {
	sniffClientOnce.Do(func() {
		// The render service is operator-deployed infra — typically a
		// private/internal host — so the private-IP dial guard is off for
		// this call; the service URL is admin-configured, not
		// attacker-controlled.
		sniffClient = fetcher.NewSafeClientOpts(60*time.Second, 0, fetcher.SafeClientOpts{AllowPrivate: true})
	})
	return sniffClient
}

// harEntry is the slice of a HAR entry the sniffer consumes.
type harEntry struct {
	Request struct {
		Method   string `json:"method"`
		URL      string `json:"url"`
		PostData struct {
			MimeType string `json:"mimeType"`
			Text     string `json:"text"`
		} `json:"postData"`
	} `json:"request"`
	Response struct {
		Status  int `json:"status"`
		Content struct {
			MimeType string `json:"mimeType"`
		} `json:"content"`
	} `json:"response"`
}

// harEnvelope accepts the render service's HAR in any common shape:
// {"har": {"log": {"entries": [...]}}} (wrapped), a bare spec HAR
// {"log": {"entries": [...]}}, or the flattened {"entries": [...]}.
type harEnvelope struct {
	HAR struct {
		Log struct {
			Entries []harEntry `json:"entries"`
		} `json:"log"`
	} `json:"har"`
	Log struct {
		Entries []harEntry `json:"entries"`
	} `json:"log"`
	Entries []harEntry `json:"entries"`
}

func (e harEnvelope) entries() []harEntry {
	switch {
	case len(e.Entries) > 0:
		return e.Entries
	case len(e.HAR.Log.Entries) > 0:
		return e.HAR.Log.Entries
	default:
		return e.Log.Entries
	}
}

// HandleIntelXHRSniff runs the headless XHR capture for one source (or a
// specific xhr_sniff channel's entry_url when channel_id is present) and
// materializes discovered JSON endpoints as api channel proposals.
func HandleIntelXHRSniff(ctx context.Context, t *asynq.Task) error {
	defer recoverPanic("TypeIntelXHRSniff")
	slog.Info("task:intel:xhr_sniff")

	if intelSourcesStore == nil {
		slog.Warn("task:intel:xhr_sniff: sources store not wired, skipping")
		return nil
	}

	var p intelXHRSniffPayload
	if err := parsePayloadOrSkip(t, &p); err != nil {
		return err
	}

	sourceID, err := scanUUID(p.SourceID)
	if err != nil {
		return fmt.Errorf("task:intel:xhr_sniff: %w: %w", err, asynq.SkipRetry)
	}

	// Config gate: no render service → clean skip (the task is defined but
	// the environment never provides it).
	serviceURL := ""
	if appSettings != nil {
		serviceURL = strings.TrimRight(appSettings.GetStringOr(ctx, "intel.render.service_url", ""), "/")
	}
	if serviceURL == "" {
		slog.Info("task:intel:xhr_sniff: intel.render.service_url unset, skipping", "source_id", p.SourceID)
		return nil
	}

	src, err := intelSourcesStore.Get(ctx, sourceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Info("task:intel:xhr_sniff: source no longer exists, skipping", "source_id", p.SourceID)
			return nil
		}
		return fmt.Errorf("task:intel:xhr_sniff: load source: %w", err)
	}
	if !src.Enabled || src.Status != "active" || verifySkipSource(src) {
		slog.Info("task:intel:xhr_sniff: source not snifffable, skipping", "source_id", p.SourceID)
		return nil
	}

	// The sniff target is the channel's entry_url when a specific xhr_sniff
	// channel fired this task, else the source URL. wait_ms (page-settle
	// budget) is a provider_config override on the same row.
	target := src.SourceUrl
	waitMs := 5000
	if p.ChannelID != "" {
		if chID, cerr := scanUUID(p.ChannelID); cerr == nil {
			if ch, gerr := intelSourcesStore.GetChannel(ctx, chID); gerr == nil {
				target = ch.EntryUrl
				var cfg map[string]any
				if json.Unmarshal(ch.ProviderConfig, &cfg) == nil {
					if ms, ok := cfg["wait_ms"].(float64); ok && ms > 0 {
						waitMs = int(ms)
					}
				}
			}
		}
	}

	harBody, err := intelSniffHAR(ctx, serviceURL, target, waitMs)
	if err != nil {
		return fmt.Errorf("task:intel:xhr_sniff: %w", err)
	}

	endpoints := intelSniffEndpoints(harBody)
	if len(endpoints) == 0 {
		slog.Info("task:intel:xhr_sniff: no JSON endpoints discovered", "source_id", p.SourceID, "target", target)
		return nil
	}

	var inputs []sources.ChannelProposalInput
	for _, ep := range endpoints {
		inputs = append(inputs, intelSniffProposalInput(ep))
	}

	created, skipped, err := sources.NewService(intelSourcesStore).
		MaterializeChannels(ctx, sourceID, inputs)
	if err != nil {
		return fmt.Errorf("task:intel:xhr_sniff: materialize channels: %w", err)
	}

	slog.Info("task:intel:xhr_sniff: done", "source_id", p.SourceID, "endpoints", len(endpoints), "channels_created", created, "channels_skipped", skipped)
	return nil
}

// intelSniffHAR calls the render service's /har endpoint and returns the
// parsed envelope. Contract: POST {service_url}/har {"url","wait_ms"} →
// 200 {"har": <HAR>} or {"entries": [...]}.
func intelSniffHAR(ctx context.Context, serviceURL, target string, waitMs int) (harEnvelope, error) {
	var env harEnvelope

	reqBody, err := json.Marshal(map[string]any{"url": target, "wait_ms": waitMs})
	if err != nil {
		return env, fmt.Errorf("har request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serviceURL+"/har", bytes.NewReader(reqBody))
	if err != nil {
		return env, fmt.Errorf("har request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := intelSniffHTTPClient().Do(req)
	if err != nil {
		return env, fmt.Errorf("har: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(fetcher.NewMaxBodyReader(resp.Body, 20<<20))
	if err != nil {
		return env, fmt.Errorf("har read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return env, fmt.Errorf("har: status %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return env, fmt.Errorf("har decode: %w", err)
	}

	return env, nil
}

// intelSniffProposalInput builds the channel proposal for one discovered
// endpoint. A POST endpoint's request body (HAR postData) seeds
// provider_config.body so the materialized api channel replays the same
// request — a bodyless POST would 4xx forever. JSON bodies pass through
// verbatim; non-JSON bodies are stored as a JSON string (the api fetcher
// sends string bodies unquoted) with the captured mimeType replayed via
// provider_config.headers.
func intelSniffProposalInput(ep sniffEndpoint) sources.ChannelProposalInput {
	cfg := map[string]any{
		"endpoint":       ep.URL,
		"method":         ep.Method,
		"discovered_via": "xhr_sniff",
	}
	if ep.Method == http.MethodPost && len(ep.Body) > 0 {
		if json.Valid(ep.Body) {
			cfg["body"] = json.RawMessage(ep.Body)
		} else {
			cfg["body"] = string(ep.Body)
			if ep.BodyMime != "" {
				cfg["headers"] = map[string]string{"Content-Type": ep.BodyMime}
			}
		}
	}

	cfgJSON, _ := json.Marshal(cfg)

	return sources.ChannelProposalInput{
		Label:          "Discovered API " + ep.Method + " " + ep.URL,
		EntryURL:       ep.URL,
		CrawlMethod:    string(sources.CrawlMethodAPI),
		Pipeline:       string(sources.PipelineExtract),
		ProviderConfig: cfgJSON,
		Confidence:     0.7,
	}
}

// sniffEndpoint is one discovered JSON XHR endpoint — method + clean URL
// (query stripped; query variance dedupes to the same endpoint) plus the
// request body and its MIME type for POSTs.
type sniffEndpoint struct {
	Method   string
	URL      string
	Body     []byte
	BodyMime string
}

// intelSniffEndpoints extracts deduplicated JSON API endpoints from a HAR —
// GET/POST entries whose response MIME is JSON. Dedupe key is
// method+host+path (query variance ignored); private-literal hosts are
// dropped (the HAR is attacker-influenced content). Bounded at 10
// endpoints — beyond that the page is a firehose, not an API surface.
func intelSniffEndpoints(env harEnvelope) []sniffEndpoint {
	seen := map[string]bool{}
	var out []sniffEndpoint

	for _, e := range env.entries() {
		if len(out) >= 10 {
			break
		}
		if e.Response.Status < 200 || e.Response.Status >= 300 {
			continue
		}
		if !strings.Contains(strings.ToLower(e.Response.Content.MimeType), "json") {
			continue
		}
		method := strings.ToUpper(e.Request.Method)
		if method != http.MethodGet && method != http.MethodPost {
			continue
		}
		u, err := url.Parse(e.Request.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			continue
		}
		if sources.IsPrivateIPLiteral(u.Hostname()) {
			continue
		}
		key := method + " " + strings.ToLower(u.Host) + u.Path
		if seen[key] {
			continue
		}
		seen[key] = true
		var body []byte
		var bodyMime string
		if method == http.MethodPost {
			body = []byte(e.Request.PostData.Text)
			bodyMime = e.Request.PostData.MimeType
		}
		out = append(out, sniffEndpoint{
			Method:   method,
			URL:      u.Scheme + "://" + u.Host + u.EscapedPath(),
			Body:     body,
			BodyMime: bodyMime,
		})
	}

	return out
}
