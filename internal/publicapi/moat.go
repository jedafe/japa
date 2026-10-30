package publicapi

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/credentialchecker"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/languagetesthub"
	"github.com/jedafe/japa/internal/scamalert"
	"github.com/jedafe/japa/internal/scholarships"
	"github.com/jedafe/japa/internal/visaroutes"
	"github.com/jedafe/japa/internal/visatimeline"
)

// ErrReaderUnconfigured is returned when a moat-domain reader was not wired.
var ErrReaderUnconfigured = errors.New("publicapi: reader not configured")

// ErrDestinationNotFound is returned when a slug does not resolve to an
// expected public space (used by GetUniversity and the destination brief).
var ErrDestinationNotFound = errors.New("destination_not_found")

// ---------------------------------------------------------------------------
// Reader interfaces — narrow slices of the domain services (agent data API).
// Ref: docs/superpowers/specs/2026-09-23-agent-data-api-design.md
// ---------------------------------------------------------------------------

// VisaRouteReader is satisfied by *visaroutes.Service.
type VisaRouteReader interface {
	List(ctx context.Context, destSpaceID pgtype.UUID, profession, education string, experienceYears, page, limit int32) ([]visaroutes.RouteDTO, int64, error)
	Compare(ctx context.Context, ids []pgtype.UUID) ([]visaroutes.RouteDTO, error)
	GetByID(ctx context.Context, id pgtype.UUID) (visaroutes.RouteDTO, error)
	Enabled(ctx context.Context) bool
}

// VisaTimelineReader is satisfied by *visatimeline.Service.
type VisaTimelineReader interface {
	GetStats(ctx context.Context, originSpaceID, destSpaceID pgtype.UUID, visaCategory, consulate string, months int) ([]visatimeline.ProcessingStatDTO, error)
	GetTrend(ctx context.Context, destSpaceID pgtype.UUID, visaCategory, consulate string, months int) ([]visatimeline.TrendPointDTO, error)
	Enabled(ctx context.Context) bool
}

// CredentialReader is satisfied by *credentialchecker.Service.
// NOTE: credentialchecker.Service has no Enabled method — the web route
// gates via features.Gate("credential_checker.enabled") middleware only,
// and the public route does the same. Do not add Enabled here.
type CredentialReader interface {
	ListProfessions(ctx context.Context) ([]credentialchecker.ProfessionDTO, error)
	ListPathways(ctx context.Context, destSpaceID, issuingSpaceID, professionID pgtype.UUID, page, limit int32) ([]credentialchecker.PathwayDTO, int64, error)
	GetPathway(ctx context.Context, id pgtype.UUID) (credentialchecker.PathwayDetailDTO, error)
}

// LanguageTestReader is satisfied by *languagetesthub.Service.
type LanguageTestReader interface {
	ListTests(ctx context.Context) ([]languagetesthub.TestDTO, error)
	GetTestBySlug(ctx context.Context, slug string) (languagetesthub.TestDTO, error)
	ListRequirements(ctx context.Context, destSpaceID pgtype.UUID, visaCategory string) ([]languagetesthub.RequirementDTO, error)
	Enabled(ctx context.Context) bool
}

// ScholarshipReader is satisfied by *scholarships.Service.
type ScholarshipReader interface {
	FindScholarships(ctx context.Context, filters scholarships.ScholarshipFilters) ([]scholarships.ScholarshipResult, int64, error)
	GetScholarshipDetail(ctx context.Context, guideID pgtype.UUID) (scholarships.ScholarshipResult, error)
	Statuses() []scholarships.Option
	ListLevels(ctx context.Context) ([]scholarships.Option, error)
	Enabled(ctx context.Context) bool
}

// SpaceReader is satisfied by *spaces.Service once Task 3 adds the
// GetPublicSpaceBySlug delegate to store.go. Used for universities
// (university-type spaces) and destination resolution for the brief.
// GetPublicSpaceBySlug (db/queries/spaces.sql) enforces
// visibility='public' AND status='active' AND deleted_at IS NULL in SQL —
// the §47-intended public guard lives in the query, not app code.
type SpaceReader interface {
	GetSpaceTypeBySlug(ctx context.Context, slug string) (gen.SpaceType, error)
	ListSpacesByTypePaginated(ctx context.Context, spaceTypeID pgtype.UUID, search, geoCode, region, subregion, within string, includeInactive bool, sort string, limit, offset int32) ([]gen.ListSpacesByTypePaginatedRow, error)
	CountSpacesByType(ctx context.Context, spaceTypeID pgtype.UUID, search, geoCode, region, subregion, within string, includeInactive bool) (int64, error)
	GetPublicSpaceBySlug(ctx context.Context, slug string) (gen.GetPublicSpaceBySlugRow, error)
	// GetPublicSpaceByGeoCode enforces the same public/active/deleted guard
	// in SQL — the extension digest resolves destination_country through it.
	GetPublicSpaceByGeoCode(ctx context.Context, geoCode string) (gen.GetPublicSpaceByGeoCodeRow, error)
}

