// The university domain projects an intel_record onto a spaces row
// (space_types.slug='education', level='university') plus the
// custom_field_values of the seeded university_fields group (§39). The
// university_intel_links table ties the space to the intel record and
// carries the field-level editorial_overrides set — the same
// human-edit-wins invariant every projection promoter enforces.
//
// Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md
package promoter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/extractor"
	"github.com/jedafe/japa/internal/intelligence/stores"
	"github.com/jedafe/japa/internal/universities"
)

// universitySpaceType is the space_types.slug universities live on
// (migration 000970 moved them off the retired 'university' type); matches
// universitySpaceTypeSlug in internal/universities.
const universitySpaceType = "education"

// universityLevel is the spaces.level marking a platform university.
const universityLevel = "university"

// universityFieldGroupSlug is the seeded custom_field_groups slug holding
// the university field definitions (§39).
const universityFieldGroupSlug = "university_fields"

// cfvEntityType is the custom_field_values.entity_type for space-scoped
// values (matches universities.customFieldEntityTypeSpace).
const cfvEntityType = "space"

// universityCFVKeys is the set of extraction field names that map 1:1 onto
// university_fields custom-field keys. Everything else in the schema
// (name, slug, country, city, description, source_url) maps to space
// columns or is unused.
var universityCFVKeys = []string{
	"initials", "official_website_url", "contact_email", "accreditation_body",
	"world_ranking", "country_ranking", "founded_year", "type",
	"programs_offered", "tuition_range", "student_population",
	"acceptance_rate", "international_support", "scholarship_available",
	"application_url",
}

// UniversityStore is the subset of *stores.Store the university promoter
// needs.
type UniversityStore interface {
	PersonaResolver
	GetIntelRecord(ctx context.Context, id pgtype.UUID) (gen.IntelRecord, error)
	GetIntelSource(ctx context.Context, id pgtype.UUID) (gen.IntelSource, error)
	GetUniversitySpaceBySlug(ctx context.Context, slug string) (gen.GetUniversitySpaceBySlugRow, error)
	GetUniversityIntelLinkByIntelRecord(ctx context.Context, intelRecordID pgtype.UUID) (gen.UniversityIntelLink, error)
	GetUniversityIntelLinkBySpace(ctx context.Context, spaceID pgtype.UUID) (gen.UniversityIntelLink, error)
	GetSpaceTypeBySlug(ctx context.Context, slug string) (gen.SpaceType, error)
	GetSpaceByID(ctx context.Context, id pgtype.UUID) (gen.GetSpaceByIDRow, error)
	GetCountrySpace(ctx context.Context, countryCode string) (pgtype.UUID, error)
	GetCustomFieldGroupBySlug(ctx context.Context, slug string) (gen.CustomFieldGroup, error)
	ListCustomFieldsByGroup(ctx context.Context, groupID pgtype.UUID) ([]gen.ListCustomFieldsByGroupRow, error)
	ListCustomFieldValuesByEntity(ctx context.Context, arg gen.ListCustomFieldValuesByEntityParams) ([]gen.CustomFieldValue, error)
	UpdateIntelRecordStatus(ctx context.Context, arg gen.UpdateIntelRecordStatusParams) error
	WithTx(ctx context.Context, fn func(UniversityTxStore) error) error
}

// UniversityTxStore is the write-side subset used inside the promote
// transaction so multi-table writes stay atomic (§6.2).
type UniversityTxStore interface {
	CreateSpace(ctx context.Context, arg gen.CreateSpaceParams) (gen.CreateSpaceRow, error)
	UpdateSpace(ctx context.Context, arg gen.UpdateSpaceParams) (gen.UpdateSpaceRow, error)
	UpsertCustomFieldValue(ctx context.Context, arg gen.UpsertCustomFieldValueParams) (gen.CustomFieldValue, error)
	UpsertUniversityIntelLink(ctx context.Context, arg gen.UpsertUniversityIntelLinkParams) (gen.UniversityIntelLink, error)
	AddUniversityIntelOverrides(ctx context.Context, arg gen.AddUniversityIntelOverridesParams) error
	UpdateIntelRecordStatus(ctx context.Context, arg gen.UpdateIntelRecordStatusParams) error
	CreateIntelReviewLog(ctx context.Context, arg gen.CreateIntelReviewLogParams) error
}

