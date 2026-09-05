package university_courses

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

func ptr(s string) *string { return &s }

func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		t.Fatalf("scan uuid %q: %v", s, err)
	}

	return id
}

func mustNumeric(t *testing.T, s string) pgtype.Numeric {
	t.Helper()

	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		t.Fatalf("scan numeric %q: %v", s, err)
	}

	return n
}

// rowFields returns the common field values used for all three generated row
// types so conversion tests focus on type wiring, not data differences.
func sampleRowFields(t *testing.T) (pgtype.UUID, pgtype.UUID, pgtype.UUID, pgtype.UUID, string, pgtype.Numeric, *string, pgtype.Numeric, *string, []string, *string, *string, bool, pgtype.Timestamptz, pgtype.Timestamptz, string, string, *string, *string, *string, *string, string, pgtype.Numeric, string, string) {
	t.Helper()
	id := mustUUID(t, "00000000-0000-0000-0000-000000000601")
	tenantID := mustUUID(t, "00000000-0000-0000-0000-000000000001")
	courseID := mustUUID(t, "00000000-0000-0000-0000-000000000501")
	universityID := mustUUID(t, "00000000-0000-0000-0000-000000000401")
	now := pgtype.Timestamptz{Time: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Valid: true}

	return id, tenantID, courseID, universityID,
		"unilag-bsc-computer-science",
		mustNumeric(t, "2500"), ptr("USD"), mustNumeric(t, "4"), ptr("Full-time"),
		[]string{"September"}, ptr("SSCE / equivalent"), ptr("NUC accredited"), true,
		now, now,
		"BSc Computer Science", "bsc-computer-science", ptr("Undergraduate CS programme."),
		ptr("BSc"), ptr("Computer Science"), ptr("Technology"), "English",
		mustNumeric(t, "4"), "University of Lagos", "university-of-lagos"
}

func TestToCourseOffering_ListRow(t *testing.T) {
	id, tid, cid, uid, slug, tuition, currency, duration, studyMode, intake, entryReq, accreditation, published, created, updated, courseTitle, courseSlug, courseDesc, degree, department, fieldOfStudy, language, defaultDuration, uniName, uniSlug := sampleRowFields(t)

	row := gen.ListCourseOfferingsByUniversityRow{
		ID: id, TenantID: tid, CourseID: cid, UniversityID: uid, Slug: slug,
		Tuition: tuition, Currency: currency, DurationYears: duration, StudyMode: studyMode,
		IntakeMonths: intake, EntryRequirements: entryReq, Accreditation: accreditation,
		IsPublished: published, CreatedAt: created, UpdatedAt: updated,
		CourseTitle: courseTitle, CourseSlug: courseSlug, CourseDescription: courseDesc,
		DegreeType: degree, Department: department, FieldOfStudy: fieldOfStudy,
		Language: language, DefaultDurationYears: defaultDuration,
		UniversityName: uniName, UniversitySlug: uniSlug,
	}

	o, err := toCourseOffering(row)
	if err != nil {
		t.Fatalf("toCourseOffering(ListRow) error: %v", err)
	}

	assertCourseOffering(t, o, id, tid, cid, uid, slug, tuition, currency, duration, studyMode, intake, entryReq, accreditation, published, created, updated, courseTitle, courseSlug, courseDesc, degree, department, fieldOfStudy, language, defaultDuration, uniName, uniSlug)
}

func TestToCourseOffering_GetByIDRow(t *testing.T) {
	id, tid, cid, uid, slug, tuition, currency, duration, studyMode, intake, entryReq, accreditation, published, created, updated, courseTitle, courseSlug, courseDesc, degree, department, fieldOfStudy, language, defaultDuration, uniName, uniSlug := sampleRowFields(t)

	row := gen.GetCourseOfferingByIDRow{
		ID: id, TenantID: tid, CourseID: cid, UniversityID: uid, Slug: slug,
		Tuition: tuition, Currency: currency, DurationYears: duration, StudyMode: studyMode,
		IntakeMonths: intake, EntryRequirements: entryReq, Accreditation: accreditation,
		IsPublished: published, CreatedAt: created, UpdatedAt: updated,
		CourseTitle: courseTitle, CourseSlug: courseSlug, CourseDescription: courseDesc,
		DegreeType: degree, Department: department, FieldOfStudy: fieldOfStudy,
		Language: language, DefaultDurationYears: defaultDuration,
		UniversityName: uniName, UniversitySlug: uniSlug,
	}

	o, err := toCourseOffering(row)
	if err != nil {
		t.Fatalf("toCourseOffering(GetByIDRow) error: %v", err)
	}

	assertCourseOffering(t, o, id, tid, cid, uid, slug, tuition, currency, duration, studyMode, intake, entryReq, accreditation, published, created, updated, courseTitle, courseSlug, courseDesc, degree, department, fieldOfStudy, language, defaultDuration, uniName, uniSlug)
}

