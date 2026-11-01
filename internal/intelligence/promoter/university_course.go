// The university_course domain projects an intel_record onto a
// university_course_offerings row — resolving (or creating) the canonical
// university_course_catalog row and the offering's university space. The
// (course_id, university_id) unique index is the dedupe key; field-level
// editorial_overrides on the offering row enforce the human-edit-wins
// invariant on re-crawl.
//
// Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md
package promoter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/extractor"
	"github.com/jedafe/japa/internal/intelligence/stores"
)

// iso4217 is a strict 3-letter currency check — anything else fails the
// write so a malformed extraction can't poison the column.
var iso4217 = regexp.MustCompile(`^[A-Z]{3}$`)

// UniversityCourseStore is the subset of *stores.Store the course promoter
// needs.
type UniversityCourseStore interface {
	PersonaResolver
	GetIntelRecord(ctx context.Context, id pgtype.UUID) (gen.IntelRecord, error)
	GetIntelSource(ctx context.Context, id pgtype.UUID) (gen.IntelSource, error)
	GetUniversitySpaceBySlug(ctx context.Context, slug string) (gen.GetUniversitySpaceBySlugRow, error)
	GetUniversitySpaceByName(ctx context.Context, name string) ([]gen.GetUniversitySpaceByNameRow, error)
	GetCourseCatalogBySlug(ctx context.Context, slug string) (gen.GetCourseCatalogBySlugRow, error)
	GetCourseOfferingByCourseUniversity(ctx context.Context, arg gen.GetCourseOfferingByCourseUniversityParams) (gen.GetCourseOfferingByCourseUniversityRow, error)
	UpdateIntelRecordStatus(ctx context.Context, arg gen.UpdateIntelRecordStatusParams) error
	WithTx(ctx context.Context, fn func(UniversityCourseTxStore) error) error
}

// UniversityCourseTxStore is the write-side subset used inside the promote
// transaction (§6.2).
type UniversityCourseTxStore interface {
	CreateCourseCatalogIntel(ctx context.Context, arg gen.CreateCourseCatalogIntelParams) (pgtype.UUID, error)
	CreateCourseOfferingIntel(ctx context.Context, arg gen.CreateCourseOfferingIntelParams) (pgtype.UUID, error)
	UpdateCourseOfferingIntel(ctx context.Context, arg gen.UpdateCourseOfferingIntelParams) error
	UpdateIntelRecordStatus(ctx context.Context, arg gen.UpdateIntelRecordStatusParams) error
	CreateIntelReviewLog(ctx context.Context, arg gen.CreateIntelReviewLogParams) error
}

// UniversityCoursePromoter promotes university_course intel_records into
// university_course_offerings rows.
type UniversityCoursePromoter struct {
	store   UniversityCourseStore
	aiModel *string
	now     func() time.Time
}

// UniversityCoursePromoterOption configures a UniversityCoursePromoter.
type UniversityCoursePromoterOption func(*UniversityCoursePromoter)

// WithUniversityCourseClock overrides the clock (tests).
func WithUniversityCourseClock(fn func() time.Time) UniversityCoursePromoterOption {
	return func(p *UniversityCoursePromoter) { p.now = fn }
}

// WithUniversityCourseAIModel sets the AI model name (ADR-088).
func WithUniversityCourseAIModel(model string) UniversityCoursePromoterOption {
	return func(p *UniversityCoursePromoter) { p.aiModel = &model }
}

// NewUniversityCoursePromoter creates a promoter backed by the given store.
func NewUniversityCoursePromoter(store UniversityCourseStore, authorID pgtype.UUID, opts ...UniversityCoursePromoterOption) *UniversityCoursePromoter {
	_ = authorID
	p := &UniversityCoursePromoter{store: store, aiModel: ptr("gpt-4o-mini"), now: time.Now}
	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Entity returns the entity_type this promoter handles.
func (p *UniversityCoursePromoter) Entity() string { return "university_course" }

// ErrNotUniversityCourse is returned when Promote is called on a record
// whose entity_type is not "university_course".
var ErrNotUniversityCourse = errors.New("promoter: intel record is not a university_course")

// Promote resolves the university + catalog course, then creates or
// refreshes the offering row. Unlike the table-projection domains there is
// no per-record lookup up front — existence is probed on the resolved
// (course_id, university_id) pair inside the path — so this Promote writes
// the promoteDomain skeleton's flow directly.
func (p *UniversityCoursePromoter) Promote(ctx context.Context, recordID uuid.UUID) (PromoteResult, error) {
	pgID := uuidToPg(recordID)

	record, err := p.store.GetIntelRecord(ctx, pgID)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: load intel record: %w", err)
	}

	if record.EntityType != p.Entity() {
		return PromoteResult{}, ErrNotUniversityCourse
	}

	ext, err := parseExtraction(record.Extracted)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: parse extracted: %w", err)
	}

	uniID, uniSlug, err := p.resolveUniversity(ctx, ext)
	if err != nil {
		return PromoteResult{}, err
	}

	courseID, courseSlug, err := p.resolveCatalog(ctx, ext)
	if err != nil {
		return PromoteResult{}, err
	}

	existing, err := p.store.GetCourseOfferingByCourseUniversity(ctx, gen.GetCourseOfferingByCourseUniversityParams{
		CourseID:     courseID,
		UniversityID: uniID,
	})
	switch {
	case err == nil:
		return p.courseUpdatePath(ctx, record, existing, ext)
	case isNotFound(err):
		return p.courseCreatePath(ctx, record, courseID, courseSlug, uniID, uniSlug, ext)
	default:
		return PromoteResult{}, fmt.Errorf("promoter: load existing offering: %w", err)
	}
}

