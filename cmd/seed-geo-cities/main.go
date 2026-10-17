// Command seed-geo-cities bulk-seeds city-level geo spaces for every country.
//
// Source: GeoNames cities5000 dump — all cities with population > 5,000 or
// seats of administrative divisions (~70k cities worldwide), committed as
// db/seed/cities_5000.tsv alongside the admin1 code table
// db/seed/admin1_codes.tsv used to resolve each city's parent state.
//
// Ref: japaconnect-engineering/adr/ADR-020-geo-seeding-country-state-auto-city-manual.md
// (amended: major cities are bulk-seeded on admin request; smaller settlements
// remain admin-created on demand via the "create city space" action).
//
// Parent resolution, per city:
//  1. State space in the same country whose normalized name matches the
//     GeoNames admin1 NAME (exact, space-collapsed, or unique token-subset).
//  2. State space whose geo_code equals the GeoNames admin1 code — only for
//     lettered ISO 3166-2-style codes (GeoNames mirrors ISO there), or
//     numeric codes corroborated by the admin1 name. Bare numeric GeoNames
//     codes use a different numbering than the ISO codes seeded on state
//     spaces and are otherwise untrusted.
//  3. The country space itself (states missing or unmatched).
//
// The space gets parent_id + a space_parent_spaces junction row to the same
// parent (the pattern CreateCity uses), level='city', and geo_code set to the
// ISO country code so the feed's country-code geo boost reaches city rows.
//
// Idempotent: a city is skipped when a live city space with the same
// normalized name already exists under the resolved parent scope (or under
// the country when the new row would land at country scope), and slugs are
// disambiguated deterministically (-cc, -cc-admin1, -N) against every live
// space slug. A fix-up pass also backfills NULL geo_code on pre-existing city
// spaces from their parent chain.
//
// Usage:
//
//	go run ./cmd/seed-geo-cities -dry-run   # report only
//	go run ./cmd/seed-geo-cities            # seed
package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/worker"
)

const insertSpaceSQL = `INSERT INTO spaces (id, parent_id, space_type_id, name, slug, geo_code, level,
	visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
	VALUES ($1,$2,$3,$4,$5,$6,'city','public','members','members','members','open','active')`

const insertParentSQL = `INSERT INTO space_parent_spaces (space_id, parent_space_id)
	VALUES ($1,$2) ON CONFLICT DO NOTHING`

// cityRow is one line of db/seed/cities_5000.tsv.
type cityRow struct {
	name       string
	asciiName  string
	country    string
	admin1     string
	population int64
}

type geoRow struct {
	id       pgtype.UUID
	name     string
	slug     string
	geoCode  string
	level    string
	parentID pgtype.UUID
}

var stripMarks = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// compatFold maps letters that have no NFD decomposition to an ASCII
// equivalent (ı, đ, ø, ł, …) — the diacritic strip alone leaves them as
// non-ASCII letters that then fail to match the GeoNames ASCII spellings
// ("Balikesir" vs seeded "Balıkesir").
var compatFold = strings.NewReplacer(
	"ı", "i", "İ", "i",
	"đ", "d", "Đ", "d",
	"ø", "o", "Ø", "o",
	"ð", "d", "Ð", "d",
	"þ", "th", "Þ", "th",
	"ł", "l", "Ł", "l",
	"æ", "ae", "Æ", "ae",
	"œ", "oe", "Œ", "oe",
	"ß", "ss", "ẞ", "ss",
	"ŋ", "n", "Ŋ", "n",
	"ħ", "h", "Ħ", "h",
	"ə", "e", "Ə", "e",
	"ș", "s", "Ș", "s",
	"ț", "t", "Ț", "t",
	"ŭ", "u", "Ŭ", "u",
)

// asciiFold strips diacritics and folds non-decomposing letters to ASCII.
func asciiFold(s string) string {
	out, _, err := transform.String(stripMarks, compatFold.Replace(s))
	if err != nil {
		return compatFold.Replace(s)
	}

	return out
}

