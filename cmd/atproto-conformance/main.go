// atproto-conformance is the §127 A3 release gate: after first publish,
// read the account back through the PUBLIC appview (public.api.bsky.app
// by default — JC_ATPROTO_APPVIEW_URL overrides) and assert the handle
// resolves (not handle.invalid), the profile resolves, and a post is
// indexed — the detection loop for atproto's silent non-indexing
// failure mode (ADR-139 Risks). The checks themselves live in
// internal/atproto and also serve POST /admin/atproto/conformance.
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
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/atproto"
	"github.com/jedafe/japa/internal/config"
	gen "github.com/jedafe/japa/internal/db/gen"
)

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

	var actor, expectedDID string
	var pickedHasPosts bool

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()
	q := gen.New(pool)

	switch {
	case did != "":
		actor, expectedDID = did, did
	case handle != "":
		actor = handle
	default:
		picked, err := q.PickAtprotoConformanceTarget(ctx)
		if err != nil {
			return fmt.Errorf("no atproto account found (opt in a member first): %w", err)
		}
		actor, expectedDID, pickedHasPosts = picked.Did, picked.Did, picked.HasPosts
		slog.Info("atproto conformance: auto-picked account", "did", actor)
	}

	res, err := atproto.CheckAppview(ctx, cfg.ATProtoAppViewURL, actor, expectedDID,
		func(gotDID string) bool {
			if expectedDID == gotDID && pickedHasPosts {
				return true
			}
			posts, err := q.AtprotoDIDHasPosts(ctx, gotDID)
			return err == nil && posts
		})
	if err != nil {
		return err
	}
	for _, c := range res.Checks {
		if c.Pass {
			slog.Info("atproto conformance: PASS", "check", c.Name, "detail", c.Detail)
		} else {
			slog.Error("atproto conformance: FAIL", "check", c.Name, "detail", c.Detail)
		}
	}
	if !res.Passed {
		return fmt.Errorf("conformance failed")
	}
	return nil
}