// resolveUniversity maps the extracted university_slug/university_name (+
// optional country) to a university space id. Unresolvable → ErrRouteToReview.
func (p *UniversityCoursePromoter) resolveUniversity(ctx context.Context, ext extractor.Extraction) (pgtype.UUID, string, error) {
	if slug := stringField(ext, "university_slug"); slug != "" {
		if row, err := p.store.GetUniversitySpaceBySlug(ctx, slug); err == nil {
			return row.ID, row.Slug, nil
		}
	}

	name := stringField(ext, "university_name")
	if name == "" {
		return pgtype.UUID{}, "", fmt.Errorf(
			"promoter: course offering requires university_name or university_slug: %w", ErrRouteToReview)
	}

	cands, err := p.store.GetUniversitySpaceByName(ctx, name)
	if err != nil {
		return pgtype.UUID{}, "", fmt.Errorf("promoter: university name lookup: %w", err)
	}

	alpha2 := stores.CountryCodeAlpha2(firstCountry(ext, "country"))

	var match *gen.GetUniversitySpaceByNameRow
	for i := range cands {
		c := cands[i]
		if alpha2 == "" || (c.GeoCode != nil && *c.GeoCode == alpha2) {
			if match != nil {
				return pgtype.UUID{}, "", fmt.Errorf(
					"promoter: ambiguous university_name %q matches multiple spaces: %w", name, ErrRouteToReview)
			}
			match = &c
		}
	}
	if match == nil {
		return pgtype.UUID{}, "", fmt.Errorf(
			"promoter: no university space resolves for %q (country %q): %w", name, alpha2, ErrRouteToReview)
	}

	return match.ID, match.Slug, nil
}

// resolveCatalog returns the canonical catalog row for the extracted
// course_title, creating a minimal row when the pipeline has never seen
// the course.
func (p *UniversityCoursePromoter) resolveCatalog(ctx context.Context, ext extractor.Extraction) (pgtype.UUID, string, error) {
	title := stringField(ext, "course_title")
	if title == "" {
		return pgtype.UUID{}, "", fmt.Errorf(
			"promoter: course offering requires course_title: %w", ErrRouteToReview)
	}

	slug := content.Slugify(title)
	if slug == "" {
		slug = slugify(title)
	}

	row, err := p.store.GetCourseCatalogBySlug(ctx, slug)
	if err == nil {
		return row.ID, row.Slug, nil
	}
	if !isNotFound(err) {
		return pgtype.UUID{}, "", fmt.Errorf("promoter: catalog lookup: %w", err)
	}

	var createdID pgtype.UUID
	err = p.store.WithTx(ctx, func(tx UniversityCourseTxStore) error {
		lang := stringField(ext, "language")
		if lang == "" {
			lang = "English"
		}

		id, err := tx.CreateCourseCatalogIntel(ctx, gen.CreateCourseCatalogIntelParams{
			Title:                title,
			Slug:                 slug,
			DegreeType:           stringPtrField(ext, "degree_type"),
			FieldOfStudy:         stringPtrField(ext, "field_of_study"),
			Language:             lang,
			DefaultDurationYears: numericField(ext, "duration_years"),
		})
		if err != nil {
			return fmt.Errorf("promoter: create catalog course: %w", err)
		}

		createdID = id

		return nil
	})
	if err != nil {
		return pgtype.UUID{}, "", err
	}

	return createdID, slug, nil
}