// SpaceAutoPopulateEnqueuer is the minimal interface for enqueueing the
// space:auto_populate task (§11 catalog) — the worker's Enqueuer satisfies
// it. Used to kick off Wikidata enrichment for promoter-created
// universities, same as the admin create path.
type SpaceAutoPopulateEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// taskSpaceAutoPopulate is the registered §11 task name for Wikidata
// space enrichment (worker.TypeSpaceAutoPopulate).
const taskSpaceAutoPopulate = "space:auto_populate"

// UniversityPromoter promotes university intel_records into a university
// space + university_fields custom field values, linked by a
// university_intel_links row. On the update path any field key present in
// the link's editorial_overrides is left untouched — a human edit always
// wins over a re-crawl.
type UniversityPromoter struct {
	store    UniversityStore
	enqueuer SpaceAutoPopulateEnqueuer // nil = no enrichment enqueue
	aiModel  *string                   // AI model used for extraction (ADR-088, INTEL-003)
	now      func() time.Time
}

// UniversityPromoterOption configures a UniversityPromoter.
type UniversityPromoterOption func(*UniversityPromoter)

// WithUniversityClock overrides the clock used for timestamps (tests).
func WithUniversityClock(fn func() time.Time) UniversityPromoterOption {
	return func(p *UniversityPromoter) { p.now = fn }
}

// WithUniversityAIModel sets the AI model name stamped on review logs.
func WithUniversityAIModel(model string) UniversityPromoterOption {
	return func(p *UniversityPromoter) { p.aiModel = &model }
}

// WithUniversityEnqueuer wires the Asynq enqueuer used to trigger
// space:auto_populate (Wikidata enrichment) for created universities.
func WithUniversityEnqueuer(e SpaceAutoPopulateEnqueuer) UniversityPromoterOption {
	return func(p *UniversityPromoter) { p.enqueuer = e }
}