// normName lowercases, strips diacritics, and reduces a name to a single
// space-joined token string so dataset and seeded names compare cleanly.
func normName(s string) string {
	var b strings.Builder

	for _, r := range strings.ToLower(asciiFold(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}

	return strings.Join(strings.Fields(b.String()), " ")
}

// adminTypeWords are generic administrative-type tokens removed before
// comparing GeoNames admin1 names to seeded state names.
var adminTypeWords = map[string]struct{}{
	"state": {}, "province": {}, "provinces": {}, "region": {}, "regional": {},
	"county": {}, "counties": {}, "district": {}, "governorate": {}, "muhafazah": {},
	"department": {}, "departamento": {}, "prefecture": {}, "territory": {},
	"territories": {}, "emirate": {}, "oblast": {}, "krai": {}, "kray": {},
	"kraj": {}, "canton": {}, "municipality": {}, "voivodeship": {}, "island": {},
	"islands": {}, "division": {}, "city": {}, "autonomous": {}, "republic": {},
	"federal": {}, "area": {}, "metropolitan": {}, "districto": {}, "estado": {},
	"of": {}, "the": {}, "and": {}, "de": {}, "la": {}, "du": {}, "des": {}, "del": {},
}

// tokenSet returns normName tokens minus administrative-type words.
func tokenSet(norm string) map[string]struct{} {
	set := map[string]struct{}{}

	for t := range strings.FieldsSeq(norm) {
		if _, skip := adminTypeWords[t]; !skip {
			set[t] = struct{}{}
		}
	}

	return set
}

// subset reports whether a ⊆ b (empty sets never match).
func subset(a, b map[string]struct{}) bool {
	if len(a) == 0 || len(a) > len(b) {
		return false
	}

	for t := range a {
		if _, ok := b[t]; !ok {
			return false
		}
	}

	return true
}

// slugify builds a URL slug from a city name: lowercase, diacritics stripped,
// non-alphanumeric runs collapsed to single hyphens.
func slugify(s string) string {
	out := asciiFold(s)

	var b strings.Builder

	b.Grow(len(out))

	dash := false

	for _, r := range strings.ToLower(out) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', unicode.IsLetter(r):
			b.WriteRune(r)

			dash = false
		default:
			if b.Len() > 0 && !dash {
				b.WriteByte('-')
			}

			dash = true
		}
	}

	return strings.TrimRight(b.String(), "-")
}

