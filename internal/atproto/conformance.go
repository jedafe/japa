package atproto

// Conformance — the §127 A3 release-gate readback: after first publish,
// verify the network actually indexes a member account by reading it
// back through a real appview (handle resolves, DID matches, a post is
// indexed). The same checks power POST /control/atproto/conformance
// (persisted runs, admin dashboard) and cmd/atproto-conformance (the
// pre-announcement gate) — one implementation, two surfaces.
//
// Ref: japaconnect-engineering/sections/127-atproto-native.md

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	gen "github.com/jedafe/japa/internal/db/gen"
)

// ConformanceCheck is one gate check: name, outcome, and the evidence
// an admin needs to act on a failure.
type ConformanceCheck struct {
	Name   string `json:"name"`
	Pass   bool   `json:"pass"`
	Detail string `json:"detail"`
}

// ConformanceResult is a finished run — actor targeted, resolved DID,
// per-check results, and the aggregate verdict.
type ConformanceResult struct {
	Actor  string             `json:"actor"`
	DID    string             `json:"did"`
	Checks []ConformanceCheck `json:"checks"`
	Passed bool               `json:"passed"`
}

var appviewClient = &http.Client{Timeout: 15 * time.Second}

// CheckAppview runs the readback checks against an appview base — pure
// HTTP, no DB access, so the CLI can call it directly. actor is a
// handle or DID; expectedDID asserts the resolved DID when non-empty.
// hasPosts resolves after the profile check, once the DID is known: it
// reports whether our own index holds a feed.post for the DID, so the
// post-indexed check only fails on real content. Check failures live
// in the result (Passed=false); a non-nil error is reserved for a
// malformed appview response, never a failed check.
func CheckAppview(ctx context.Context, appviewBase, actor, expectedDID string, hasPosts func(did string) bool) (ConformanceResult, error) {
	res := ConformanceResult{Actor: actor}
	check := func(name string, err error, detail string) {
		c := ConformanceCheck{Name: name, Pass: err == nil, Detail: detail}
		if err != nil {
			c.Detail = err.Error()
		}
		res.Checks = append(res.Checks, c)
	}

	// 1. Profile resolves and the handle is NOT handle.invalid — the
	// bidirectional handle↔DID proof worked end to end.
	profile, err := appviewGet(ctx, appviewBase, "/xrpc/app.bsky.actor.getProfile",
		url.Values{"actor": {actor}})
	if err != nil {
		check("profile resolves", fmt.Errorf("getProfile: %w", err), "")
		return res, nil
	}
	check("profile resolves", nil, actor)
	gotDID, _ := profile["did"].(string)
	gotHandle, _ := profile["handle"].(string)
	res.DID = gotDID
	if gotHandle == "" || gotHandle == "handle.invalid" {
		check("handle resolves", fmt.Errorf("got %q", gotHandle), "")
	} else {
		check("handle resolves", nil, gotHandle)
	}
	if expectedDID != "" && gotDID != expectedDID {
		check("did matches", fmt.Errorf("got %q, want %q", gotDID, expectedDID), "")
	} else {
		check("did matches", nil, gotDID)
	}

	// 2. A post is indexed — projection flowed repo → relay → appview.
	// Only meaningful when our own index holds a feed.post for the DID.
	expectPosts := hasPosts != nil && hasPosts(gotDID)
	feed, err := appviewGet(ctx, appviewBase, "/xrpc/app.bsky.feed.getAuthorFeed",
		url.Values{"actor": {gotDID}, "limit": {"1"}})
	if err != nil {
		check("post indexed", fmt.Errorf("getAuthorFeed: %w", err), "")
	} else {
		items, _ := feed["feed"].([]any)
		switch {
		case expectPosts && len(items) == 0:
			check("post indexed", fmt.Errorf("author feed empty"), "")
		default:
			check("post indexed", nil, fmt.Sprintf("%d post(s) visible", len(items)))
		}
	}

	res.Passed = true
	for _, c := range res.Checks {
		if !c.Pass {
			res.Passed = false
			break
		}
	}
	return res, nil
}

// RunConformanceCheck is the admin surface: resolve the target (explicit
// handle/DID, or auto-pick an active account preferring one with posts),
// run the appview checks, persist the run, and return it.
func (s *Service) RunConformanceCheck(ctx context.Context, handle, did string, runBy pgtype.UUID) (gen.AtprotoConformanceRun, ConformanceResult, error) {
	var actor, expectedDID string
	var pickedHasPosts bool

	switch {
	case did != "":
		actor, expectedDID = did, did
	case handle != "":
		actor = handle
	default:
		picked, err := s.store.q.PickAtprotoConformanceTarget(ctx)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return gen.AtprotoConformanceRun{}, ConformanceResult{}, fmt.Errorf("no atproto account found (opt in a member first)")
			}
			return gen.AtprotoConformanceRun{}, ConformanceResult{}, fmt.Errorf("atproto: pick conformance target: %w", err)
		}
		actor, expectedDID, pickedHasPosts = picked.Did, picked.Did, picked.HasPosts
	}

	res, err := CheckAppview(ctx, s.cfg.appviewURL(), actor, expectedDID, func(gotDID string) bool {
		if expectedDID == gotDID && pickedHasPosts {
			return true
		}
		posts, err := s.store.q.AtprotoDIDHasPosts(ctx, gotDID)
		return err == nil && posts
	})
	if err != nil {
		return gen.AtprotoConformanceRun{}, res, fmt.Errorf("atproto: conformance checks: %w", err)
	}

	checksJSON, err := json.Marshal(res.Checks)
	if err != nil {
		return gen.AtprotoConformanceRun{}, res, fmt.Errorf("atproto: marshal checks: %w", err)
	}
	run, err := s.store.q.InsertAtprotoConformanceRun(ctx, gen.InsertAtprotoConformanceRunParams{
		Actor:  actor,
		Did:    res.DID,
		Checks: checksJSON,
		Passed: res.Passed,
		RunBy:  runBy,
	})
	if err != nil {
		return gen.AtprotoConformanceRun{}, res, fmt.Errorf("atproto: persist conformance run: %w", err)
	}
	return run, res, nil
}

// ListConformanceRuns returns the paginated run history newest-first.
func (s *Service) ListConformanceRuns(ctx context.Context, limit, offset int32) ([]gen.AtprotoConformanceRun, int64, error) {
	rows, err := s.store.q.ListAtprotoConformanceRuns(ctx, gen.ListAtprotoConformanceRunsParams{
		PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("atproto: list conformance runs: %w", err)
	}
	total, err := s.store.q.CountAtprotoConformanceRuns(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("atproto: count conformance runs: %w", err)
	}
	return rows, total, nil
}

func appviewGet(ctx context.Context, base, path string, params url.Values) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(base, "/")+path+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := appviewClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("appview %s: %s", path, string(raw[:min(len(raw), 300)]))
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("appview %s: bad JSON: %w", path, err)
	}
	return doc, nil
}
