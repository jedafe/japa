// Command backfill-university-geo reconciles the space_parent_spaces edges of
// university spaces against their geo_code country. Universities were seeded
// with a country geo_code; the original version of this command re-ran the
// unscoped name-keyword matcher (spaces.Service.LinkSpaceToPrimarySpaces),
// which linked unis to every country carrying a shared keyword token —
// "Tbilisi Institute of Asia and Africa" gained ~98 parents via "Asia"/
// "Africa". The matcher is now geo-anchored: it returns the geo_code country
// plus any state/city keyword matches inside that country's subtree.
//
// Three passes:
//  1. Run the geo-anchored matcher per university and collect the desired
//     parent set.
//  2. Keep only geo-level candidates (country/state/city).
//  3. Insert missing junction rows (ON CONFLICT DO NOTHING). With -purge,
//     also delete existing geo-level edges that fall outside the uni's
//     geo_code country subtree. parent_id is left untouched — the within
//     filter reads the junction, and rewriting 10k rows' primary parent is
//     riskier than the filter requires.
//
// Inserts and deletes run inside one transaction each; failures abort loudly.
// -purge defaults off so a plain re-run only adds rows. Run -dry-run -purge
// first to preview deletions.
//
// Usage:
//
//	go run ./cmd/backfill-university-geo              # add missing links
//	go run ./cmd/backfill-university-geo -purge       # + remove foreign-country links
//	go run ./cmd/backfill-university-geo -dry-run -purge
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
	dryRun := flag.Bool("dry-run", false, "report proposed links/deletions without writing")
	purge := flag.Bool("purge", false, "delete geo-level parent edges outside each uni's geo_code country")
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
	q := gen.New(database.Reader)

	type uni struct {
		id      pgtype.UUID
		name    string
		geoCode string
	}

	rows, err := database.Reader.Query(ctx, `
		SELECT s.id, s.name, COALESCE(s.geo_code, '')
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
		if err := rows.Scan(&u.id, &u.name, &u.geoCode); err != nil {
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

	slog.Info("found university spaces", "count", len(unis), "dry_run", *dryRun, "purge", *purge)

	// Resolve each uni's geo_code to its country space ID once — most unis
	// share a handful of codes.
	countryByCode := map[string]pgtype.UUID{}
	uniCountry := map[pgtype.UUID]pgtype.UUID{}

	for _, u := range unis {
		if u.geoCode == "" {
			continue
		}

		countryID, ok := countryByCode[u.geoCode]
		if !ok {
			c, err := q.GetSpaceByGeoCode(ctx, &u.geoCode)
			if err != nil || !c.ID.Valid {
				slog.Warn("geo_code has no country space", "space", u.name, "geo_code", u.geoCode)

				countryByCode[u.geoCode] = pgtype.UUID{}

				continue
			}

			countryID = c.ID
			countryByCode[u.geoCode] = countryID
		}

		if countryID.Valid {
			uniCountry[u.id] = countryID
		}
	}

	// Pass 1 — run the geo-anchored matcher per university and collect
	// candidate parents.
	type link struct {
		spaceID  pgtype.UUID
		parentID pgtype.UUID
	}

	var candidates []link

	parentIDs := map[pgtype.UUID]struct{}{}
	matched, errors := 0, 0

	for i, u := range unis {
		_, parentIDsForSpace, err := svc.LinkSpaceToPrimarySpaces(ctx, u.id, u.name, u.geoCode)
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

	slog.Info("insert phase complete", "universities_linked", len(linkedUnis), "links_written_or_proposed", inserted, "non_geo_matches_skipped", skipped)

	if !*purge {
		fmt.Printf("\nDone: %d geo links %s across %d universities, %d non-geo matches skipped\n", inserted, map[bool]string{true: "proposed", false: "written"}[*dryRun], len(linkedUnis), skipped)
		return
	}

	// Purge pass — for every current uni→geo-parent edge, resolve the parent's
	// country ancestor and drop the edge unless it belongs to the uni's own
	// geo_code country. Universities without a resolvable geo_code country are
	// left untouched (no reliable anchor).
	edgeRows, err := database.Reader.Query(ctx, `
		SELECT sps.space_id, sps.parent_space_id
		FROM space_parent_spaces sps
		JOIN spaces sp ON sp.id = sps.parent_space_id
		JOIN spaces su ON su.id = sps.space_id
		JOIN space_types st ON st.id = su.space_type_id
		WHERE st.slug = 'university'
		  AND su.deleted_at IS NULL AND sp.deleted_at IS NULL
		  AND sp.level IN ('country', 'state', 'city')
	`)
	if err != nil {
		slog.Error("failed to query existing geo edges", "error", err)
		os.Exit(1)
	}

	var edges []link

	parentSet := map[pgtype.UUID]struct{}{}

	for edgeRows.Next() {
		var e link
		if err := edgeRows.Scan(&e.spaceID, &e.parentID); err != nil {
			continue
		}

		edges = append(edges, e)
		parentSet[e.parentID] = struct{}{}
	}

	edgeRows.Close()

	parentIDsAll := make([]pgtype.UUID, 0, len(parentSet))
	for pid := range parentSet {
		parentIDsAll = append(parentIDsAll, pid)
	}

	// Resolve each parent space's country ancestor (a country resolves to
	// itself).
	parentCountry := map[pgtype.UUID]pgtype.UUID{}

	if len(parentIDsAll) > 0 {
		anc, err := q.GetSpaceCountryAncestorIDs(ctx, parentIDsAll)
		if err != nil {
			slog.Error("failed to resolve parent country ancestors", "error", err)
			os.Exit(1)
		}

		for _, r := range anc {
			parentCountry[r.SpaceID] = r.CountryID
		}
	}

	var stale []link

	for _, e := range edges {
		uc, anchored := uniCountry[e.spaceID]
		if !anchored {
			continue // no geo anchor — conservative keep
		}

		if parentCountry[e.parentID] == uc {
			continue // in-country ancestry
		}

		stale = append(stale, e)
	}

	slog.Info("purge analysis", "existing_geo_edges", len(edges), "foreign_edges", len(stale))

	if !*dryRun && len(stale) > 0 {
		err := db.WithTx(ctx, database.Writer, func(q *gen.Queries) error {
			for _, e := range stale {
				if err := q.RemoveParentSpace(ctx, gen.RemoveParentSpaceParams{
					SpaceID:       e.spaceID,
					ParentSpaceID: e.parentID,
				}); err != nil {
					return fmt.Errorf("removing link %s -> %s: %w", e.spaceID, e.parentID, err)
				}
			}

			return nil
		})
		if err != nil {
			slog.Error("purge failed — transaction rolled back", "error", err)
			os.Exit(1)
		}
	}

	fmt.Printf("\nDone: %d geo links %s across %d universities, %d non-geo matches skipped, %d foreign edges %s\n",
		inserted, map[bool]string{true: "proposed", false: "written"}[*dryRun], len(linkedUnis), skipped,
		len(stale), map[bool]string{true: "would be purged", false: "purged"}[*dryRun])
}