// NewUniversityPromoter creates a promoter backed by the given store.
// authorID is the system user credited as the review-log actor.
func NewUniversityPromoter(store UniversityStore, authorID pgtype.UUID, opts ...UniversityPromoterOption) *UniversityPromoter {
	_ = authorID // the review-log actor is resolved per-source via persona; kept for signature parity
	p := &UniversityPromoter{store: store, aiModel: ptr("gpt-4o-mini"), now: time.Now}
	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Entity returns the entity_type this promoter handles.
func (p *UniversityPromoter) Entity() string { return "university" }

// ErrNotUniversity is returned when Promote is called on an intel record
// whose entity_type is not "university".
var ErrNotUniversity = errors.New("promoter: intel record is not a university")

// Promote maps the intel record identified by recordID to a university
// space + custom field values, creating it or updating it in place while
// respecting editorial overrides.
func (p *UniversityPromoter) Promote(ctx context.Context, recordID uuid.UUID) (PromoteResult, error) {
	return promoteDomain(ctx, p.store, recordID, domainPromoteOps[gen.UniversityIntelLink]{
		entityType: p.Entity(),
		notErr:     ErrNotUniversity,
		loadLabel:  "university intel link",
		getExisting: func(ctx context.Context, pgID pgtype.UUID) (gen.UniversityIntelLink, error) {
			return p.store.GetUniversityIntelLinkByIntelRecord(ctx, pgID)
		},
		createPath: p.universityCreatePath,
		updatePath: p.universityUpdatePath,
	})
}

// ---------------------------------------------------------------------------
// Field resolution + write shape
// ---------------------------------------------------------------------------

// universityFieldDef is the resolved custom_fields row the promoter needs
// to write one CFV.
type universityFieldDef struct {
	id        pgtype.UUID
	key       string
	fieldType string
	options   []byte
}

// resolveUniversityFieldDefs loads the university_fields group once and
// resolves every CFV-mapped field definition in a single query — admins
// can mutate defs, so this intentionally runs per-promote rather than
// being cached. Missing keys (field removed by an admin) are absent from
// the map, never an error — extraction still lands on the record.
func (p *UniversityPromoter) resolveUniversityFieldDefs(ctx context.Context) map[string]universityFieldDef {
	defs := map[string]universityFieldDef{}

	group, err := p.store.GetCustomFieldGroupBySlug(ctx, universityFieldGroupSlug)
	if err != nil {
		slog.Warn("promoter: university_fields group not resolvable, skipping CFV writes", "error", err)
		return defs
	}

	rows, err := p.store.ListCustomFieldsByGroup(ctx, group.ID)
	if err != nil {
		slog.Warn("promoter: university fields not listable, skipping CFV writes", "error", err)
		return defs
	}

	keys := make(map[string]struct{}, len(universityCFVKeys))
	for _, k := range universityCFVKeys {
		keys[k] = struct{}{}
	}

	for _, row := range rows {
		if _, ok := keys[row.Key]; !ok {
			continue
		}

		defs[row.Key] = universityFieldDef{id: row.ID, key: row.Key, fieldType: row.FieldType, options: row.Options}
	}

	return defs
}

// cfvWriteFor maps one extracted field to an UpsertCustomFieldValueParams
// writing to the column the field's type expects (§39 + wikifacts write
// shapes: select → {"value": v}, multiselect → bare array, boolean →
// value_bool, number → value_number, everything else → value_text).
// ok=false when the extraction produced no storable value.
func cfvWriteFor(def universityFieldDef, ext extractor.Extraction, entityID pgtype.UUID) (gen.UpsertCustomFieldValueParams, bool) {
	params := gen.UpsertCustomFieldValueParams{
		FieldID:    def.id,
		EntityType: cfvEntityType,
		EntityID:   entityID,
	}

	switch def.fieldType {
	case "number":
		num := numericField(ext, def.key)
		if !num.Valid {
			return params, false
		}

		params.ValueNumber = num
	case "boolean":
		b := boolField(ext, def.key)
		if b == nil {
			return params, false
		}

		params.ValueBool = b
	case "select":
		s := stringField(ext, def.key)
		if s == "" {
			return params, false
		}

		vj, err := json.Marshal(map[string]string{"value": s})
		if err != nil {
			return params, false
		}

		params.ValueJson = vj
	case "multiselect":
		vals := stringSliceField(ext, def.key)
		if len(vals) == 0 {
			return params, false
		}

		vj, err := json.Marshal(vals)
		if err != nil {
			return params, false
		}

		params.ValueJson = vj
	default: // text, url, email
		s := stringField(ext, def.key)
		if s == "" {
			return params, false
		}

		params.ValueText = &s
	}

	return params, true
}

// cfvStoredEquals reports whether a stored CFV already holds the value the
// write params would set — the diff check behind "updated" vs "verified".
func cfvStoredEquals(stored gen.CustomFieldValue, def universityFieldDef, want gen.UpsertCustomFieldValueParams) bool {
	switch def.fieldType {
	case "number":
		return numericEquals(stored.ValueNumber, want.ValueNumber)
	case "boolean":
		return stored.ValueBool != nil && want.ValueBool != nil && *stored.ValueBool == *want.ValueBool
	case "select":
		return selectValueOf(stored.ValueJson) == selectValueOf(want.ValueJson) && want.ValueJson != nil
	case "multiselect":
		return stringSetEquals(multiselectValues(stored.ValueJson), multiselectValues(want.ValueJson))
	default:
		return stored.ValueText != nil && want.ValueText != nil && *stored.ValueText == *want.ValueText
	}
}

// numericEquals compares two pgtype.Numeric values via their string forms —
// pgtype.Numeric has no native equality.
func numericEquals(a, b pgtype.Numeric) bool {
	if !a.Valid || !b.Valid {
		return a.Valid == b.Valid
	}

	av, _ := a.Value()
	bv, _ := b.Value()
	ar, aok := new(big.Rat).SetString(fmt.Sprintf("%v", av))
	br, bok := new(big.Rat).SetString(fmt.Sprintf("%v", bv))

	return aok && bok && ar.Cmp(br) == 0
}

// selectValueOf unwraps the {"value": "..."} select envelope.
func selectValueOf(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}

	if s, ok := m["value"].(string); ok {
		return s
	}

	return ""
}

// multiselectValues decodes the bare-array multiselect shape.
func multiselectValues(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}

	var vals []string
	if err := json.Unmarshal(raw, &vals); err != nil {
		return nil
	}

	return vals
}

func stringSetEquals(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	as := append([]string{}, a...)
	bs := append([]string{}, b...)
	sort.Strings(as)
	sort.Strings(bs)

	for i := range as {
		if as[i] != bs[i] {
			return false
		}
	}

	return true
}

