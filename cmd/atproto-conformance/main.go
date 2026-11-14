// atproto-conformance is the §127 A3 release gate: after first publish,
// read the account back through the PUBLIC appview (public.api.bsky.app)
// and assert the handle resolves (not handle.invalid), the profile
// resolves, and a post is indexed — the detection loop for atproto's
// silent non-indexing failure mode (ADR-139 Risks).
//
// Usage:
//
//	go run ./cmd/atproto-conformance                    # auto-pick an active account
//	go run ./cmd/atproto-conformance -handle alice.japa.to
//	go run ./cmd/atproto-conformance -did did:web:api.japa.to:atproto:u:{uuid}
//
// Exit 0 on PASS, 1 on any FAIL.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/config"
)

const appviewBase = "https://public.api.bsky.app"

var appviewClient = &http.Client{Timeout: 15 * time.Second}

// target is the account under test. hasPosts marks whether a projected
// feed.post is expected to be visible in the appview feed.
type target struct {
	actor    string // handle or DID for the getProfile call
	did      string // expected DID ("" when not yet verified)
	hasPosts bool
}

func main() {
	handle := flag.String("handle", "", "member handle to verify (e.g. alice.japa.to)")
	did := flag.String("did", "", "member DID to verify")
	flag.Parse()

	if err := run(*handle, *did); err != nil {
		slog.Error("atproto conformance: FAIL", "error", err)
		os.Exit(1)
	}
}

func run(handle, did string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var tgt target
	switch {
	case did != "":
		tgt = target{actor: did, did: did, hasPosts: true}
	case handle != "":
		tgt = target{actor: handle, hasPosts: true}
	default:
		picked, err := pickAccount(ctx)
		if err != nil {
			return err
		}
		tgt = picked
		slog.Info("atproto conformance: auto-picked account", "did", tgt.did)
	}

	failed := false
	check := func(name string, err error, detail string) {
		if err != nil {
			failed = true
			slog.Error("atproto conformance: FAIL", "check", name, "error", err, "detail", detail)
			return
		}
		slog.Info("atproto conformance: PASS", "check", name, "detail", detail)
	}

	// 1. Profile resolves and the handle is NOT handle.invalid — the
	// bidirectional handle↔DID proof worked end to end.
	profile, err := getJSON(ctx, "/xrpc/app.bsky.actor.getProfile", url.Values{"actor": {tgt.actor}})
	if err != nil {
		check("profile resolves", fmt.Errorf("getProfile: %w", err), tgt.actor)
		return fmt.Errorf("conformance failed")
	}
	gotDID, _ := profile["did"].(string)
	gotHandle, _ := profile["handle"].(string)
	if gotHandle == "" || gotHandle == "handle.invalid" {
		check("handle resolves", fmt.Errorf("got %q", gotHandle), tgt.actor)
	} else {
		check("handle resolves", nil, gotHandle)
	}
	if tgt.did != "" && gotDID != tgt.did {
		check("did matches", fmt.Errorf("got %q, want %q", gotDID, tgt.did), tgt.actor)
	} else {
		check("did matches", nil, gotDID)
	}

	// 2. A post is indexed — projection flowed repo → relay → appview.
	feed, err := getJSON(ctx, "/xrpc/app.bsky.feed.getAuthorFeed", url.Values{
		"actor": {gotDID}, "limit": {"1"},
	})
	if err != nil {
		check("author feed", fmt.Errorf("getAuthorFeed: %w", err), gotDID)
	} else {
		items, _ := feed["feed"].([]any)
		switch {
		case tgt.hasPosts && len(items) == 0:
			check("post indexed", fmt.Errorf("author feed empty"), gotDID)
		default:
			check("post indexed", nil, fmt.Sprintf("%d post(s) visible", len(items)))
		}
	}

	if failed {
		return fmt.Errorf("conformance failed")
	}
	return nil
}

// pickAccount chooses an active account — preferring one with at least
// one indexed feed.post so the post check is meaningful.
func pickAccount(ctx context.Context) (target, error) {
	cfg, err := config.Load()
	if err != nil {
		return target{}, fmt.Errorf("loading config: %w", err)
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return target{}, fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	var t target
	err = pool.QueryRow(ctx, `
		SELECT a.did, EXISTS (
			SELECT 1 FROM atproto_records r
			WHERE r.account_id = a.id AND r.collection = 'app.bsky.feed.post'
		)
		FROM atproto_accounts a
		WHERE a.status = 'active'
		ORDER BY EXISTS (
			SELECT 1 FROM atproto_records r
			WHERE r.account_id = a.id AND r.collection = 'app.bsky.feed.post'
		) DESC, a.created_at ASC
		LIMIT 1`).Scan(&t.did, &t.hasPosts)
	if err != nil {
		return target{}, fmt.Errorf("no atproto account found (opt in a member first): %w", err)
	}
	t.actor = t.did
	return t, nil
}

func getJSON(ctx context.Context, path string, params url.Values) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, appviewBase+path+"?"+params.Encode(), nil)
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