// ScamAlertReader is satisfied by *scamalert.Service (brief only in v1).
type ScamAlertReader interface {
	List(ctx context.Context, destSpaceID pgtype.UUID, scamType string, page, limit int32) ([]scamalert.ScamReportDTO, int64, error)
	Enabled(ctx context.Context) bool
}

// MoatReaders bundles every domain reader the agent data API needs.
// Injected via Service.SetMoatReaders during server wiring.
type MoatReaders struct {
	VisaRoutes    VisaRouteReader
	VisaTimeline  VisaTimelineReader
	Credentials   CredentialReader
	LanguageTests LanguageTestReader
	Scholarships  ScholarshipReader
	Spaces        SpaceReader
	ScamAlerts    ScamAlertReader
}

const universitySpaceTypeSlug = "university"

// ---------------------------------------------------------------------------
// Service wiring + delegation
// ---------------------------------------------------------------------------

// SetMoatReaders injects the domain readers used by the agent data API.
func (s *Service) SetMoatReaders(r *MoatReaders) { s.moat = r }

// SetCache injects the Valkey client used for brief response caching.
func (s *Service) SetCache(c valkeycompat.Cmdable) { s.cache = c }

// SetSettingsReader injects the settings reader used for the brief cache TTL.
func (s *Service) SetSettingsReader(sr rateLimitSettingsReader) { s.settingsReader = sr }

func (s *Service) ListVisaRoutes(ctx context.Context, destSpaceID pgtype.UUID, profession, education string, experienceYears, page, limit int32) ([]visaroutes.RouteDTO, int64, error) {
	if s.moat == nil || s.moat.VisaRoutes == nil {
		return nil, 0, ErrReaderUnconfigured
	}

	return s.moat.VisaRoutes.List(ctx, destSpaceID, profession, education, experienceYears, page, limit)
}

func (s *Service) GetVisaRoute(ctx context.Context, id pgtype.UUID) (visaroutes.RouteDTO, error) {
	if s.moat == nil || s.moat.VisaRoutes == nil {
		return visaroutes.RouteDTO{}, ErrReaderUnconfigured
	}

	return s.moat.VisaRoutes.GetByID(ctx, id)
}

func (s *Service) CompareVisaRoutes(ctx context.Context, ids []pgtype.UUID) ([]visaroutes.RouteDTO, error) {
	if s.moat == nil || s.moat.VisaRoutes == nil {
		return nil, ErrReaderUnconfigured
	}

	return s.moat.VisaRoutes.Compare(ctx, ids)
}

func (s *Service) GetVisaTimelineStats(ctx context.Context, originSpaceID, destSpaceID pgtype.UUID, visaCategory, consulate string, months int) ([]visatimeline.ProcessingStatDTO, error) {
	if s.moat == nil || s.moat.VisaTimeline == nil {
		return nil, ErrReaderUnconfigured
	}

	return s.moat.VisaTimeline.GetStats(ctx, originSpaceID, destSpaceID, visaCategory, consulate, months)
}

func (s *Service) GetVisaTimelineTrend(ctx context.Context, destSpaceID pgtype.UUID, visaCategory, consulate string, months int) ([]visatimeline.TrendPointDTO, error) {
	if s.moat == nil || s.moat.VisaTimeline == nil {
		return nil, ErrReaderUnconfigured
	}

	return s.moat.VisaTimeline.GetTrend(ctx, destSpaceID, visaCategory, consulate, months)
}

func (s *Service) ListCredentialProfessions(ctx context.Context) ([]credentialchecker.ProfessionDTO, error) {
	if s.moat == nil || s.moat.Credentials == nil {
		return nil, ErrReaderUnconfigured
	}

	return s.moat.Credentials.ListProfessions(ctx)
}

func (s *Service) ListCredentialPathways(ctx context.Context, destSpaceID, issuingSpaceID, professionID pgtype.UUID, page, limit int32) ([]credentialchecker.PathwayDTO, int64, error) {
	if s.moat == nil || s.moat.Credentials == nil {
		return nil, 0, ErrReaderUnconfigured
	}

	return s.moat.Credentials.ListPathways(ctx, destSpaceID, issuingSpaceID, professionID, page, limit)
}

