package sources

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// slugify helpers mirror content.Slugify to avoid an import cycle.
var (
	invalidSlugChars       = regexp.MustCompile(`[^a-z0-9-]+`)
	multipleHyphens        = regexp.MustCompile(`-{2,}`)
	leadingTrailingHyphens = regexp.MustCompile(`^-+|-+$`)
)

func slugify(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = invalidSlugChars.ReplaceAllString(s, "-")
	s = multipleHyphens.ReplaceAllString(s, "-")
	s = leadingTrailingHyphens.ReplaceAllString(s, "")

	if s == "" {
		return "untitled"
	}

	return s
}

// Service applies the business logic for the intel_sources registry. It
// wraps a Store and is the entry point used by HTTP/admin handlers and the
// scheduler. Methods here are intentionally thin: validation that belongs
// at the domain layer lives here, while persistence is delegated to the
// Store.
type Service struct {
	store         *Store
	metadataQueue func(ctx context.Context, tenantID pgtype.UUID, sourceID SourceID) error
}

// NewService creates a sources service backed by the given store.
func NewService(store *Store) *Service {
	return &Service{store: store}
}

// SetMetadataQueue wires a background task enqueue function for source
// metadata refresh. It is called after a source becomes active so the public
// directory can show its logo and description without live link-preview fetches.
func (s *Service) SetMetadataQueue(q func(ctx context.Context, tenantID pgtype.UUID, sourceID SourceID) error) {
	s.metadataQueue = q
}

// queueMetadataFetch enqueues a metadata fetch for active sources. Failures are
// logged but not returned so DB writes do not fail when the queue is down.
func (s *Service) queueMetadataFetch(ctx context.Context, tenantID pgtype.UUID, src Source) {
	if s.metadataQueue == nil || !src.Enabled || src.Status != "active" || !src.ID.Valid {
		return
	}

	if err := s.metadataQueue(ctx, tenantID, src.ID); err != nil {
		slog.Warn("sources: metadata enqueue failed", "source_id", src.ID.String(), "error", err)
	}
}

// Create registers a new source in the registry. Defaults status to 'active'
// when the caller did not supply a status (e.g. admin creation flow).
func (s *Service) Create(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelSourceParams) (Source, error) {
	if arg.Status == nil || arg.Status == "" {
		arg.Status = "active"
	}

	src, err := s.store.Create(ctx, tenantID, arg)
	if err != nil {
		return Source{}, err
	}

	s.queueMetadataFetch(ctx, tenantID, src)
	return src, nil
}

// Get retrieves a source by ID.
func (s *Service) Get(ctx context.Context, tenantID pgtype.UUID, id SourceID) (Source, error) {
	return s.store.Get(ctx, tenantID, id)
}

// GetByURL retrieves a source by its canonical source_url.
func (s *Service) GetByURL(ctx context.Context, tenantID pgtype.UUID, sourceURL string) (Source, error) {
	return s.store.GetByURL(ctx, tenantID, sourceURL)
}

// List returns a page of sources.
func (s *Service) List(ctx context.Context, tenantID pgtype.UUID, arg gen.ListIntelSourcesParams) ([]Source, error) {
	return s.store.List(ctx, tenantID, arg)
}

// ListByCategory returns enabled sources for a given category.
func (s *Service) ListByCategory(ctx context.Context, tenantID pgtype.UUID, category Category) ([]Source, error) {
	return s.store.ListByCategory(ctx, tenantID, string(category))
}

// ListDue returns up to limit sources whose crawl interval has elapsed.
func (s *Service) ListDue(ctx context.Context, tenantID pgtype.UUID, limit int32) ([]Source, error) {
	return s.store.ListDue(ctx, tenantID, limit)
}

// Enable re-enables a previously disabled source so future crawls resume.
func (s *Service) Enable(ctx context.Context, tenantID pgtype.UUID, id SourceID) error {
	return s.store.UpdateEnabled(ctx, tenantID, id, true)
}

// Disable disables a source: future crawls stop, but existing promoted
// records are preserved.
func (s *Service) Disable(ctx context.Context, tenantID pgtype.UUID, id SourceID) error {
	return s.store.UpdateEnabled(ctx, tenantID, id, false)
}

// Delete removes a source from the registry.
func (s *Service) Delete(ctx context.Context, tenantID pgtype.UUID, id SourceID) error {
	return s.store.Delete(ctx, tenantID, id)
}

// Update edits the configurable fields of a source (name, domain, url,
// category, trust level, crawl settings, license, usage restriction, enabled
// toggle). Returns the updated source row.
func (s *Service) Update(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelSourceParams) (Source, error) {
	return s.store.Update(ctx, tenantID, arg)
}

// ListProposed returns a page of sources with status='proposed' for admin
// review. Tenant-scoped.
func (s *Service) ListProposed(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]Source, error) {
	return s.store.ListProposed(ctx, tenantID, limit, offset)
}

// ApproveProposed activates a proposed source (status='active', enabled=true)
// and records the approver. Tenant-scoped.
func (s *Service) ApproveProposed(ctx context.Context, tenantID, id, approvedBy pgtype.UUID) (Source, error) {
	src, err := s.store.ApproveProposed(ctx, tenantID, id, approvedBy)
	if err != nil {
		return Source{}, err
	}

	s.queueMetadataFetch(ctx, tenantID, src)
	return src, nil
}