// ---------------------------------------------------------------------------
// Create path
// ---------------------------------------------------------------------------

// universityCreatePath resolves a slug, adopts an existing unlinked
// university space when one matches, or creates a new space; writes the
// extracted CFVs; and links the space to the intel record — all in one
// transaction.
func (p *UniversityPromoter) universityCreatePath(ctx context.Context, record gen.IntelRecord, source gen.IntelSource, ext extractor.Extraction) (PromoteResult, error) {
	name := stringField(ext, "name")
	if name == "" {
		return PromoteResult{}, fmt.Errorf("promoter: university requires a name: %w", ErrRouteToReview)
	}

	slug := stringField(ext, "slug")
	if slug == "" {
		slug = content.Slugify(name)
	}
	if slug == "" {
		slug = slugify(name)
	}

	// geo_code stores ISO alpha-2; the extraction carries ISO-3.
	alpha2 := stores.CountryCodeAlpha2(firstCountry(ext, "country"))
	if alpha2 == "" {
		return PromoteResult{}, fmt.Errorf(
			"promoter: university requires a resolvable country but none extracted: %w", ErrRouteToReview)
	}

	var geoCode *string
	geoCode = &alpha2

	// Parent: the destination country geo space, when one exists.
	var parentID pgtype.UUID
	if id, err := p.store.GetCountrySpace(ctx, alpha2); err == nil && id.Valid {
		parentID = id
	}

	defs := p.resolveUniversityFieldDefs(ctx)

	// Adopt: a university space already carries the slug (admin-seeded or
	// created by a different source's record). Link it and run the update
	// diff instead of colliding on the slug UNIQUE constraint.
	if existing, err := p.store.GetUniversitySpaceBySlug(ctx, slug); err == nil {
		return p.universityAdoptPath(ctx, record, existing, defs, ext)
	} else if !isNotFound(err) {
		return PromoteResult{}, fmt.Errorf("promoter: lookup university space by slug: %w", err)
	}

	spaceType, err := p.store.GetSpaceTypeBySlug(ctx, universitySpaceType)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: resolve education space type: %w", err)
	}

	var result PromoteResult

	var createdSpaceID pgtype.UUID

	err = p.store.WithTx(ctx, func(tx UniversityTxStore) error {
		space, err := tx.CreateSpace(ctx, gen.CreateSpaceParams{
			ParentID:       parentID,
			SpaceTypeID:    spaceType.ID,
			Name:           name,
			Slug:           slug,
			Description:    stringPtrField(ext, "description"),
			GeoCode:        geoCode,
			Level:          universityLevel,
			Visibility:     "public",
			PostingPerm:    "open",
			ReplyingPerm:   "open",
			CommentingPerm: "open",
			JoiningPerm:    "open",
			Status:         "active",
		})
		if err != nil {
			return fmt.Errorf("promoter: create university space: %w", err)
		}

		if err := upsertExtractedCFVs(ctx, tx, defs, ext, space.ID); err != nil {
			return err
		}

		if err := backfillInitials(ctx, tx, defs, ext, space.ID, name); err != nil {
			return err
		}

		if _, err := tx.UpsertUniversityIntelLink(ctx, gen.UpsertUniversityIntelLinkParams{
			SpaceID:       space.ID,
			IntelRecordID: record.ID,
		}); err != nil {
			return fmt.Errorf("promoter: link university intel record: %w", err)
		}

		if err := tx.UpdateIntelRecordStatus(ctx, gen.UpdateIntelRecordStatusParams{
			ID:     record.ID,
			Status: "extracted",
		}); err != nil {
			return fmt.Errorf("promoter: mark intel record extracted: %w", err)
		}

		createdSpaceID = space.ID
		result = PromoteResult{DomainID: pgToUUID(space.ID), Action: ActionCreated}

		return nil
	})
	if err != nil {
		return result, err
	}

	// Fire-and-forget Wikidata enrichment for the new university (P1813
	// short name, founded year, student count, logo) — same hook the admin
	// create path runs (§39).
	if p.enqueuer != nil && createdSpaceID.Valid {
		payload, _ := json.Marshal(map[string]string{"space_id": pgToUUID(createdSpaceID).String()})
		if err := p.enqueuer.Enqueue(taskSpaceAutoPopulate, payload); err != nil {
			slog.Warn("promoter: enqueue space:auto_populate failed", "space_id", createdSpaceID.String(), "error", err)
		}
	}

	return result, nil
}

