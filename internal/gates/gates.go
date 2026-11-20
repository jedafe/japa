// Package gates implements ops access gates: super-admin-operated
// start/stop toggles that control public reachability of privileged
// surfaces (admin panel, control panel, Stalwart admin gate) through the
// Coolify API, plus API-internal surface lockdowns enforced by middleware.
// Ref: japaconnect-engineering/sections/123-ops-access-gates.md
package gates

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Target is one gate-able resource from JC_ACCESS_GATES.
type Target struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Kind   string `json:"kind"` // application | service | api_surface
	UUID   string `json:"uuid,omitempty"`
	Domain string `json:"domain,omitempty"`
	// api_surface targets: Prefix is the route prefix the lockdown middleware
	// protects; Flag is the feature_flags key written by toggles.
	Prefix string `json:"prefix,omitempty"`
	Flag   string `json:"flag,omitempty"`
	// ProbeURL is an optional URL fetched on status reads — the UI renders it
	// as a route-level up/down signal next to the resource state.
	ProbeURL string `json:"probe_url,omitempty"`
}

// Status is a Target plus its live state.
type Status struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Kind   string `json:"kind"`
	Domain string `json:"domain,omitempty"`
	// State is the normalized gate state: on | off | transitioning | unknown.
	State string `json:"state"`
	// Raw is the backend status verbatim (Coolify status, "open"/"locked").
	Raw string `json:"status"`
	// Probe is the route-level check: true = URL answered <500.
	Probe *bool `json:"route_reachable,omitempty"`
	// ProbeStatus describes the probe outcome ("200", "503", "timeout", …).
	ProbeStatus string `json:"route_status,omitempty"`
}

// FlagIO is the feature-flag surface needed by api_surface gates — satisfied
// by *features.Checker (§42).
type FlagIO interface {
	IsEnabled(ctx context.Context, key string) bool
	SetEnabled(ctx context.Context, key string, enabled bool, updatedBy pgtype.UUID) error
}

// Service talks to the Coolify API and the feature-flag store. Constructed
// with New; when nil the routes should report feature_disabled.
type Service struct {
	base    string // Coolify base URL — may be empty when only api_surface gates are configured
	token   string
	http    *http.Client
	probe   *http.Client
	flags   FlagIO
	targets []Target
	idx     map[string]int
	mu      []sync.Mutex // per-target serialization of toggle calls

	probeMu    sync.Mutex
	probeCache map[string]probeResult
}

// New builds a Service from config. targetsJSON is the JC_ACCESS_GATES
// document; baseURL/token come from JC_COOLIFY_URL / JC_COOLIFY_TOKEN; flags
// is the feature-flag checker (required when any api_surface target exists).
// A nil service with err == nil means the feature is disabled (no targets).
func New(baseURL, token, targetsJSON string, flags FlagIO) (*Service, error) {
	if targetsJSON == "" {
		return nil, nil
	}
	var targets []Target
	if err := json.Unmarshal([]byte(targetsJSON), &targets); err != nil {
		return nil, fmt.Errorf("gates: invalid JC_ACCESS_GATES json: %w", err)
	}
	if len(targets) == 0 {
		return nil, nil
	}
	idx := make(map[string]int, len(targets))
	for i, t := range targets {
		if t.Key == "" {
			return nil, fmt.Errorf("gates: target at index %d missing key", i)
		}
		switch t.Kind {
		case "application", "service":
			if t.UUID == "" {
				return nil, fmt.Errorf("gates: target %q missing uuid", t.Key)
			}
		case "api_surface":
			if t.Prefix == "" || t.Flag == "" {
				return nil, fmt.Errorf("gates: api_surface target %q requires prefix and flag", t.Key)
			}
			if flags == nil {
				return nil, fmt.Errorf("gates: api_surface target %q configured but no flag store available", t.Key)
			}
		default:
			return nil, fmt.Errorf("gates: target %q has invalid kind %q", t.Key, t.Kind)
		}
		if _, dup := idx[t.Key]; dup {
			return nil, fmt.Errorf("gates: duplicate target key %q", t.Key)
		}
		idx[t.Key] = i
	}
	return &Service{
		base:       strings.TrimRight(baseURL, "/"),
		token:      token,
		http:       &http.Client{Timeout: 15 * time.Second},
		probe:      &http.Client{Timeout: 5 * time.Second},
		flags:      flags,
		targets:    targets,
		idx:        idx,
		mu:         make([]sync.Mutex, len(targets)),
		probeCache: make(map[string]probeResult),
	}, nil
}

// List returns every configured target with its live state. Per-target fetch
// failures mark that gate "unknown" rather than failing the whole list.
func (s *Service) List(ctx context.Context) []Status {
	out := make([]Status, len(s.targets))
	var wg sync.WaitGroup
	for i, t := range s.targets {
		out[i] = Status{Key: t.Key, Label: t.Label, Kind: t.Kind, Domain: t.Domain, State: "unknown"}
		wg.Add(1)
		go func(i int, t Target) {
			defer wg.Done()
			raw, err := s.fetchStatus(ctx, t)
			if err == nil {
				out[i].Raw = raw
				out[i].State = normalize(raw)
			}
			if t.ProbeURL != "" {
				ok, st := s.probeURL(ctx, t.ProbeURL)
				out[i].Probe = &ok
				out[i].ProbeStatus = st
			}
		}(i, t)
	}
	wg.Wait()
	return out
}