// CrawlNow is a no-op service method retained for API surface compatibility.
// The actual crawl-now trigger is handled by the HTTP handler which enqueues
// the intel:fetch_source Asynq task directly (INTEL-011 fix). This method
// verifies the source exists and is enabled.
func (s *Service) CrawlNow(ctx context.Context, tenantID pgtype.UUID, id SourceID) error {
	src, err := s.store.Get(ctx, tenantID, id)
	if err != nil {
		return fmt.Errorf("sources: crawl-now: load source: %w", err)
	}

	if !src.Enabled {
		return errors.New("sources: crawl-now: source is disabled")
	}

	return nil
}

// ProposeInput is the public submission payload for a new source proposal.
type ProposeInput struct {
	Name        string
	Domain      string
	SourceURL   string
	Category    string
	SourceType  string
	Description *string
}

// Propose creates a new source proposal from an authenticated user.
// The source is inserted with status='proposed' and enabled=false; an admin
// must approve it before it appears in the public directory.
func (s *Service) Propose(ctx context.Context, tenantID, userID pgtype.UUID, input ProposeInput) (Source, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.SourceURL = strings.TrimSpace(input.SourceURL)
	input.Domain = strings.TrimSpace(input.Domain)
	input.Category = strings.TrimSpace(input.Category)
	input.SourceType = strings.TrimSpace(input.SourceType)

	if input.Name == "" {
		return Source{}, errpkg.ErrRequired
	}
	if input.SourceURL == "" {
		return Source{}, errpkg.ErrRequired
	}
	if input.SourceType == "" {
		input.SourceType = "rss"
	}

	u, err := url.Parse(input.SourceURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return Source{}, fmt.Errorf("sources: propose: %w: invalid source_url", errpkg.ErrInvalid)
	}

	if input.Domain == "" {
		input.Domain = u.Hostname()
	}
	if input.Domain == "" || strings.Contains(input.Domain, ":") {
		return Source{}, fmt.Errorf("sources: propose: %w: invalid domain", errpkg.ErrInvalid)
	}

	// Public URLs must be unique per tenant.
	if _, err := s.store.GetByURL(ctx, tenantID, input.SourceURL); err == nil {
		return Source{}, errpkg.ErrAlreadyExists
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Source{}, fmt.Errorf("sources: propose: check existing url: %w", err)
	}

	slug := s.uniqueSlug(ctx, tenantID, slugify(input.Name))

	now := time.Now().UTC()
	proposedAt := pgtype.Timestamptz{Time: now, Valid: true}

	crawlMethod := "manual"
	crawlFrequency := "manual"
	switch input.SourceType {
	case "rss":
		crawlMethod = "rss"
		crawlFrequency = "hourly"
	case "api", "youtube":
		crawlMethod = "api"
		crawlFrequency = "daily"
	}

	arg := gen.CreateIntelSourceParams{

		Name:                input.Name,
		Domain:              input.Domain,
		SourceUrl:           input.SourceURL,
		Category:            input.Category,
		SourceType:          input.SourceType,
		SourceKind:          "publisher",
		TrustLevel:          5,
		CrawlMethod:         crawlMethod,
		CrawlFrequency:      crawlFrequency,
		CrawlFrequencyMins:  0,
		RobotsPolicy:        []byte("{}"),
		AttributionRequired: false,
		ProviderConfig:      []byte("{}"),
		Enabled:             false,
		Slug:                &slug,
		Description:         input.Description,
		Status:              "proposed",
		ProposedBy:          userID,
		ProposedAt:          proposedAt,
	}

	return s.store.Create(ctx, tenantID, arg)
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (s *Service) CreateAutoForVideo(ctx context.Context, tenantID, userID pgtype.UUID, title, sourceURL, thumbnailURL *string) (Source, error) {
	if title == nil || *title == "" {
		return Source{}, fmt.Errorf("%w: name", errpkg.ErrRequired)
	}
	if sourceURL == nil || *sourceURL == "" {
		return Source{}, fmt.Errorf("%w: source_url", errpkg.ErrRequired)
	}

	existing, err := s.store.GetByURL(ctx, tenantID, *sourceURL)
	if err == nil {
		s.queueMetadataFetch(ctx, tenantID, existing)
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Source{}, err
	}

	domain := ""
	u, err := url.Parse(*sourceURL)
	if err == nil && u.Host != "" {
		domain = u.Host
	}

	slug := s.uniqueSlug(ctx, tenantID, *title)

	arg := gen.CreateIntelSourceParams{

		Name:               *title,
		Slug:               &slug,
		Domain:             domain,
		SourceUrl:          *sourceURL,
		Category:           "video",
		SourceType:         "video_upload",
		SourceKind:         "publisher",
		TrustLevel:         2,
		CrawlMethod:        "manual",
		CrawlFrequency:     "manual",
		CrawlFrequencyMins: 0,
		Enabled:            true,
		Status:             "active",
		LogoUrl:            thumbnailURL,
	}

	return s.Create(ctx, tenantID, arg)
}

func (s *Service) CreateSourceFromEntity(ctx context.Context, tenantID, entityID, userID pgtype.UUID, entityType, name, slug, sourceURL, category string) (Source, error) {
	if name == "" {
		return Source{}, fmt.Errorf("%w: name", errpkg.ErrRequired)
	}
	if slug == "" {
		return Source{}, fmt.Errorf("%w: slug", errpkg.ErrRequired)
	}

	if sourceURL != "" {
		existing, err := s.store.GetByURL(ctx, tenantID, sourceURL)
		if err == nil {
			s.queueMetadataFetch(ctx, tenantID, existing)
			return existing, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Source{}, err
		}
	}

	domain := ""
	u, err := url.Parse(sourceURL)
	if err == nil && u.Host != "" {
		domain = u.Host
	}

	sourceKind := "institution"
	if entityType == "user" || entityType == "profile" {
		sourceKind = "individual"
	}

	arg := gen.CreateIntelSourceParams{

		Name:               name,
		Slug:               &slug,
		Domain:             domain,
		SourceUrl:          sourceURL,
		Category:           category,
		SourceType:         "static",
		SourceKind:         sourceKind,
		TrustLevel:         3,
		CrawlMethod:        "manual",
		CrawlFrequency:     "manual",
		CrawlFrequencyMins: 0,
		Enabled:            true,
		Status:             "active",
	}

	return s.Create(ctx, tenantID, arg)
}

func (s *Service) CreateAutoForComparison(ctx context.Context, tenantID, userID pgtype.UUID, sourceName, sourceURL *string, trustLevel int32) (Source, error) {
	if sourceName == nil || *sourceName == "" {
		return Source{}, fmt.Errorf("%w: source_name", errpkg.ErrRequired)
	}

	existing, err := s.store.GetByURL(ctx, tenantID, derefString(sourceURL))
	if err == nil {
		s.queueMetadataFetch(ctx, tenantID, existing)
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Source{}, err
	}

	domain := ""
	if sourceURL != nil && *sourceURL != "" {
		u, err := url.Parse(*sourceURL)
		if err == nil && u.Host != "" {
			domain = u.Host
		}
	}

	slug := s.uniqueSlug(ctx, tenantID, *sourceName)

	arg := gen.CreateIntelSourceParams{

		Name:               *sourceName,
		Slug:               &slug,
		Domain:             domain,
		SourceUrl:          derefString(sourceURL),
		Category:           "comparison",
		SourceType:         "comparison_api",
		SourceKind:         "publisher",
		TrustLevel:         int16(max(1, min(5, trustLevel))),
		CrawlMethod:        "api",
		CrawlFrequency:     "daily",
		CrawlFrequencyMins: 0,
		Enabled:            true,
		Status:             "active",
	}

	return s.Create(ctx, tenantID, arg)
}

func (s *Service) uniqueSlug(ctx context.Context, tenantID pgtype.UUID, base string) string {
	base = slugify(base)
	if base == "" || base == "untitled" {
		base = "source"
	}

	candidate := base
	for i := 0; i < 100; i++ {
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", base, i)
		}
		_, err := s.store.GetBySlug(ctx, tenantID, candidate)
		if errors.Is(err, pgx.ErrNoRows) {
			return candidate
		}
		if err != nil {
			// On unexpected error, fall back to a UUID-suffixed slug.
			return fmt.Sprintf("%s-%d", base, time.Now().Unix())
		}
	}
	return fmt.Sprintf("%s-%d", base, time.Now().Unix())
}