func main() {
	dryRun := flag.Bool("dry-run", false, "report proposed inserts without writing")
	file := flag.String("file", "db/seed/cities_5000.tsv", "GeoNames cities TSV (geonameid, name, asciiname, country, admin1, population)")
	admin1File := flag.String("admin1", "db/seed/admin1_codes.tsv", "GeoNames admin1 codes TSV (country, admin1, name, asciiname)")
	aliasFile := flag.String("alias", "db/seed/admin1_alias.tsv", "admin1 name overrides TSV (country, admin1, seeded-state-name) — wins over admin1_codes")
	batchSize := flag.Int("batch", 500, "inserts per transaction")
	populateTop := flag.Int("populate-top", 0,
		"after seeding, enqueue space:auto_populate for the N highest-population cities per country (0 = off). Covers cities inserted this run; pre-existing cities are not re-enqueued.")
	flag.Parse()

	ctx := context.Background()

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

	admin1Name, err := loadAdmin1(*admin1File)
	if err != nil {
		slog.Error("failed to load admin1 codes", "error", err)
		os.Exit(1)
	}
	// Hand-maintained overrides win over the GeoNames names for codes whose
	// official admin1 name differs from the seeded state name (e.g. GeoNames
	// "FCT" vs seeded "Federal Capital Territory").
	if err := loadAdmin1Aliases(*aliasFile, admin1Name); err != nil {
		if !os.IsNotExist(err) {
			slog.Error("failed to load admin1 aliases", "error", err)
			os.Exit(1)
		}
	}

	cityTypeID, err := lookupCityTypeID(ctx, database)
	if err != nil {
		slog.Error("failed to resolve city space type", "error", err)
		os.Exit(1)
	}

	slugTaken, err := liveSlugs(ctx, database)
	if err != nil {
		slog.Error("failed to load space slugs", "error", err)
		os.Exit(1)
	}

	// ── Load existing geo hierarchy ────────────────────────────────────
	geoRows, err := database.Reader.Query(ctx, `
		SELECT id, name, slug, COALESCE(geo_code, ''), level, parent_id
		FROM spaces
		WHERE level IN ('country','state','city') AND deleted_at IS NULL
	`)
	if err != nil {
		slog.Error("failed to load geo spaces", "error", err)
		os.Exit(1)
	}

	var geo []geoRow

	for geoRows.Next() {
		var g geoRow
		if err := geoRows.Scan(&g.id, &g.name, &g.slug, &g.geoCode, &g.level, &g.parentID); err != nil {
			slog.Warn("skipping geo row: scan error", "error", err)
			continue
		}

		geo = append(geo, g)
	}

	geoRows.Close()

	if err := geoRows.Err(); err != nil {
		slog.Error("geo row iteration failed", "error", err)
		os.Exit(1)
	}

	countryByCode := map[string]pgtype.UUID{} // upper(ISO) -> country id
	countryOf := map[pgtype.UUID]pgtype.UUID{}
	levelByID := map[pgtype.UUID]string{}
	statesByID := map[pgtype.UUID]geoRow{}

	var existingCities []geoRow

	for _, g := range geo {
		levelByID[g.id] = g.level

		switch g.level {
		case "country":
			countryByCode[strings.ToUpper(g.geoCode)] = g.id
			countryOf[g.id] = g.id
		case "state":
			statesByID[g.id] = g
		case "city":
			existingCities = append(existingCities, g)
		}
	}

	// Index states under their country (state.parent_id is the country).
	statesByCode := map[pgtype.UUID]map[string]pgtype.UUID{}    // country -> lower(geo_code) -> state
	statesByNorm := map[pgtype.UUID]map[string]pgtype.UUID{}    // country -> normName -> state
	statesBySqueeze := map[pgtype.UUID]map[string]pgtype.UUID{} // country -> normName without spaces -> state
	stateTokens := map[pgtype.UUID]map[pgtype.UUID]map[string]struct{}{}
	countryGeoCode := map[pgtype.UUID]string{} // country id -> ISO code

	for _, g := range geo {
		if g.level == "country" && g.geoCode != "" {
			countryGeoCode[g.id] = g.geoCode
		}
	}

	for _, st := range statesByID {
		cid := st.parentID
		if !cid.Valid || levelByID[cid] != "country" {
			continue
		}

		countryOf[st.id] = cid

		if statesByCode[cid] == nil {
			statesByCode[cid] = map[string]pgtype.UUID{}
			statesByNorm[cid] = map[string]pgtype.UUID{}
			statesBySqueeze[cid] = map[string]pgtype.UUID{}
			stateTokens[cid] = map[pgtype.UUID]map[string]struct{}{}
		}

		if st.geoCode != "" {
			statesByCode[cid][strings.ToLower(st.geoCode)] = st.id
		}

		norm := normName(st.name)
		if _, dup := statesByNorm[cid][norm]; !dup {
			statesByNorm[cid][norm] = st.id
		}

		sq := strings.ReplaceAll(norm, " ", "")
		if _, dup := statesBySqueeze[cid][sq]; !dup {
			statesBySqueeze[cid][sq] = st.id
		}

		stateTokens[cid][st.id] = tokenSet(norm)
	}

	// Dedup indexes over existing city spaces:
	//   scopeKey — "parentID|norm" for each city (exact-scope dedup)
	//   countryWide — "countryID|norm" for every city, resolved through its
	//     parent chain; consulted only when a new city would land directly on
	//     the country, so same-named cities in different states still seed.
	scopeKey := map[string]struct{}{}
	countryWide := map[string]struct{}{}

	var needGeoCode []geoRow

	for _, c := range existingCities {
		if c.parentID.Valid {
			scopeKey[c.parentID.String()+"|"+normName(c.name)] = struct{}{}

			if cid, ok := countryOf[c.parentID]; ok {
				countryWide[cid.String()+"|"+normName(c.name)] = struct{}{}
				countryOf[c.id] = cid
			} else if levelByID[c.parentID] == "country" {
				countryWide[c.parentID.String()+"|"+normName(c.name)] = struct{}{}
				countryOf[c.id] = c.parentID
			}
		}

		if c.geoCode == "" {
			needGeoCode = append(needGeoCode, c)
		}
	}

	slog.Info("geo hierarchy loaded",
		"countries", len(countryByCode),
		"states", len(statesByID),
		"existing_cities", len(existingCities))

	// ── Load and dedup the dataset ─────────────────────────────────────
	cities, err := loadCities(*file)
	if err != nil {
		slog.Error("failed to load cities", "error", err)
		os.Exit(1)
	}

	slog.Info("dataset loaded", "rows", len(cities))

	// ── Reparent pass ────────────────────────────────────────────────
	// Re-resolve every live dataset-backed city. A city pinned to the wrong
	// parent by an earlier run (e.g. a GeoNames/ISO numeric admin1 code
	// collision) is moved to the state the current resolver picks, and its
	// space_parent_spaces edge repointed. scopeKey is updated in lockstep so
	// the moved row still dedups its own dataset row and frees its old scope.
	// Rows whose dataset variants resolve to different parents, or whose new
	// scope already holds a same-named live city, are left in place.
	type reparentOp struct {
		id, oldParent, newParent pgtype.UUID
	}

	var reparents []reparentOp

	var retired []geoRow

	var skippedScope int

	byCityKey := map[string][]cityRow{}

	for _, d := range cities {
		k := d.country + "|" + normName(d.name)
		byCityKey[k] = append(byCityKey[k], d)
	}

	for _, c := range existingCities {
		rows := byCityKey[c.geoCode+"|"+normName(c.name)]
		if len(rows) == 0 {
			continue // admin-created city, not dataset-managed
		}

		if pinned := pinAdmin1(rows, c.slug, c.geoCode); len(pinned) > 0 {
			rows = pinned
		}

		targets := map[pgtype.UUID]struct{}{}

		var only pgtype.UUID

		for _, d := range rows {
			cid, ok := countryByCode[d.country]
			if !ok {
				continue
			}

			p := cid
			if sid, ok := resolveState(d, cid, admin1Name, statesByCode, statesByNorm, statesBySqueeze, stateTokens, statesByID); ok {
				p = sid
			}

			targets[p] = struct{}{}
			only = p
		}

		if len(targets) != 1 {
			// Ambiguous variant set: keep the row only if its current parent
			// is one of the valid targets; otherwise it is a misattached
			// variant we cannot identify — retire it and let the insert
			// planner re-create the canonical rows at their resolved scopes.
			if len(targets) > 1 {
				if _, valid := targets[c.parentID]; valid {
					continue
				}

				delete(scopeKey, c.parentID.String()+"|"+normName(c.name))
				retired = append(retired, c)
			}

			continue
		}

		if only == c.parentID {
			continue
		}

		norm := normName(c.name)
		oldKey := c.parentID.String() + "|" + norm

		newKey := only.String() + "|" + norm
		if _, occupied := scopeKey[newKey]; occupied {
			skippedScope++

			slog.Info("reparent skipped: target scope already holds same-named city",
				"slug", c.slug, "id", c.id.String())

			continue
		}

		delete(scopeKey, oldKey)
		scopeKey[newKey] = struct{}{}

		reparents = append(reparents, reparentOp{id: c.id, oldParent: c.parentID, newParent: only})
		c.parentID = only
	}

	slog.Info("reparent plan", "reparents", len(reparents), "retired", len(retired),
		"skipped_scope_occupied", skippedScope)

	// ── Resolve parents and plan inserts ───────────────────────────────
	type insert struct {
		id       pgtype.UUID
		parentID pgtype.UUID
		name     string
		slug     string
		geoCode  string
		pop      int64
	}

	var planned []insert

	var skippedExisting, skippedCountry, parentedState, parentedCountry int

	for _, c := range cities {
		countryID, ok := countryByCode[c.country]
		if !ok {
			skippedCountry++
			continue
		}

		parentID := countryID
		if sid, ok := resolveState(c, countryID, admin1Name, statesByCode, statesByNorm, statesBySqueeze, stateTokens, statesByID); ok {
			parentID = sid
		}

		norm := normName(c.name)

		key := parentID.String() + "|" + norm
		if _, dup := scopeKey[key]; dup {
			skippedExisting++
			continue
		}
		// Country-parented rows must not duplicate a city that already exists
		// anywhere under the country; state-parented rows must not duplicate a
		// city that sits directly on the country.
		if levelByID[parentID] == "country" {
			if _, dup := countryWide[key]; dup {
				skippedExisting++
				continue
			}

			parentedCountry++
		} else {
			if _, dup := scopeKey[countryID.String()+"|"+norm]; dup {
				skippedExisting++
				continue
			}

			parentedState++
		}

		base := c.asciiName
		if base == "" {
			base = c.name
		}

		slug := uniqueSlug(slugify(base), c.country, c.admin1, slugTaken)
		if slug == "" {
			slog.Warn("no slug derivable", "city", c.name, "country", c.country)
			continue
		}

		slugTaken[slug] = struct{}{}
		scopeKey[key] = struct{}{}
		countryWide[countryID.String()+"|"+norm] = struct{}{}

		id := uuid.New()
		planned = append(planned, insert{
			id:       pgtype.UUID{Bytes: id, Valid: true},
			parentID: parentID,
			name:     c.name,
			slug:     slug,
			geoCode:  c.country,
			pop:      c.population,
		})
	}

	slog.Info("plan complete",
		"planned_inserts", len(planned),
		"skipped_existing", skippedExisting,
		"skipped_unknown_country", skippedCountry,
		"parented_to_state", parentedState,
		"parented_to_country", parentedCountry)

	// geo_code fix-up count for pre-existing cities (resolved via parent chain).
	geoFixable := 0

	for _, c := range needGeoCode {
		if cid, ok := countryOf[c.parentID]; ok && countryGeoCode[cid] != "" {
			geoFixable++
		}
	}

	if *dryRun {
		for _, p := range planned {
			fmt.Printf("plan %s\t%s\t%s\tparent=%s\n", p.geoCode, p.name, p.slug, levelByID[p.parentID])
		}

		for _, r := range retired {
			fmt.Printf("retire %s\t%s\t%s\n", r.geoCode, r.name, r.slug)
		}

		fmt.Printf("\ndry-run: %d city spaces would be created (%d to state, %d to country), %d reparented, %d retired (ambiguous variants), %d scope-occupied skipped, %d skipped existing, %d unknown-country rows skipped, %d existing cities need geo_code backfill\n",
			len(planned), parentedState, parentedCountry, len(reparents), len(retired), skippedScope, skippedExisting, skippedCountry, geoFixable)

		return
	}

	// ── Retire ambiguous misattached rows ───────────────────────────────
	for start := 0; start < len(retired); start += *batchSize {
		end := min(start+*batchSize, len(retired))
		chunk := retired[start:end]

		tx, err := database.Writer.Begin(ctx)
		if err != nil {
			slog.Error("begin retire tx failed", "error", err)
			os.Exit(1)
		}

		batch := &pgx.Batch{}
		for _, r := range chunk {
			batch.Queue(`UPDATE spaces SET status = 'deleted', deleted_at = now(), updated_at = now() WHERE id = $1`, r.id)
		}

		br := tx.SendBatch(ctx, batch)

		var batchErr error

		for range batch.Len() {
			if _, err := br.Exec(); err != nil && batchErr == nil {
				batchErr = err
			}
		}

		if cerr := br.Close(); cerr != nil && batchErr == nil {
			batchErr = cerr
		}

		if batchErr != nil {
			_ = tx.Rollback(ctx)

			slog.Error("retire batch failed — rolled back", "error", batchErr, "chunk_start", start)
			os.Exit(1)
		}

		if err := tx.Commit(ctx); err != nil {
			slog.Error("retire commit failed", "error", err, "chunk_start", start)
			os.Exit(1)
		}
	}

	// ── Apply reparents in chunked transactions ─────────────────────────
	reparented := 0

	for start := 0; start < len(reparents); start += *batchSize {
		end := min(start+*batchSize, len(reparents))
		chunk := reparents[start:end]

		tx, err := database.Writer.Begin(ctx)
		if err != nil {
			slog.Error("begin reparent tx failed", "error", err)
			os.Exit(1)
		}

		batch := &pgx.Batch{}
		for _, r := range chunk {
			batch.Queue(`UPDATE spaces SET parent_id = $2, updated_at = now() WHERE id = $1`, r.id, r.newParent)
			batch.Queue(`DELETE FROM space_parent_spaces WHERE space_id = $1 AND parent_space_id = $2`, r.id, r.oldParent)
			batch.Queue(insertParentSQL, r.id, r.newParent)
		}

		br := tx.SendBatch(ctx, batch)

		var batchErr error

		for range batch.Len() {
			if _, err := br.Exec(); err != nil && batchErr == nil {
				batchErr = err
			}
		}

		if cerr := br.Close(); cerr != nil && batchErr == nil {
			batchErr = cerr
		}

		if batchErr != nil {
			_ = tx.Rollback(ctx)

			slog.Error("reparent batch failed — rolled back", "error", batchErr, "chunk_start", start)
			os.Exit(1)
		}

		if err := tx.Commit(ctx); err != nil {
			slog.Error("reparent commit failed", "error", err, "chunk_start", start)
			os.Exit(1)
		}

		reparented += len(chunk)
	}

	// ── Insert in chunked transactions ─────────────────────────────────
	inserted := 0

	for start := 0; start < len(planned); start += *batchSize {
		end := min(start+*batchSize, len(planned))
		chunk := planned[start:end]

		tx, err := database.Writer.Begin(ctx)
		if err != nil {
			slog.Error("begin tx failed", "error", err)
			os.Exit(1)
		}

		batch := &pgx.Batch{}
		for _, p := range chunk {
			batch.Queue(insertSpaceSQL, p.id, p.parentID, cityTypeID, p.name, p.slug, p.geoCode)
			batch.Queue(insertParentSQL, p.id, p.parentID)
		}

		br := tx.SendBatch(ctx, batch)

		var batchErr error

		for range batch.Len() {
			if _, err := br.Exec(); err != nil && batchErr == nil {
				batchErr = err
			}
		}

		if cerr := br.Close(); cerr != nil && batchErr == nil {
			batchErr = cerr
		}

		if batchErr != nil {
			_ = tx.Rollback(ctx)

			slog.Error("insert batch failed — rolled back", "error", batchErr, "chunk_start", start)
			os.Exit(1)
		}

		if err := tx.Commit(ctx); err != nil {
			slog.Error("commit failed", "error", err, "chunk_start", start)
			os.Exit(1)
		}

		inserted += len(chunk)
		slog.Info("insert progress", "inserted", inserted, "of", len(planned))
	}

	// Backfill geo_code on pre-existing cities that had none.
	geoFixed := 0

	for _, c := range needGeoCode {
		cid, ok := countryOf[c.parentID]
		if !ok || !cid.Valid {
			continue
		}

		code := countryGeoCode[cid]
		if code == "" {
			continue
		}

		if _, err := database.Writer.Exec(ctx,
			`UPDATE spaces SET geo_code = $1, updated_at = now() WHERE id = $2 AND (geo_code IS NULL OR geo_code = '')`,
			code, c.id); err != nil {
			slog.Warn("geo_code backfill failed", "city", c.name, "error", err)
			continue
		}

		geoFixed++
	}

	// --populate-top: enqueue space:auto_populate for the N most populous
	// cities per country so their header/logo media land without hitting
	// every city (Wikipedia rate-limits — see worker space:auto_populate
	// throttle note). Off by default; only cities inserted this run are
	// enqueued — pre-existing city rows aren't tracked here.
	enqueued := 0

	if *populateTop > 0 && !*dryRun && len(planned) > 0 {
		byCountry := make(map[string][]insert)
		for _, p := range planned {
			byCountry[p.geoCode] = append(byCountry[p.geoCode], p)
		}

		asynqClient := worker.NewClient(worker.DefaultConfig())
		defer asynqClient.Close()

		for country, list := range byCountry {
			slices.SortFunc(list, func(a, b insert) int {
				switch {
				case a.pop > b.pop:
					return -1
				case a.pop < b.pop:
					return 1
				default:
					return 0
				}
			})

			for i := 0; i < *populateTop && i < len(list); i++ {
				payload, _ := json.Marshal(map[string]string{
					"version":  "1",
					"space_id": list[i].id.String(),
				})

				// TaskOpts carries the task's registered defaults (Unique dedup
				// on repeat space_id enqueues, 2m timeout, 10 max retries) — a
				// bare Queue() opt would silently drop them to asynq defaults.
				if _, err := asynqClient.EnqueueContext(ctx,
					asynq.NewTask(worker.TypeSpaceAutoPopulate, payload, worker.TaskOpts(worker.TypeSpaceAutoPopulate)...)); err != nil {
					slog.Warn("populate-top enqueue failed", "city", list[i].name, "country", country, "error", err)
					continue
				}

				enqueued++
			}
		}

		slog.Info("populate-top enqueued auto_populate", "tasks", enqueued, "per_country_cap", *populateTop)
	}

	fmt.Printf("\nDone: %d city spaces created (%d to state, %d to country), %d reparented, %d retired, %d scope-occupied skipped, %d skipped existing, %d unknown-country rows skipped, %d existing cities geo_code backfilled\n",
		inserted, parentedState, parentedCountry, reparented, len(retired), skippedScope, skippedExisting, skippedCountry, geoFixed)
}

