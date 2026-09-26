// Command backfill-university-geo links existing university spaces to their
// state/city (and country) primary geo spaces by re-running the same keyword
// matcher used at space creation time (spaces.Service.LinkSpaceToPrimarySpaces).
// Universities were seeded with only a country geo_code; this fills in the
// space_parent_spaces junction rows for state/city ancestors so the
// ?state=/?city= (within) catalog filters can resolve them.
//
// The insert uses ON CONFLICT DO NOTHING, so the command is idempotent and
// safe to re-run. Non-geo matches (e.g. education-category spaces sharing a
// name) and self-links are filtered out before insertion. parent_id is left
// untouched — the within filter reads the junction, and rewriting 10k rows'
// primary parent is riskier than the filter requires.
//
// Usage:
//
//	go run ./cmd/backfill-university-geo            # apply
//	go run ./cmd/backfill-university-geo -dry-run   # report only
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
	gen "github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/spaces"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "report proposed links without writing")
	flag.Parse()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	database, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	svc := spaces.NewService(database)

	type uni struct {
		id   pgtype.UUID
		name string
	}

	rows, err := database.Reader.Query(ctx, `
		SELECT s.id, s.name
		FROM spaces s
		JOIN space_types st ON st.id = s.space_type_id
		WHERE st.slug = 'university' AND s.deleted_at IS NULL
		ORDER BY s.id
	`)
	if err != nil {
		slog.Error("failed to query university spaces", "error", err)
		os.Exit(1)
	}

	var unis []uni
	for rows.Next() {
		var u uni
		if err := rows.Scan(&u.id, &u.name); err != nil {
			slog.Warn("skipping row: scan error", "error", err)
			continue
		}
		unis = append(unis, u)
	}
	rows.Close()

	if err := rows.Err(); err != nil {
		slog.Error("row iteration error", "error", err)
		os.Exit(1)
	}

	slog.Info("found university spaces", "count", len(unis), "dry_run", *dryRun)

	// Pass 1 — run the matcher per university and collect candidate parents.
	// parentLevel caches level lookups so each parent space is queried once.
	type link struct {
		spaceID  pgtype.UUID
		parentID pgtype.UUID
	}
	var candidates []link
	parentIDs := map[pgtype.UUID]struct{}{}
	matched, errors := 0, 0

	for i, u := range unis {
		_, parentIDsForSpace, err := svc.LinkSpaceToPrimarySpaces(ctx, u.id, u.name)
		if err != nil {
			slog.Warn("matcher failed", "space", u.name, "error", err)
			errors++
			continue
		}
		for _, pid := range parentIDsForSpace {
			if !pid.Valid || pid == u.id {
				continue
			}
			candidates = append(candidates, link{spaceID: u.id, parentID: pid})
			parentIDs[pid] = struct{}{}
		}
		if len(parentIDsForSpace) > 0 {
			matched++
		}
		if (i+1)%500 == 0 {
			slog.Info("matcher progress", "done", i+1, "of", len(unis))
		}
	}

	slog.Info("matcher complete", "universities_matched", matched, "matcher_errors", errors, "candidate_links", len(candidates))

	// Pass 2 — resolve levels for candidate parents; keep only geo spaces
	// (country/state/city). Education-category matches would pollute the
	// junction without helping the within filter.
	uniqueParents := make([]pgtype.UUID, 0, len(parentIDs))
	for pid := range parentIDs {
		uniqueParents = append(uniqueParents, pid)
	}

	geoLevels := map[string]bool{"country": true, "state": true, "city": true}
	geoParent := map[pgtype.UUID]bool{}
	if len(uniqueParents) > 0 {
		lvlRows, err := database.Reader.Query(ctx, `SELECT id, level FROM spaces WHERE id = ANY($1)`, uniqueParents)
		if err != nil {
			slog.Error("failed to resolve parent levels", "error", err)
			os.Exit(1)
		}
		for lvlRows.Next() {
			var id pgtype.UUID
			var level string
			if err := lvlRows.Scan(&id, &level); err != nil {
				continue
			}
			geoParent[id] = geoLevels[level]
		}
		lvlRows.Close()
	}

	// Pass 3 — insert missing junction rows in a single transaction. Inside a
	// tx an error aborts the whole batch, so failures surface loudly instead
	// of silently skipping rows; ON CONFLICT DO NOTHING keeps re-runs clean.
	// linkedUnis counts universities that end up with at least one geo link —
	// a truer signal than `matched`, which counts any keyword hit.
	inserted, skipped := 0, 0
	linkedUnis := map[pgtype.UUID]struct{}{}

	if *dryRun {
		for _, c := range candidates {
			if !geoParent[c.parentID] {
				skipped++
				continue
			}
			inserted++
			linkedUnis[c.spaceID] = struct{}{}
		}
	} else {
		err := db.WithTx(ctx, database.Writer, func(q *gen.Queries) error {
			for _, c := range candidates {
				if !geoParent[c.parentID] {
					skipped++
					continue
				}
				if err := q.AddParentSpace(ctx, gen.AddParentSpaceParams{
					SpaceID:       c.spaceID,
					ParentSpaceID: c.parentID,
				}); err != nil {
					return fmt.Errorf("inserting link %s -> %s: %w", c.spaceID, c.parentID, err)
				}
				inserted++
				linkedUnis[c.spaceID] = struct{}{}
			}
			return nil
		})
		if err != nil {
			slog.Error("backfill failed — transaction rolled back", "error", err)
			os.Exit(1)
		}
	}

	slog.Info("backfill complete", "universities_linked", len(linkedUnis), "links_written_or_proposed", inserted, "non_geo_matches_skipped", skipped)
	fmt.Printf("\nDone: %d geo links %s across %d universities, %d non-geo matches skipped\n", inserted, map[bool]string{true: "proposed", false: "written"}[*dryRun], len(linkedUnis), skipped)
}