// ToggleSourceBlock blocks the source if it is not blocked, otherwise unblocks it.
// Returns the new block state and any error.
func (s *Service) ToggleSourceBlock(ctx context.Context, tenantID, blockerID, sourceID pgtype.UUID) (bool, error) {
	blocked, err := s.store.IsSourceBlocked(ctx, tenantID, blockerID, sourceID)
	if err != nil {
		return false, fmt.Errorf("sources: toggle block: check: %w", err)
	}

	if blocked {
		if err := s.store.UnblockSource(ctx, tenantID, blockerID, sourceID); err != nil {
			return false, fmt.Errorf("sources: toggle block: unblock: %w", err)
		}
		return false, nil
	}

	_, err = s.store.BlockSource(ctx, tenantID, blockerID, sourceID)
	if err != nil {
		return false, fmt.Errorf("sources: toggle block: block: %w", err)
	}
	return true, nil
}

// IsSourceBlocked returns whether the user has blocked the given source.
func (s *Service) IsSourceBlocked(ctx context.Context, tenantID, blockerID, sourceID pgtype.UUID) (bool, error) {
	return s.store.IsSourceBlocked(ctx, tenantID, blockerID, sourceID)
}

// ListBlockedSourceIDs returns the source IDs blocked by the given user.
func (s *Service) ListBlockedSourceIDs(ctx context.Context, tenantID, blockerID pgtype.UUID) ([]pgtype.UUID, error) {
	return s.store.ListBlockedSourceIDs(ctx, tenantID, blockerID)
}
