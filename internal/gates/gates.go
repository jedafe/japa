// Package gates implements ops access gates: super-admin-operated
// start/stop toggles that control public reachability of privileged
// surfaces (admin panel, control panel, Stalwart admin gate) through the
// Coolify API. Ref: japaconnect-engineering/sections/123-ops-access-gates.md
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
)

// Target is one gate-able Coolify resource from JC_ACCESS_GATES.
type Target struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Kind   string `json:"kind"` // application | service
	UUID   string `json:"uuid"`
	Domain string `json:"domain,omitempty"`
}

// Status is a Target plus its live Coolify state.
type Status struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Kind   string `json:"kind"`
	Domain string `json:"domain,omitempty"`
	// State is the normalized gate state: on | off | transitioning | unknown.
	State string `json:"state"`
	// Raw is Coolify's status string verbatim (e.g. "running:unknown").
	Raw string `json:"status"`
}

// Service talks to the Coolify API. Constructed with New; when Enabled() is
// false the routes should report feature_disabled without calling it.
type Service struct {
	base    string
	token   string
	http    *http.Client
	targets []Target
	idx     map[string]int
	mu      []sync.Mutex // per-target serialization of toggle calls
}

// New builds a Service from config. targetsJSON is the JC_ACCESS_GATES
// document; baseURL/token come from JC_COOLIFY_URL / JC_COOLIFY_TOKEN.
// A nil service with err == nil means the feature is disabled (unset envs).
func New(baseURL, token, targetsJSON string) (*Service, error) {
	if baseURL == "" || token == "" || targetsJSON == "" {
		return nil, nil
	}
	var targets []Target
	if err := json.Unmarshal([]byte(targetsJSON), &targets); err != nil {
		return nil, fmt.Errorf("gates: invalid JC_ACCESS_GATES json: %w", err)
	}
	idx := make(map[string]int, len(targets))
	for i, t := range targets {
		if t.Key == "" || t.UUID == "" || (t.Kind != "application" && t.Kind != "service") {
			return nil, fmt.Errorf("gates: invalid target at index %d (key/kind/uuid required, kind must be application|service)", i)
		}
		if _, dup := idx[t.Key]; dup {
			return nil, fmt.Errorf("gates: duplicate target key %q", t.Key)
		}
		idx[t.Key] = i
	}
	return &Service{
		base:    baseURL,
		token:   token,
		http:    &http.Client{Timeout: 15 * time.Second},
		targets: targets,
		idx:     idx,
		mu:      make([]sync.Mutex, len(targets)),
	}, nil
}

// List returns every configured target with its live Coolify status. A
// per-target fetch failure marks that gate "unknown" rather than failing the
// whole list — the operator still sees the rest.
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
		}(i, t)
	}
	wg.Wait()
	return out
}

// Toggle flips a gate: running → stop, anything else → start. Serialized per
// target so concurrent flips cannot interleave. Returns the gate's status
// after the operation call (Coolify applies async — "transitioning" is a
// normal result while the proxy reconfigures).
func (s *Service) Toggle(ctx context.Context, key string) (*Status, error) {
	i, ok := s.idx[key]
	if !ok {
		return nil, errUnknownKey{key}
	}
	t := s.targets[i]
	s.mu[i].Lock()
	defer s.mu[i].Unlock()

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

// resourcePath maps the target onto its Coolify API path.
func (t Target) resourcePath() string {
	kind := "applications"
	if t.Kind == "service" {
		kind = "services"
	}
	return fmt.Sprintf("/api/v1/%s/%s", kind, t.UUID)
}

// fetchStatus reads the resource's status field from Coolify.
func (s *Service) fetchStatus(ctx context.Context, t Target) (string, error) {
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

// normalize maps Coolify resource statuses onto the gate state axis.
func normalize(status string) string {
	switch {
	case strings.HasPrefix(status, "running"), strings.HasPrefix(status, "healthy"):
		return "on"
	case strings.HasPrefix(status, "exited"), strings.HasPrefix(status, "stopped"),
		strings.HasPrefix(status, "dead"), strings.HasPrefix(status, "unhealthy:exited"):
		return "off"
	case strings.HasPrefix(status, "starting"), strings.HasPrefix(status, "restarting"),
		strings.HasPrefix(status, "stopping"), strings.HasPrefix(status, "deploying"),
		strings.HasPrefix(status, "degraded"):
		return "transitioning"
	default:
		return "unknown"
	}
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