// courseCreatePath inserts the offering row (slug = "{catalog}-{university}")
// and flips the record to 'extracted'.
func (p *UniversityCoursePromoter) courseCreatePath(ctx context.Context, record gen.IntelRecord, courseID pgtype.UUID, courseSlug string, uniID pgtype.UUID, uniSlug string, ext extractor.Extraction) (PromoteResult, error) {
	currency := normalizedCurrency(ext)

	var result PromoteResult

	err := p.store.WithTx(ctx, func(tx UniversityCourseTxStore) error {
		id, err := tx.CreateCourseOfferingIntel(ctx, gen.CreateCourseOfferingIntelParams{
			CourseID:          courseID,
			UniversityID:      uniID,
			Slug:              courseSlug + "-" + uniSlug,
			Tuition:           numericField(ext, "tuition"),
			Currency:          currency,
			DurationYears:     numericField(ext, "duration_years"),
			StudyMode:         stringPtrField(ext, "study_mode"),
			IntakeMonths:      normalizedIntakes(ext),
			EntryRequirements: stringPtrField(ext, "entry_requirements"),
			Accreditation:     stringPtrField(ext, "accreditation"),
			IsPublished:       true,
			IntelRecordID:     record.ID,
		})
		if err != nil {
			return fmt.Errorf("promoter: create course offering: %w", err)
		}

		if err := tx.UpdateIntelRecordStatus(ctx, gen.UpdateIntelRecordStatusParams{
			ID:     record.ID,
			Status: "extracted",
		}); err != nil {
			return fmt.Errorf("promoter: mark intel record extracted: %w", err)
		}

		result = PromoteResult{DomainID: pgToUUID(id), Action: ActionCreated}

		return nil
	})

	return result, err
}

// courseUpdatePath diffs the extraction against the offering row, skipping
// overridden fields, and writes changes + auto_update review log in one tx.
func (p *UniversityCoursePromoter) courseUpdatePath(ctx context.Context, record gen.IntelRecord, existing gen.GetCourseOfferingByCourseUniversityRow, ext extractor.Extraction) (PromoteResult, error) {
	overrides, err := parseOverrideKeys(existing.EditorialOverrides)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: parse editorial overrides: %w", err)
	}

	params, changeEntries, suppressed := buildCourseOfferingUpdateParams(existing, ext, overrides)
	params.IntelRecordID = record.ID

	changes := append(changeEntries, suppressed...)

	var changesJSON []byte
	if len(changes) > 0 {
		changesJSON, _ = json.Marshal(changes)
	}

	var result PromoteResult

	err = p.store.WithTx(ctx, func(tx UniversityCourseTxStore) error {
		if err := tx.UpdateCourseOfferingIntel(ctx, params); err != nil {
			return fmt.Errorf("promoter: update course offering: %w", err)
		}

		if len(changes) > 0 {
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

		var names []string
		for _, ch := range changeEntries {
			names = append(names, ch.Field)
		}
		result = PromoteResult{
			DomainID:      pgToUUID(existing.ID),
			Action:        ActionVerified,
			ChangedFields: names,
		}
		if len(names) > 0 {
			result.Action = ActionUpdated
		}

		return nil
	})

	return result, err
}