// Toggle flips a gate: running → stop/lock, anything else → start/unlock.
// Serialized per target. Coolify applies async — "transitioning" is a normal
// result while the proxy reconfigures; api_surface flips are immediate.
func (s *Service) Toggle(ctx context.Context, key string, actor pgtype.UUID) (*Status, error) {
	i, ok := s.idx[key]
	if !ok {
		return nil, errUnknownKey{key}
	}
	t := s.targets[i]
	s.mu[i].Lock()
	defer s.mu[i].Unlock()

	if t.Kind == "api_surface" {
		locked := s.flags.IsEnabled(ctx, t.Flag)
		op := "lock"
		if locked {
			op = "unlock"
		}
		if err := s.flags.SetEnabled(ctx, t.Flag, !locked, actor); err != nil {
			return nil, fmt.Errorf("gates: %s %q: %w", op, key, err)
		}
		st := surfaceStatus(!locked)
		return &Status{Key: t.Key, Label: t.Label, Kind: t.Kind, Domain: t.Domain, State: normalize(st), Raw: st}, nil
	}

	raw, err := s.fetchStatus(ctx, t)
	if err != nil {
		return nil, fmt.Errorf("gates: status for %q: %w", key, err)
	}
	action := "start"
	if normalize(raw) == "on" {
		action = "stop"
	}
	if err := s.call(ctx, http.MethodPost, t, "/"+action); err != nil {
		return nil, fmt.Errorf("gates: %s %q: %w", action, key, err)
	}
	raw, err = s.fetchStatus(ctx, t)
	if err != nil {
		// The start/stop call was accepted; a failed status read must not
		// report failure to the operator — the UI polls for convergence.
		return &Status{Key: t.Key, Label: t.Label, Kind: t.Kind, Domain: t.Domain, State: "transitioning"}, nil
	}
	return &Status{Key: t.Key, Label: t.Label, Kind: t.Kind, Domain: t.Domain, State: normalize(raw), Raw: raw}, nil
}

type errUnknownKey struct{ key string }

func (e errUnknownKey) Error() string { return "unknown gate: " + e.key }

// IsUnknownKey reports whether err means the requested gate is not
// configured (→ 404).
func IsUnknownKey(err error) bool {
	_, ok := err.(errUnknownKey)
	return ok
}

// fetchStatus reads live state for a target: Coolify resource status for
// application/service, flag state for api_surface.
func (s *Service) fetchStatus(ctx context.Context, t Target) (string, error) {
	if t.Kind == "api_surface" {
		return surfaceStatus(s.flags.IsEnabled(ctx, t.Flag)), nil
	}
	if s.base == "" || s.token == "" {
		return "", fmt.Errorf("coolify credentials not configured")
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := s.callInto(ctx, http.MethodGet, t, "", &body); err != nil {
		return "", err
	}
	if body.Status == "" {
		return "unknown", nil
	}
	return body.Status, nil
}

// surfaceStatus maps the lockdown flag onto the raw-status axis: flag on =
// surface locked = gate off.
func surfaceStatus(locked bool) string {
	if locked {
		return "locked"
	}
	return "open"
}

// normalize maps raw statuses onto the gate state axis.
func normalize(status string) string {
	switch {
	case strings.HasPrefix(status, "running"), strings.HasPrefix(status, "healthy"),
		status == "open":
		return "on"
	case strings.HasPrefix(status, "exited"), strings.HasPrefix(status, "stopped"),
		strings.HasPrefix(status, "dead"), strings.HasPrefix(status, "unhealthy:exited"),
		status == "locked":
		return "off"
	case strings.HasPrefix(status, "starting"), strings.HasPrefix(status, "restarting"),
		strings.HasPrefix(status, "stopping"), strings.HasPrefix(status, "deploying"),
		strings.HasPrefix(status, "degraded"):
		return "transitioning"
	default:
		return "unknown"
	}
}

// resourcePath maps the target onto its Coolify API path.
func (t Target) resourcePath() string {
	kind := "applications"
	if t.Kind == "service" {
		kind = "services"
	}
	return fmt.Sprintf("/api/v1/%s/%s", kind, t.UUID)
}

// callInto performs a Coolify API request and decodes the JSON body into v
// (nil v = discard body).
func (s *Service) callInto(ctx context.Context, method string, t Target, suffix string, v any) error {
	req, err := http.NewRequestWithContext(ctx, method, s.base+t.resourcePath()+suffix, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Accept", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("coolify %s: %d %s", method, resp.StatusCode, truncate(string(data), 200))
	}
	if v != nil {
		return json.Unmarshal(data, v)
	}
	return nil
}

func (s *Service) call(ctx context.Context, method string, t Target, suffix string) error {
	return s.callInto(ctx, method, t, suffix, nil)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

type probeResult struct {
	ok     bool
	status string
	at     time.Time
}

// probeURL checks whether a gate's public route answers — reachable means a
// completed HTTP response below 500 (auth redirects/401s count as up).
// Results cache 10s so the UI's transition polling doesn't fan out per call.
func (s *Service) probeURL(ctx context.Context, url string) (bool, string) {
	s.probeMu.Lock()
	if r, hit := s.probeCache[url]; hit && time.Since(r.at) < 10*time.Second {
		s.probeMu.Unlock()
		return r.ok, r.status
	}
	s.probeMu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, "invalid_url"
	}
	resp, err := s.probe.Do(req)
	ok, status := false, "timeout"
	if err == nil {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 8192))
		resp.Body.Close()
		ok = resp.StatusCode < 500
		status = fmt.Sprintf("%d", resp.StatusCode)
	} else if ctx.Err() == nil && !strings.Contains(err.Error(), "deadline") {
		status = "error"
	}

	s.probeMu.Lock()
	s.probeCache[url] = probeResult{ok: ok, status: status, at: time.Now()}
	s.probeMu.Unlock()
	return ok, status
}