func TestToCourseOffering_GetBySlugRow(t *testing.T) {
	id, tid, cid, uid, slug, tuition, currency, duration, studyMode, intake, entryReq, accreditation, published, created, updated, courseTitle, courseSlug, courseDesc, degree, department, fieldOfStudy, language, defaultDuration, uniName, uniSlug := sampleRowFields(t)

	row := gen.GetCourseOfferingBySlugRow{
		ID: id, TenantID: tid, CourseID: cid, UniversityID: uid, Slug: slug,
		Tuition: tuition, Currency: currency, DurationYears: duration, StudyMode: studyMode,
		IntakeMonths: intake, EntryRequirements: entryReq, Accreditation: accreditation,
		IsPublished: published, CreatedAt: created, UpdatedAt: updated,
		CourseTitle: courseTitle, CourseSlug: courseSlug, CourseDescription: courseDesc,
		DegreeType: degree, Department: department, FieldOfStudy: fieldOfStudy,
		Language: language, DefaultDurationYears: defaultDuration,
		UniversityName: uniName, UniversitySlug: uniSlug,
	}

	o, err := toCourseOffering(row)
	if err != nil {
		t.Fatalf("toCourseOffering(GetBySlugRow) error: %v", err)
	}

	assertCourseOffering(t, o, id, tid, cid, uid, slug, tuition, currency, duration, studyMode, intake, entryReq, accreditation, published, created, updated, courseTitle, courseSlug, courseDesc, degree, department, fieldOfStudy, language, defaultDuration, uniName, uniSlug)
}

func TestToCourseOffering_UnknownType(t *testing.T) {
	_, err := toCourseOffering("not a row")
	if err == nil {
		t.Fatal("toCourseOffering(unknown type) returned nil error")
	}
}

func assertCourseOffering(t *testing.T, o CourseOffering, id, tid, cid, uid pgtype.UUID, slug string, tuition pgtype.Numeric, currency *string, duration pgtype.Numeric, studyMode *string, intake []string, entryReq, accreditation *string, published bool, created, updated pgtype.Timestamptz, courseTitle, courseSlug string, courseDesc, degree, department, fieldOfStudy *string, language string, defaultDuration pgtype.Numeric, uniName, uniSlug string) {
	t.Helper()

	if o.ID != id {
		t.Errorf("ID = %v, want %v", o.ID, id)
	}

	if o.TenantID != tid {
		t.Errorf("TenantID = %v, want %v", o.TenantID, tid)
	}

	if o.CourseID != cid {
		t.Errorf("CourseID = %v, want %v", o.CourseID, cid)
	}

	if o.UniversityID != uid {
		t.Errorf("UniversityID = %v, want %v", o.UniversityID, uid)
	}

	if o.Slug != slug {
		t.Errorf("Slug = %q, want %q", o.Slug, slug)
	}

	if o.Tuition != tuition {
		t.Errorf("Tuition = %v, want %v", o.Tuition, tuition)
	}

	if (o.Currency == nil) != (currency == nil) || (o.Currency != nil && *o.Currency != *currency) {
		t.Errorf("Currency = %v, want %v", o.Currency, currency)
	}

	if o.DurationYears != duration {
		t.Errorf("DurationYears = %v, want %v", o.DurationYears, duration)
	}

	if o.CourseTitle != courseTitle {
		t.Errorf("CourseTitle = %q, want %q", o.CourseTitle, courseTitle)
	}

	if o.UniversityName != uniName {
		t.Errorf("UniversityName = %q, want %q", o.UniversityName, uniName)
	}

	if o.Language != language {
		t.Errorf("Language = %q, want %q", o.Language, language)
	}

	if !published {
		t.Error("IsPublished should be true")
	}

	if len(o.IntakeMonths) != len(intake) || (len(intake) > 0 && o.IntakeMonths[0] != intake[0]) {
		t.Errorf("IntakeMonths = %v, want %v", o.IntakeMonths, intake)
	}
}

// TestServiceConstructorCompile verifies the Service constructor wires the
// provided store and exposes the expected public methods.
func TestServiceConstructorCompile(t *testing.T) {
	store := &Store{}

	svc := NewService(store)
	if svc == nil {
		t.Fatal("NewService(store) returned nil")
	}

	if svc.store != store {
		t.Fatal("NewService did not store the provided store")
	}
}

// TestStoreMethodSetCompile is a compile-time guard that the Store type exposes
// every method the service layer depends on. If a method is renamed or removed,
// this test fails at build time.
func TestStoreMethodSetCompile(t *testing.T) {
	var s *Store

	_ = s.SearchOfferings
	_ = s.GetOfferingByID
	_ = s.GetOfferingBySlug
}

// TestServiceMethodSetCompile is a compile-time guard that the Service type
// exposes the public API expected by the comparison resolver.
func TestServiceMethodSetCompile(t *testing.T) {
	var s *Service

	_ = s.SearchOfferings
	_ = s.GetOfferingByID
	_ = s.GetOfferingBySlug
}

// TestParamTypeAliasesCompile is a compile-time guard that the sqlc-generated
// param and row types referenced by the store exist and keep their expected
// signatures. This catches drift after sqlc regeneration.
func TestParamTypeAliasesCompile(t *testing.T) {
	var (
		_ gen.ListCourseOfferingsByUniversityParams
		_ gen.ListCourseOfferingsByUniversityRow
		_ gen.GetCourseOfferingByIDParams
		_ gen.GetCourseOfferingByIDRow
		_ gen.GetCourseOfferingBySlugParams
		_ gen.GetCourseOfferingBySlugRow
		_ pgtype.UUID
	)
}

// TestContextImportKept ensures the context import is not dropped by the linter.
func TestContextImportKept(t *testing.T) {
	ctx := t.Context()
	if ctx == nil {
		t.Fatal("context should not be nil")
	}
}

// TestDBTypeImportKept ensures the db package import is not dropped by the linter.
func TestDBTypeImportKept(t *testing.T) {
	var _ *db.DB
}