// buildCourseOfferingUpdateParams seeds the update from the existing row —
// preserving is_published (a curated draft stays a draft) — then applies
// each extracted field not in overrides that actually changed.
func buildCourseOfferingUpdateParams(existing gen.GetCourseOfferingByCourseUniversityRow, ext extractor.Extraction, overrides map[string]struct{}) (gen.UpdateCourseOfferingIntelParams, []fieldChange, []fieldChange) {
	// changed carries applied field names; applied carries full From/To
	// entries for the review log (suppressed already carries them).
	params := gen.UpdateCourseOfferingIntelParams{
		ID:                existing.ID,
		Tuition:           existing.Tuition,
		Currency:          existing.Currency,
		DurationYears:     existing.DurationYears,
		StudyMode:         existing.StudyMode,
		IntakeMonths:      existing.IntakeMonths,
		EntryRequirements: existing.EntryRequirements,
		Accreditation:     existing.Accreditation,
		IsPublished:       existing.IsPublished,
		IntelRecordID:     existing.IntelRecordID,
	}

	var (
		changed    []string
		applied    []fieldChange
		suppressed []fieldChange
	)
	skip := func(key string) bool {
		_, ok := overrides[key]
		return ok
	}

	if n := numericField(ext, "tuition"); n.Valid && !numericEquals(existing.Tuition, n) {
		if skip("tuition") {
			v, _ := n.Value()
			ev, _ := existing.Tuition.Value()
			suppressed = append(suppressed, fieldChange{Field: "tuition", From: ev, To: v, Suppressed: true})
		} else {
			params.Tuition = n
			v, _ := n.Value()
			ev, _ := existing.Tuition.Value()
			applied = append(applied, fieldChange{Field: "tuition", From: ev, To: v})
			changed = append(changed, "tuition")
		}
	}
	if cur := normalizedCurrency(ext); cur != nil && (existing.Currency == nil || *cur != *existing.Currency) {
		if skip("currency") {
			var from any
			if existing.Currency != nil {
				from = *existing.Currency
			}
			suppressed = append(suppressed, fieldChange{Field: "currency", From: from, To: *cur, Suppressed: true})
		} else {
			params.Currency = cur
			var from any
			if existing.Currency != nil {
				from = *existing.Currency
			}
			applied = append(applied, fieldChange{Field: "currency", From: from, To: *cur})
			changed = append(changed, "currency")
		}
	}
	if n := numericField(ext, "duration_years"); n.Valid && !numericEquals(existing.DurationYears, n) {
		if skip("duration_years") {
			v, _ := n.Value()
			ev, _ := existing.DurationYears.Value()
			suppressed = append(suppressed, fieldChange{Field: "duration_years", From: ev, To: v, Suppressed: true})
		} else {
			params.DurationYears = n
			v, _ := n.Value()
			ev, _ := existing.DurationYears.Value()
			applied = append(applied, fieldChange{Field: "duration_years", From: ev, To: v})
			changed = append(changed, "duration_years")
		}
	}
	if s := stringPtrField(ext, "study_mode"); s != nil && (existing.StudyMode == nil || *s != *existing.StudyMode) {
		if skip("study_mode") {
			var from any
			if existing.StudyMode != nil {
				from = *existing.StudyMode
			}
			suppressed = append(suppressed, fieldChange{Field: "study_mode", From: from, To: *s, Suppressed: true})
		} else {
			params.StudyMode = s
			var from any
			if existing.StudyMode != nil {
				from = *existing.StudyMode
			}
			applied = append(applied, fieldChange{Field: "study_mode", From: from, To: *s})
			changed = append(changed, "study_mode")
		}
	}
	if v := normalizedIntakes(ext); v != nil && !stringSetEquals(v, existing.IntakeMonths) {
		if skip("intake_months") {
			suppressed = append(suppressed, fieldChange{Field: "intake_months", From: existing.IntakeMonths, To: v, Suppressed: true})
		} else {
			params.IntakeMonths = v
			applied = append(applied, fieldChange{Field: "intake_months", From: existing.IntakeMonths, To: v})
			changed = append(changed, "intake_months")
		}
	}
	if s := stringPtrField(ext, "entry_requirements"); s != nil && (existing.EntryRequirements == nil || *s != *existing.EntryRequirements) {
		if skip("entry_requirements") {
			var from any
			if existing.EntryRequirements != nil {
				from = *existing.EntryRequirements
			}
			suppressed = append(suppressed, fieldChange{Field: "entry_requirements", From: from, To: *s, Suppressed: true})
		} else {
			params.EntryRequirements = s
			var from any
			if existing.EntryRequirements != nil {
				from = *existing.EntryRequirements
			}
			applied = append(applied, fieldChange{Field: "entry_requirements", From: from, To: *s})
			changed = append(changed, "entry_requirements")
		}
	}
	if s := stringPtrField(ext, "accreditation"); s != nil && (existing.Accreditation == nil || *s != *existing.Accreditation) {
		if skip("accreditation") {
			var from any
			if existing.Accreditation != nil {
				from = *existing.Accreditation
			}
			suppressed = append(suppressed, fieldChange{Field: "accreditation", From: from, To: *s, Suppressed: true})
		} else {
			params.Accreditation = s
			var from any
			if existing.Accreditation != nil {
				from = *existing.Accreditation
			}
			applied = append(applied, fieldChange{Field: "accreditation", From: from, To: *s})
			changed = append(changed, "accreditation")
		}
	}

	return params, applied, suppressed
}

// normalizedCurrency returns the extraction's currency uppercased when it
// passes the ISO-4217 shape check, else nil.
func normalizedCurrency(ext extractor.Extraction) *string {
	s := strings.ToUpper(strings.TrimSpace(stringField(ext, "currency")))
	if !iso4217.MatchString(s) {
		return nil
	}

	return &s
}

// normalizedIntakes returns lowercased intake month names, or nil when the
// extraction produced none.
func normalizedIntakes(ext extractor.Extraction) []string {
	vals := stringSliceField(ext, "intake_months")
	if len(vals) == 0 {
		return nil
	}

	out := make([]string, 0, len(vals))
	for _, v := range vals {
		out = append(out, strings.ToLower(v))
	}

	return out
}