// adoptProtections returns the override set for first-link adoption: every
// CFV-mapped field that already holds a value, plus any populated space
// columns (name, description, geo_code→"country"). The extraction then
// only fills fields the humans never provided.
func (p *UniversityPromoter) adoptProtections(ctx context.Context, space gen.GetSpaceByIDRow, defs map[string]universityFieldDef) (map[string]struct{}, error) {
	protected := map[string]struct{}{}

	if space.Name != "" {
		protected["name"] = struct{}{}
	}
	if space.Description != nil && *space.Description != "" {
		protected["description"] = struct{}{}
	}
	if space.GeoCode != nil && *space.GeoCode != "" {
		protected["country"] = struct{}{}
	}

	stored, err := p.store.ListCustomFieldValuesByEntity(ctx, gen.ListCustomFieldValuesByEntityParams{
		EntityType: cfvEntityType,
		EntityID:   space.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("promoter: list adopt cfvs: %w", err)
	}

	keyByFieldID := make(map[string]string, len(defs))
	for key, def := range defs {
		keyByFieldID[def.id.String()] = key
	}
	for _, v := range stored {
		if key, ok := keyByFieldID[v.FieldID.String()]; ok {
			protected[key] = struct{}{}
		}
	}

	return protected, nil
}

// universityIdentityMatches reports whether an extraction plausibly
// describes the same university as the space: names must agree
// (case-insensitive exact or equal slugs, so "St. John's" ≈ "st-johns")
// and the country must not conflict (either side unknown counts as
// agreement — a missing signal is not a contradiction).
func universityIdentityMatches(spaceName string, spaceGeo *string, ext extractor.Extraction) bool {
	extName := stringField(ext, "name")
	if extName == "" {
		return false
	}

	sameName := strings.EqualFold(strings.TrimSpace(spaceName), strings.TrimSpace(extName)) ||
		content.Slugify(spaceName) == content.Slugify(extName)
	if !sameName {
		return false
	}

	alpha2 := stores.CountryCodeAlpha2(firstCountry(ext, "country"))
	if alpha2 == "" || spaceGeo == nil || *spaceGeo == "" {
		return true
	}

	return alpha2 == *spaceGeo
}

// universityAdoptPath links an existing unlinked university space to the
// record and applies the update diff against it. A space already linked to
// a DIFFERENT intel record routes to review — repointing the link (the
// space_id PK conflict) would silently unbind the other record and the two
// would ping-pong the link on every crawl.
func (p *UniversityPromoter) universityAdoptPath(ctx context.Context, record gen.IntelRecord, space gen.GetUniversitySpaceBySlugRow, defs map[string]universityFieldDef, ext extractor.Extraction) (PromoteResult, error) {
	// Identity check covers both adopt cases: a slug collision with a
	// differently-identified space (manual rename, slugify collision) and a
	// space already linked to another intel record. Same entity = repoint
	// is the designed behavior (link tracks the newest promoted record);
	// conflicting identity = a human must untangle it.
	if !universityIdentityMatches(space.Name, space.GeoCode, ext) {
		return PromoteResult{}, fmt.Errorf(
			"promoter: slug %q collides with a differently-identified university space (%q vs %q): %w",
			space.Slug, space.Name, stringField(ext, "name"), ErrRouteToReview)
	}

	// A space already linked to a DIFFERENT record routes to review —
	// repointing the link would silently unbind the other record and the
	// two would ping-pong the link on every crawl. Same-record re-promotion
	// (link.IntelRecordID == record.ID) is the designed repoint case.
	link, err := p.store.GetUniversityIntelLinkBySpace(ctx, space.ID)
	switch {
	case err != nil && !isNotFound(err):
		return PromoteResult{}, fmt.Errorf("promoter: check university intel link: %w", err)
	case err == nil && link.IntelRecordID != record.ID:
		return PromoteResult{}, fmt.Errorf(
			"promoter: slug %q space is already linked to a different intel record: %w",
			space.Slug, ErrRouteToReview)
	}

	spaceRow, err := p.store.GetSpaceByID(ctx, space.ID)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: load adopted university space: %w", err)
	}

	// Conservative adopt: every field the space already carries a value for
	// is treated as human-curated and seeded into the link's overrides set —
	// first-link adoption fills gaps only, never clobbers existing data.
	// Divergences surface as suppressed entries in the review log and stay
	// protected until an admin releases the key.
	protected, err := p.adoptProtections(ctx, spaceRow, defs)
	if err != nil {
		return PromoteResult{}, err
	}

	writes, changed, suppressed, err := p.planUniversityWrites(ctx, spaceRow, defs, ext, protected)
	if err != nil {
		return PromoteResult{}, err
	}

	changesJSON := marshalUniversityChanges(changed, writes, suppressed)

	err = p.store.WithTx(ctx, func(tx UniversityTxStore) error {
		if err := applyUniversityWrites(ctx, tx, writes); err != nil {
			return err
		}

		if _, err := tx.UpsertUniversityIntelLink(ctx, gen.UpsertUniversityIntelLinkParams{
			SpaceID:       space.ID,
			IntelRecordID: record.ID,
		}); err != nil {
			return fmt.Errorf("promoter: link university intel record: %w", err)
		}

		if len(protected) > 0 {
			overrides := make(map[string]bool, len(protected))
			for k := range protected {
				overrides[k] = true
			}
			raw, merr := json.Marshal(overrides)
			if merr == nil {
				if err := tx.AddUniversityIntelOverrides(ctx, gen.AddUniversityIntelOverridesParams{
					SpaceID:   space.ID,
					Overrides: raw,
				}); err != nil {
					return fmt.Errorf("promoter: seed adopt overrides: %w", err)
				}
			}
		}

		if err := tx.UpdateIntelRecordStatus(ctx, gen.UpdateIntelRecordStatusParams{
			ID:     record.ID,
			Status: "extracted",
		}); err != nil {
			return fmt.Errorf("promoter: mark intel record extracted: %w", err)
		}

		if len(changed)+len(suppressed) > 0 {
			note := noteAutoRecrawl
			if err := tx.CreateIntelReviewLog(ctx, gen.CreateIntelReviewLogParams{
				IntelRecordID: record.ID,
				Action:        "auto_update",
				FieldChanges:  changesJSON,
				Note:          &note,
			}); err != nil {
				return fmt.Errorf("promoter: write adopt review log: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return PromoteResult{}, err
	}

	action := ActionVerified
	if len(changed) > 0 {
		action = ActionUpdated
	}

	return PromoteResult{DomainID: pgToUUID(space.ID), Action: action, ChangedFields: changed}, nil
}

// ---------------------------------------------------------------------------
// Update path
// ---------------------------------------------------------------------------

// universityUpdatePath diffs the extraction against the linked space's
// columns + CFVs, skipping every key in the link's editorial_overrides, and
// writes the changes + auto_update review log atomically.
func (p *UniversityPromoter) universityUpdatePath(ctx context.Context, record gen.IntelRecord, existing gen.UniversityIntelLink, ext extractor.Extraction) (PromoteResult, error) {
	overrides, err := parseOverrideKeys(existing.EditorialOverrides)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: parse editorial overrides: %w", err)
	}

	space, err := p.store.GetSpaceByID(ctx, existing.SpaceID)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: load linked university space: %w", err)
	}

	defs := p.resolveUniversityFieldDefs(ctx)

	writes, changed, suppressed, err := p.planUniversityWrites(ctx, space, defs, ext, overrides)
	if err != nil {
		return PromoteResult{}, err
	}

	changesJSON := marshalUniversityChanges(changed, writes, suppressed)

	err = p.store.WithTx(ctx, func(tx UniversityTxStore) error {
		if err := applyUniversityWrites(ctx, tx, writes); err != nil {
			return err
		}

		if _, err := tx.UpsertUniversityIntelLink(ctx, gen.UpsertUniversityIntelLinkParams{
			SpaceID:       existing.SpaceID,
			IntelRecordID: record.ID,
		}); err != nil {
			return fmt.Errorf("promoter: relink university intel record: %w", err)
		}

		if len(changed)+len(suppressed) > 0 {
			note := noteAutoRecrawl
			if err := tx.CreateIntelReviewLog(ctx, gen.CreateIntelReviewLogParams{
				IntelRecordID: record.ID,
				Action:        "auto_update",
				FieldChanges:  changesJSON,
				Note:          &note,
			}); err != nil {
				return fmt.Errorf("promoter: write auto_update review log: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return PromoteResult{}, err
	}

	action := ActionVerified
	if len(changed) > 0 {
		action = ActionUpdated
	}

	return PromoteResult{DomainID: pgToUUID(existing.SpaceID), Action: action, ChangedFields: changed}, nil
}

// universityWrite is one pending write the diff produced.
type universityWrite struct {
	field  string
	from   any
	to     any
	params *gen.UpsertCustomFieldValueParams // CFV writes
	space  *gen.UpdateSpaceParams            // space-column write (single)
}

// planUniversityWrites compares extraction values against stored state and
// returns the writes to apply — the reads run outside the tx, the writes
// inside it via applyUniversityWrites (§6.2 atomicity).
func (p *UniversityPromoter) planUniversityWrites(ctx context.Context, space gen.GetSpaceByIDRow, defs map[string]universityFieldDef, ext extractor.Extraction, overrides map[string]struct{}) ([]universityWrite, []string, []fieldChange, error) {
	skip := func(key string) bool {
		if overrides == nil {
			return false
		}
		_, ok := overrides[key]
		return ok
	}

	var (
		writes     []universityWrite
		changed    []string
		suppressed []fieldChange
	)

	// ---- Space columns ----
	spaceParams := gen.UpdateSpaceParams{
		ID:             space.ID,
		Name:           space.Name,
		Description:    space.Description,
		WikiContent:    space.WikiContent,
		GeoCode:        space.GeoCode,
		Visibility:     space.Visibility,
		PostingPerm:    space.PostingPerm,
		ReplyingPerm:   space.ReplyingPerm,
		CommentingPerm: space.CommentingPerm,
		JoiningPerm:    space.JoiningPerm,
		Status:         space.Status,
	}
	spaceDirty := false

	if name := stringField(ext, "name"); name != "" && name != space.Name {
		if skip("name") {
			suppressed = append(suppressed, fieldChange{Field: "name", From: space.Name, To: name, Suppressed: true})
		} else {
			writes = append(writes, universityWrite{field: "name", from: space.Name, to: name})
			spaceParams.Name = name
			spaceDirty = true
			changed = append(changed, "name")
		}
	}

	if desc := stringPtrField(ext, "description"); desc != nil && (space.Description == nil || *desc != *space.Description) {
		var from any
		if space.Description != nil {
			from = *space.Description
		}
		if skip("description") {
			suppressed = append(suppressed, fieldChange{Field: "description", From: from, To: *desc, Suppressed: true})
		} else {
			writes = append(writes, universityWrite{field: "description", from: from, to: *desc})
			spaceParams.Description = desc
			spaceDirty = true
			changed = append(changed, "description")
		}
	}

	if alpha2 := stores.CountryCodeAlpha2(firstCountry(ext, "country")); alpha2 != "" {
		existing := ""
		if space.GeoCode != nil {
			existing = *space.GeoCode
		}
		if alpha2 != existing {
			if skip("country") {
				suppressed = append(suppressed, fieldChange{Field: "country", From: existing, To: alpha2, Suppressed: true})
			} else {
				writes = append(writes, universityWrite{field: "country", from: existing, to: alpha2})
				spaceParams.GeoCode = &alpha2
				spaceDirty = true
				changed = append(changed, "country")
			}
		}
	}

	if spaceDirty {
		sp := spaceParams
		writes = append(writes, universityWrite{field: "__space__", space: &sp})
	}

	// ---- Custom field values ----
	stored, err := p.store.ListCustomFieldValuesByEntity(ctx, gen.ListCustomFieldValuesByEntityParams{
		EntityType: cfvEntityType,
		EntityID:   space.ID,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("promoter: list university cfvs: %w", err)
	}

	storedByFieldID := make(map[string]gen.CustomFieldValue, len(stored))
	for _, v := range stored {
		storedByFieldID[v.FieldID.String()] = v
	}

	for key, def := range defs {
		want, ok := cfvWriteFor(def, ext, space.ID)
		if !ok {
			continue
		}

		if prev, exists := storedByFieldID[def.id.String()]; exists && cfvStoredEquals(prev, def, want) {
			continue
		}

		if skip(key) {
			suppressed = append(suppressed, fieldChange{Field: key, From: cfvStoredDisplay(def, storedByFieldID[def.id.String()]), To: cfvDisplayValue(def, want), Suppressed: true})
			continue
		}

		params := want
		writes = append(writes, universityWrite{field: key, to: cfvDisplayValue(def, params), params: &params})
		changed = append(changed, key)
	}

	return writes, changed, suppressed, nil
}

// applyUniversityWrites executes the planned writes inside the transaction.
func applyUniversityWrites(ctx context.Context, tx UniversityTxStore, writes []universityWrite) error {
	for _, w := range writes {
		switch {
		case w.space != nil:
			if _, err := tx.UpdateSpace(ctx, *w.space); err != nil {
				return fmt.Errorf("promoter: update university space: %w", err)
			}
		case w.params != nil:
			if _, err := tx.UpsertCustomFieldValue(ctx, *w.params); err != nil {
				return fmt.Errorf("promoter: upsert cfv %q: %w", w.field, err)
			}
		}
	}

	return nil
}

// cfvStoredDisplay renders a stored CFV's current value for the review log.
func cfvStoredDisplay(def universityFieldDef, stored gen.CustomFieldValue) any {
	switch def.fieldType {
	case "number":
		if stored.ValueNumber.Valid {
			v, _ := stored.ValueNumber.Value()
			return v
		}
	case "boolean":
		if stored.ValueBool != nil {
			return *stored.ValueBool
		}
	case "select":
		return selectValueOf(stored.ValueJson)
	case "multiselect":
		return multiselectValues(stored.ValueJson)
	default:
		if stored.ValueText != nil {
			return *stored.ValueText
		}
	}

	return nil
}

// cfvDisplayValue renders a CFV write's new value for the review log.
func cfvDisplayValue(def universityFieldDef, params gen.UpsertCustomFieldValueParams) any {
	switch {
	case params.ValueText != nil:
		return *params.ValueText
	case params.ValueBool != nil:
		return *params.ValueBool
	case params.ValueNumber.Valid:
		v, _ := params.ValueNumber.Value()
		return v
	case len(params.ValueJson) > 0:
		var out any
		_ = json.Unmarshal(params.ValueJson, &out)
		return out
	}

	return nil
}

// marshalUniversityChanges renders applied + suppressed fieldChange entries
// for the review log.
func marshalUniversityChanges(changed []string, writes []universityWrite, suppressed []fieldChange) []byte {
	byField := make(map[string]universityWrite, len(writes))
	for _, w := range writes {
		byField[w.field] = w
	}

	changes := make([]fieldChange, 0, len(changed)+len(suppressed))
	for _, c := range changed {
		w := byField[c]
		changes = append(changes, fieldChange{Field: c, From: w.from, To: w.to})
	}
	changes = append(changes, suppressed...)

	out, _ := json.Marshal(changes)

	return out
}

// upsertExtractedCFVs writes every extracted CFV-mapped field — used on the
// create path where nothing exists yet.
func upsertExtractedCFVs(ctx context.Context, tx UniversityTxStore, defs map[string]universityFieldDef, ext extractor.Extraction, spaceID pgtype.UUID) error {
	for _, key := range universityCFVKeys {
		def, ok := defs[key]
		if !ok {
			continue
		}

		params, ok := cfvWriteFor(def, ext, spaceID)
		if !ok {
			continue
		}

		if _, err := tx.UpsertCustomFieldValue(ctx, params); err != nil {
			return fmt.Errorf("promoter: upsert cfv %q: %w", key, err)
		}
	}

	return nil
}

// backfillInitials derives initials when the extraction produced none —
// same rule as the universities admin create path (Wikidata P1813 refines
// it later via space:auto_populate).
func backfillInitials(ctx context.Context, tx UniversityTxStore, defs map[string]universityFieldDef, ext extractor.Extraction, spaceID pgtype.UUID, name string) error {
	if stringField(ext, "initials") != "" {
		return nil
	}

	def, ok := defs["initials"]
	if !ok {
		return nil
	}

	initials := universities.DeriveInitials(name, nil, nil)
	if initials == "" {
		return nil
	}

	if _, err := tx.UpsertCustomFieldValue(ctx, gen.UpsertCustomFieldValueParams{
		FieldID:    def.id,
		EntityType: cfvEntityType,
		EntityID:   spaceID,
		ValueText:  &initials,
	}); err != nil {
		return fmt.Errorf("promoter: backfill initials: %w", err)
	}

	return nil
}