func (s *Service) GetCredentialPathway(ctx context.Context, id pgtype.UUID) (credentialchecker.PathwayDetailDTO, error) {
	if s.moat == nil || s.moat.Credentials == nil {
		return credentialchecker.PathwayDetailDTO{}, ErrReaderUnconfigured
	}

	return s.moat.Credentials.GetPathway(ctx, id)
}

func (s *Service) ListLanguageTests(ctx context.Context) ([]languagetesthub.TestDTO, error) {
	if s.moat == nil || s.moat.LanguageTests == nil {
		return nil, ErrReaderUnconfigured
	}

	return s.moat.LanguageTests.ListTests(ctx)
}

func (s *Service) GetLanguageTest(ctx context.Context, slug string) (languagetesthub.TestDTO, error) {
	if s.moat == nil || s.moat.LanguageTests == nil {
		return languagetesthub.TestDTO{}, ErrReaderUnconfigured
	}

	return s.moat.LanguageTests.GetTestBySlug(ctx, slug)
}

func (s *Service) ListLanguageTestRequirements(ctx context.Context, destSpaceID pgtype.UUID, visaCategory string) ([]languagetesthub.RequirementDTO, error) {
	if s.moat == nil || s.moat.LanguageTests == nil {
		return nil, ErrReaderUnconfigured
	}

	return s.moat.LanguageTests.ListRequirements(ctx, destSpaceID, visaCategory)
}

func (s *Service) FindScholarships(ctx context.Context, filters scholarships.ScholarshipFilters) ([]scholarships.ScholarshipResult, int64, error) {
	if s.moat == nil || s.moat.Scholarships == nil {
		return nil, 0, ErrReaderUnconfigured
	}

	return s.moat.Scholarships.FindScholarships(ctx, filters)
}

func (s *Service) GetScholarship(ctx context.Context, guideID pgtype.UUID) (scholarships.ScholarshipResult, error) {
	if s.moat == nil || s.moat.Scholarships == nil {
		return scholarships.ScholarshipResult{}, ErrReaderUnconfigured
	}

	return s.moat.Scholarships.GetScholarshipDetail(ctx, guideID)
}

func (s *Service) ScholarshipStatuses() ([]scholarships.Option, error) {
	if s.moat == nil || s.moat.Scholarships == nil {
		return nil, ErrReaderUnconfigured
	}

	return s.moat.Scholarships.Statuses(), nil
}

func (s *Service) ScholarshipLevels(ctx context.Context) ([]scholarships.Option, error) {
	if s.moat == nil || s.moat.Scholarships == nil {
		return nil, ErrReaderUnconfigured
	}

	return s.moat.Scholarships.ListLevels(ctx)
}

func (s *Service) ListUniversities(ctx context.Context, search, country, sort string, limit, offset int32) ([]gen.ListSpacesByTypePaginatedRow, int64, error) {
	if s.moat == nil || s.moat.Spaces == nil {
		return nil, 0, ErrReaderUnconfigured
	}

	st, err := s.moat.Spaces.GetSpaceTypeBySlug(ctx, universitySpaceTypeSlug)
	if err != nil {
		return nil, 0, err
	}

	items, err := s.moat.Spaces.ListSpacesByTypePaginated(ctx, st.ID, search, country, "", "", "", false, sort, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	total, err := s.moat.Spaces.CountSpacesByType(ctx, st.ID, search, country, "", "", "", false)
	if err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

func (s *Service) GetUniversity(ctx context.Context, slug string) (gen.GetPublicSpaceBySlugRow, error) {
	if s.moat == nil || s.moat.Spaces == nil {
		return gen.GetPublicSpaceBySlugRow{}, ErrReaderUnconfigured
	}

	st, err := s.moat.Spaces.GetSpaceTypeBySlug(ctx, universitySpaceTypeSlug)
	if err != nil {
		return gen.GetPublicSpaceBySlugRow{}, err
	}
	// GetPublicSpaceBySlug enforces visibility='public' AND status='active'
	// AND deleted_at IS NULL in SQL; only the space-type check remains here.
	space, err := s.moat.Spaces.GetPublicSpaceBySlug(ctx, slug)
	if err != nil {
		return gen.GetPublicSpaceBySlugRow{}, err
	}

	if space.SpaceTypeID != st.ID {
		return gen.GetPublicSpaceBySlugRow{}, ErrDestinationNotFound
	}

	return space, nil
}