// lookupCityTypeID returns the 'city' space_types id.
func lookupCityTypeID(ctx context.Context, database *db.DB) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := database.Reader.QueryRow(ctx,
		`SELECT id FROM space_types WHERE slug = 'city'`).Scan(&id)

	return id, err
}

// liveSlugs returns every slug held by a non-deleted space (the unique index
// scope) so generated slugs never collide.
func liveSlugs(ctx context.Context, database *db.DB) (map[string]struct{}, error) {
	rows, err := database.Reader.Query(ctx, `SELECT slug FROM spaces WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	set := map[string]struct{}{}

	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			continue
		}

		set[s] = struct{}{}
	}

	return set, rows.Err()
}

// resolveState finds the state space for a city inside its country.
//
// Name matching comes first: the GeoNames admin1 NAME is compared to seeded
// state names (normalized, then space-collapsed, then a unique token-subset
// match). The admin1 CODE is only trusted when it contains a letter
// (ISO 3166-2-style codes like 'ENG', 'JB', 'BY' — GeoNames mirrors ISO
// there), when the country has no admin1-name mapping to corroborate
// against, or when the code-matched state's own name corroborates the
// admin1 name. A bare numeric GeoNames admin1 code that fails corroboration
// is ignored: GeoNames numbers admin1s in a different order than the ISO
// 3166-2 numeric codes seeded on state spaces (e.g. GeoNames JP.01=Aichi
// but ISO JP-01=Hokkaidō), so an uncorroborated numeric match is usually
// the WRONG state.
func resolveState(c cityRow, countryID pgtype.UUID,
	admin1Name map[string]string,
	byCode map[pgtype.UUID]map[string]pgtype.UUID,
	byNorm map[pgtype.UUID]map[string]pgtype.UUID,
	bySqueeze map[pgtype.UUID]map[string]pgtype.UUID,
	tokens map[pgtype.UUID]map[pgtype.UUID]map[string]struct{},
	statesByID map[pgtype.UUID]geoRow,
) (pgtype.UUID, bool) {
	if c.admin1 == "" {
		return pgtype.UUID{}, false
	}

	name := admin1Name[c.country+"."+c.admin1]
	if name != "" {
		if id, ok := matchStateName(name, byNorm[countryID], bySqueeze[countryID], tokens[countryID]); ok {
			return id, true
		}
	}

	id, ok := byCode[countryID][strings.ToLower(c.admin1)]
	if !ok {
		return pgtype.UUID{}, false
	}

	if hasLetter(c.admin1) || name == "" || namesAgree(name, statesByID[id].name) {
		return id, true
	}

	return pgtype.UUID{}, false
}

// matchStateName resolves an admin1 NAME to a state: exact normalized match,
// then space-collapsed match ("North-Western" == "Northwestern"), then a
// unique token-subset match. Ambiguous subset hits resolve to nothing.
func matchStateName(name string,
	byNorm map[string]pgtype.UUID,
	bySqueeze map[string]pgtype.UUID,
	tokens map[pgtype.UUID]map[string]struct{},
) (pgtype.UUID, bool) {
	norm := normName(name)
	if id, ok := byNorm[norm]; ok {
		return id, true
	}

	if id, ok := bySqueeze[strings.ReplaceAll(norm, " ", "")]; ok {
		return id, true
	}

	want := tokenSet(norm)

	var hit pgtype.UUID

	found := false

	for sid, set := range tokens {
		if subset(set, want) || subset(want, set) {
			if found {
				return pgtype.UUID{}, false
			}

			hit, found = sid, true
		}
	}

	return hit, found
}

// pinAdmin1 narrows same-named dataset rows to the admin1 variant encoded in
// a collision slug ("-cc-admin1" or "-cc-admin1-N" suffix). Returns nil when
// no single variant can be pinned.
func pinAdmin1(rows []cityRow, slug, cc string) []cityRow {
	if len(rows) < 2 {
		return nil
	}

	marker := "-" + strings.ToLower(cc) + "-"

	i := strings.Index(slug, marker)
	if i < 0 {
		return nil
	}

	tail := slug[i+len(marker):]

	var out []cityRow

	for _, d := range rows {
		a := strings.ToLower(d.admin1)
		if tail == a || strings.HasPrefix(tail, a+"-") {
			out = append(out, d)
		}
	}

	return out
}

func hasLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}

	return false
}

// namesAgree reports whether an admin1 name and a state name plausibly refer
// to the same division — used to corroborate numeric admin1 code matches.
func namesAgree(adminName, stateName string) bool {
	a, s := normName(adminName), normName(stateName)
	if a == s || strings.ReplaceAll(a, " ", "") == strings.ReplaceAll(s, " ", "") {
		return true
	}

	at, st := tokenSet(a), tokenSet(s)

	return subset(at, st) || subset(st, at)
}

// uniqueSlug picks a free slug, suffixing deterministically on collision.
func uniqueSlug(base, country, admin1 string, taken map[string]struct{}) string {
	if base == "" {
		return ""
	}

	if _, ok := taken[base]; !ok {
		return base
	}

	cc := strings.ToLower(country)
	for _, cand := range []string{base + "-" + cc, base + "-" + cc + "-" + strings.ToLower(admin1)} {
		if _, ok := taken[cand]; !ok {
			return cand
		}
	}

	for i := 2; i < 100; i++ {
		cand := fmt.Sprintf("%s-%s-%s-%d", base, cc, strings.ToLower(admin1), i)
		if _, ok := taken[cand]; !ok {
			return cand
		}
	}

	return ""
}

// loadCities parses the TSV and dedups rows by (country, admin1, normalized
// name), keeping the highest-population row.
func loadCities(path string) ([]cityRow, error) {
	f, err := os.Open(path) // #nosec G304 -- operator-provided dataset path in a seed tool
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.Comma = '\t'
	r.Comment = '#'
	r.FieldsPerRecord = -1
	r.ReuseRecord = true

	type key struct {
		country, admin1, norm string
	}

	seen := map[key]int{}

	var out []cityRow

	recs, err := r.ReadAll()
	if err != nil {
		return nil, err
	}

	for _, rec := range recs {
		if len(rec) < 6 {
			continue
		}

		pop, _ := strconv.ParseInt(rec[5], 10, 64)

		c := cityRow{
			name:       strings.TrimSpace(rec[1]),
			asciiName:  strings.TrimSpace(rec[2]),
			country:    strings.TrimSpace(rec[3]),
			admin1:     strings.TrimSpace(rec[4]),
			population: pop,
		}
		if c.name == "" || c.country == "" {
			continue
		}

		k := key{c.country, c.admin1, normName(c.name)}
		if i, dup := seen[k]; dup {
			if c.population > out[i].population {
				out[i] = c
			}

			continue
		}

		seen[k] = len(out)
		out = append(out, c)
	}

	slices.SortFunc(out, func(a, b cityRow) int {
		if a.country != b.country {
			return strings.Compare(a.country, b.country)
		}

		return strings.Compare(a.name, b.name)
	})

	return out, nil
}

// loadAdmin1Aliases merges "country\tadmin1\tseeded-state-name" overrides into
// the admin1-name map. Missing file is fine — overrides are optional.
func loadAdmin1Aliases(path string, into map[string]string) error {
	f, err := os.Open(path) // #nosec G304 -- operator-provided dataset path in a seed tool
	if err != nil {
		return err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.Comma = '\t'
	r.Comment = '#'
	r.FieldsPerRecord = -1

	recs, err := r.ReadAll()
	if err != nil {
		return err
	}

	for _, rec := range recs {
		if len(rec) < 3 {
			continue
		}

		into[strings.TrimSpace(rec[0])+"."+strings.TrimSpace(rec[1])] = strings.TrimSpace(rec[2])
	}

	return nil
}

// loadAdmin1 parses "country\tadmin1\tname\tasciiname" into "CC.A1" -> name,
// preferring the ASCII variant.
func loadAdmin1(path string) (map[string]string, error) {
	f, err := os.Open(path) // #nosec G304 -- operator-provided dataset path in a seed tool
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.Comma = '\t'
	r.Comment = '#'
	r.FieldsPerRecord = -1

	m := map[string]string{}

	recs, err := r.ReadAll()
	if err != nil {
		return nil, err
	}

	for _, rec := range recs {
		if len(rec) < 3 {
			continue
		}

		name := strings.TrimSpace(rec[2])
		if len(rec) >= 4 && strings.TrimSpace(rec[3]) != "" {
			name = strings.TrimSpace(rec[3])
		}

		m[strings.TrimSpace(rec[0])+"."+strings.TrimSpace(rec[1])] = name
	}

	return m, nil
}
