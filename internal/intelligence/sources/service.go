package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"slices"
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
	metadataQueue func(ctx context.Context, sourceID SourceID) error
	probeQueue    func(ctx context.Context, sourceID SourceID) error
}

// NewService creates a sources service backed by the given store.
func NewService(store *Store) *Service {
	return &Service{store: store}
}

// SetMetadataQueue wires a background task enqueue function for source
// metadata refresh. It is called after a source becomes active so the public
// directory can show its logo and description without live link-preview fetches.
func (s *Service) SetMetadataQueue(q func(ctx context.Context, sourceID SourceID) error) {
	s.metadataQueue = q
}

// queueMetadataFetch enqueues a metadata fetch for active sources. Failures are
// logged but not returned so DB writes do not fail when the queue is down.
func (s *Service) queueMetadataFetch(ctx context.Context, src Source) {
	if s.metadataQueue == nil || !src.Enabled || src.Status != "active" || !src.ID.Valid {
		return
	}

	if err := s.metadataQueue(ctx, src.ID); err != nil {
		slog.Warn("sources: metadata enqueue failed", "source_id", src.ID.String(), "error", err)
	}
}

// SetProbeQueue wires a background task enqueue function for the deep
// verify probe (intel:probe_verify). It is called after a proposed source
// is approved so the probe can materialize intel_source_channels rows.
// Same injected-func pattern as SetMetadataQueue — the sources package
// must not import the worker.
func (s *Service) SetProbeQueue(q func(ctx context.Context, sourceID SourceID) error) {
	s.probeQueue = q
}

// queueProbeVerify enqueues the deep verify probe for an active source.
// Failures are logged but not returned so DB writes do not fail when the
// queue is down. video_channel rows are §106 channel subscriptions —
// video:sync_channels owns ingestion and there is nothing to probe. The
// same exemption the worker's verifySkipSource applies to registry-only
// manual rows (crawl_method='manual' with source_type != 'auto') is applied
// here so the task is never enqueued; approved proposals are manual-by-
// design but keep source_type='auto', so they still verify.
func (s *Service) queueProbeVerify(ctx context.Context, src Source) {
	if s.probeQueue == nil || !src.Enabled || src.Status != "active" || !src.ID.Valid {
		return
	}

	if src.SourceType == string(SourceTypeVideoChannel) {
		return
	}

	if src.CrawlMethod == string(CrawlMethodManual) && src.SourceType != "auto" {
		return
	}

	if err := s.probeQueue(ctx, src.ID); err != nil {
		slog.Warn("sources: probe verify enqueue failed", "source_id", src.ID.String(), "error", err)
	}
}

// isVideoProviderHost reports whether host is a supported video platform —
// the same provider set as the video resolver (§106): YouTube, TikTok,
// Instagram, Vimeo. A 'video'-typed source on one of these hosts is a
// channel subscription (video_channel), not a crawl target.
func isVideoProviderHost(host string) bool {
	host = strings.TrimPrefix(strings.ToLower(host), "www.")
	host = strings.TrimPrefix(host, "m.")

	switch host {
	case "youtube.com", "youtu.be", "tiktok.com", "instagram.com", "vimeo.com":
		return true
	}

	return false
}

// videoChannelHost returns the provider host when the source is a video
// platform URL — derived from the explicit domain first, falling back to
// parsing source_url.
func videoChannelHost(domain, sourceURL string) string {
	if domain != "" {
		return domain
	}

	u, err := url.Parse(sourceURL)
	if err != nil {
		return ""
	}

	return u.Hostname()
}

// Create registers a new source in the registry. Defaults status to 'active'
// when the caller did not supply a status (e.g. admin creation flow).
func (s *Service) Create(ctx context.Context, arg gen.CreateIntelSourceParams) (Source, error) {
	// §106 channel-sync amendment: a 'video' source type on a supported video
	// platform URL registers a channel subscription (video_channel/manual) —
	// the video:sync_channels poller owns ingestion, not the intel crawler.
	if arg.SourceType == string(SourceTypeVideo) && isVideoProviderHost(videoChannelHost(arg.Domain, arg.SourceUrl)) {
		arg.SourceType = string(SourceTypeVideoChannel)
		arg.CrawlMethod = string(CrawlMethodManual)
		arg.CrawlFrequency = string(CrawlFrequencyManual)
		arg.CrawlFrequencyMins = 0
	}
	if arg.Status == nil || arg.Status == "" {
		arg.Status = "active"
	}

	// Multi-category (intel-channels §6): a non-nil categories array is
	// normalized like Propose and dual-writes the legacy primary category
	// when the caller left it empty. A nil array keeps the INSERT's
	// COALESCE fallback (ARRAY[category]) so back-compat callers that only
	// set category are untouched.
	if arg.Categories != nil {
		arg.Categories = NormalizeCategories(arg.Categories)
		if arg.Category == "" && len(arg.Categories) > 0 {
			arg.Category = arg.Categories[0]
		}
	}

	// robots_policy and provider_config are NOT NULL in the schema; a nil
	// []byte binds SQL NULL and overrides the column default, violating the
	// constraint (SQLSTATE 23502). Default both to an empty object at this
	// chokepoint so auto-create paths (video, comparison, entity) cannot
	// regress on it again.
	if len(arg.RobotsPolicy) == 0 {
		arg.RobotsPolicy = []byte("{}")
	}
	if len(arg.ProviderConfig) == 0 {
		arg.ProviderConfig = []byte("{}")
	}

	// A slug-less row is unreachable in the public directory (the detail
	// route is slug-keyed) — derive one from the name/domain when the caller
	// did not supply it so every create path produces a linkable source.
	if arg.Slug == nil || *arg.Slug == "" {
		base := arg.Name
		if strings.TrimSpace(base) == "" {
			base = arg.Domain
		}
		slug := s.uniqueSlug(ctx, slugify(base))
		arg.Slug = &slug
	}

	src, err := s.store.Create(ctx, arg)
	if err != nil {
		return Source{}, err
	}

	// intel-channels Phase 1 (final review I1): the scheduler selects
	// channels, not sources — an admin-created (or auto-created) crawlable
	// source needs a Default channel mirroring the 000681 backfill or it is
	// silently never crawled.
	s.ensureDefaultChannel(ctx, src)
	s.queueMetadataFetch(ctx, src)
	// The deep verify probe discovers additional channels on top of the
	// Default (dedupe handles the overlap). It no-ops for disabled,
	// non-active, and video_channel sources internally.
	s.queueProbeVerify(ctx, src)
	return src, nil
}

// sourceCrawlable reports whether a source can own a schedulable pull
// channel: a non-manual, non-inbound crawl_method, a source_url to point at,
// and not a §106 video_channel subscription (video:sync_channels owns those).
// The enabled flag is deliberately not part of crawlability — a disabled
// source still gets its Default channel, disabled, mirroring the 000681
// backfill's enabled=source.enabled semantics.
func sourceCrawlable(src Source) bool {
	if src.SourceType == string(SourceTypeVideoChannel) || src.SourceUrl == "" {
		return false
	}

	m := CrawlMethod(src.CrawlMethod)

	return m.IsValid() && m != CrawlMethodManual && !InboundMethod(m)
}

// ensureDefaultChannel materializes the source's "Default" channel —
// the same row the 000681 backfill created for pre-existing sources —
// when the source is crawlable. It mirrors the backfill shape:
// entry_url=source_url, crawl_method=source's, enabled=source.enabled,
// origin='admin', plus the intel-channels enrichments: entity_types and
// relevance_topics via ResolveCategoryMap on the source's categories,
// pipeline from the resolved map (default 'extract'),
// relevance_mode='filtered'.
//
// The row goes through Store.CreateChannel rather than Service.CreateChannel
// because it intentionally allows empty entity_types — like the backfill,
// whose rows fall back to source-level entity-type resolution at extract
// time. A source whose categories resolve no entity types must still get
// its channel or it can never be crawled. The entry_url SSRF literal check
// runs explicitly so this path shares the channel-create IP guard.
//
// Fire-and-forget like queueMetadataFetch: a UNIQUE
// (source_id, entry_url, entity_types) collision (re-create, probe already
// materialized the same row) or an insert failure is logged, never returned
// — source creation must not fail because the channel already exists.
func (s *Service) ensureDefaultChannel(ctx context.Context, src Source) {
	if !sourceCrawlable(src) {
		return
	}

	method := CrawlMethod(src.CrawlMethod)
	// Strict literal check ("" = no same-host carve-out): a source_url that
	// IS a private literal gets no Default channel — the dial guard would
	// refuse every crawl anyway, so the row would only churn failures.
	if err := validateChannelEntryURL(src.SourceUrl, method, ""); err != nil {
		slog.Warn("sources: default channel skipped — entry_url rejected",
			"source_id", src.ID.String(), "entry_url", src.SourceUrl, "error", err)
		return
	}

	resolved := ResolveCategoryMap(nil, sourceCategories(src))

	// Per-channel resolution (exit scenario): the Default channel takes the
	// entity types and pipeline of the categories that nominate the
	// source's crawl method — an rss source on [news, job] curates (news)
	// rather than inheriting the merged source-level extract pipeline.
	entityTypes, pipeline := resolveChannelMapForMethod(method, resolved)
	if !pipeline.IsValid() {
		pipeline = PipelineExtract
	}

	freq := src.CrawlFrequencyMins
	if freq <= 0 {
		freq = 1440
	}

	_, err := s.store.CreateChannel(ctx, gen.CreateIntelSourceChannelParams{
		SourceID:             src.ID,
		Label:                "Default",
		EntryUrl:             src.SourceUrl,
		CrawlMethod:          src.CrawlMethod,
		EntityTypes:          entityTypes,
		Pipeline:             string(pipeline),
		ProviderConfig:       []byte("{}"),
		CrawlFrequencyMins:   freq,
		Enabled:              src.Enabled,
		Origin:               string(ChannelOriginAdmin),
		RelevanceMode:        string(RelevanceFiltered),
		RelevanceTopics:      resolved.Topics,
		RelevanceUrlPatterns: []string{},
	})
	if err != nil {
		if errors.Is(err, errpkg.ErrAlreadyExists) {
			return // the channel already exists — dedupe, nothing to do
		}

		slog.Warn("sources: default channel create failed",
			"source_id", src.ID.String(), "entry_url", src.SourceUrl, "error", err)
	}
}

// Get retrieves a source by ID.
func (s *Service) Get(ctx context.Context, id SourceID) (Source, error) {
	return s.store.Get(ctx, id)
}

// GetByURL retrieves a source by its canonical source_url.
func (s *Service) GetByURL(ctx context.Context, sourceURL string) (Source, error) {
	return s.store.GetByURL(ctx, sourceURL)
}

// List returns a page of sources.
func (s *Service) List(ctx context.Context, arg gen.ListIntelSourcesParams) ([]Source, error) {
	return s.store.List(ctx, arg)
}

// ListByCategory returns enabled sources for a given category.
func (s *Service) ListByCategory(ctx context.Context, category Category) ([]Source, error) {
	return s.store.ListByCategory(ctx, string(category))
}

// ListDue returns up to limit sources whose crawl interval has elapsed.
func (s *Service) ListDue(ctx context.Context, limit int32) ([]Source, error) {
	return s.store.ListDue(ctx, limit)
}

// Enable re-enables a previously disabled source so future crawls resume.
func (s *Service) Enable(ctx context.Context, id SourceID) error {
	return s.store.UpdateEnabled(ctx, id, true)
}

// Disable disables a source: future crawls stop, but existing promoted
// records are preserved.
func (s *Service) Disable(ctx context.Context, id SourceID) error {
	return s.store.UpdateEnabled(ctx, id, false)
}

// Delete removes a source from the registry.
func (s *Service) Delete(ctx context.Context, id SourceID) error {
	return s.store.Delete(ctx, id)
}

// Update edits the configurable fields of a source (name, domain, url,
// category, trust level, crawl settings, license, usage restriction, enabled
// toggle). Returns the updated source row.
func (s *Service) Update(ctx context.Context, arg gen.UpdateIntelSourceParams) (Source, error) {
	// Same video→video_channel mapping as Create — editing a source to
	// source_type='video' on a supported platform must produce a channel
	// subscription, not an orphan 'video' row the poller ignores.
	if arg.SourceType == string(SourceTypeVideo) && isVideoProviderHost(videoChannelHost(arg.Domain, arg.SourceUrl)) {
		arg.SourceType = string(SourceTypeVideoChannel)
		arg.CrawlMethod = string(CrawlMethodManual)
		arg.CrawlFrequency = string(CrawlFrequencyManual)
		arg.CrawlFrequencyMins = 0
	}

	return s.store.Update(ctx, arg)
}

// ListVideoChannelsDue returns enabled video_channel sources due for a
// channel-feed sync (§106 channel-sync amendment).
func (s *Service) ListVideoChannelsDue(ctx context.Context, intervalMins, limit int32) ([]Source, error) {
	return s.store.ListVideoChannelsDue(ctx, intervalMins, limit)
}

// UpdateCrawlStatus records the outcome of a crawl or sync attempt on the
// source row (last_crawled_at, success/error stamps, consecutive_failures).
func (s *Service) UpdateCrawlStatus(ctx context.Context, arg gen.UpdateIntelSourceCrawlStatusParams) error {
	return s.store.UpdateCrawlStatus(ctx, arg)
}

// ClearPersona removes the persona attribution — the update query COALESCEs
// persona_id, so clearing to NULL goes through this dedicated path.
func (s *Service) ClearPersona(ctx context.Context, id SourceID) error {
	return s.store.ClearPersona(ctx, id)
}

// UpdateProviderConfig replaces the source's provider_config document. The
// caller merges keys it does not own — see Store.UpdateProviderConfig.
func (s *Service) UpdateProviderConfig(ctx context.Context, arg gen.UpdateIntelSourceProviderConfigParams) error {
	return s.store.UpdateProviderConfig(ctx, arg)
}

// UpdateCategories replaces a source's categories array by direct
// assignment — an empty array is an explicit clear to '{}'. Values are
// normalized like Propose (trim, lowercase, dedupe, order preserved); the
// admin PATCH calls this only when the categories key was present.
func (s *Service) UpdateCategories(ctx context.Context, arg gen.UpdateIntelSourceCategoriesParams) (Source, error) {
	arg.Categories = NormalizeCategories(arg.Categories)
	return s.store.UpdateCategories(ctx, arg)
}

// ListProposed returns a page of sources with status='proposed' for admin
// review — with the proposer's display name resolved. Platform-scoped.
func (s *Service) ListProposed(ctx context.Context, limit, offset int32, search string) ([]ProposedSource, error) {
	return s.store.ListProposed(ctx, limit, offset, search)
}

// CountProposed returns the total number of proposed sources matching the
// same search filter as ListProposed so the review surface can
// server-paginate.
func (s *Service) CountProposed(ctx context.Context, search string) (int64, error) {
	return s.store.CountProposed(ctx, search)
}

// ListFiltered runs the filtered registry list used by the control-panel
// sources management page (search/category/status/enabled + whitelisted
// sort + pagination). Platform-scoped.
func (s *Service) ListFiltered(ctx context.Context, arg gen.ListIntelSourcesFilteredParams) ([]Source, error) {
	return s.store.ListFiltered(ctx, arg)
}

// CountFiltered returns the matching-row total for ListFiltered.
func (s *Service) CountFiltered(ctx context.Context, arg gen.CountIntelSourcesFilteredParams) (int64, error) {
	return s.store.CountFiltered(ctx, arg)
}

// RejectProposed marks a proposed source as rejected and records the
// reviewer as the decision actor. Unlike ApproveProposed it enqueues no
// follow-up work — a rejected source never verifies or crawls.
func (s *Service) RejectProposed(ctx context.Context, id, reviewedBy pgtype.UUID) (Source, error) {
	return s.store.RejectProposed(ctx, id, reviewedBy)
}

// ApproveProposed activates a proposed source (status='active', enabled=true)
// and records the approver. Platform-scoped.
func (s *Service) ApproveProposed(ctx context.Context, id, approvedBy pgtype.UUID) (Source, error) {
	src, err := s.store.ApproveProposed(ctx, id, approvedBy)
	if err != nil {
		return Source{}, err
	}

	s.queueMetadataFetch(ctx, src)
	// intel-channels §2: approval triggers the deep verify probe — it
	// writes probe_report + detected_methods and materializes the source's
	// intel_source_channels rows. Fire-and-forget like the metadata queue.
	s.queueProbeVerify(ctx, src)
	return src, nil
}

// CrawlNow is a no-op service method retained for API surface compatibility.
// The actual crawl-now trigger is handled by the HTTP handler which enqueues
// the intel:fetch_source Asynq task directly (INTEL-011 fix). This method
// verifies the source exists and is enabled.
func (s *Service) CrawlNow(ctx context.Context, id SourceID) error {
	src, err := s.store.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("sources: crawl-now: load source: %w", err)
	}

	if !src.Enabled {
		return errors.New("sources: crawl-now: source is disabled")
	}

	return nil
}

// ProposeInput is the public submission payload for a new source proposal.
//
// Category and SourceType are the legacy single-value fields, kept for
// backward compatibility: Category folds into Categories when Categories is
// empty, and a legacy 'video'/'youtube' SourceType still implies
// source_kind=channel_profile (§106 video routing, now kind-driven).
type ProposeInput struct {
	Name        string
	Domain      string
	SourceURL   string
	Category    string
	Categories  []string
	SourceType  string
	SourceKind  string
	Description *string
	// ProbeSnapshot is the preview-probe report (POST /sources/probe) the
	// submitter attached to the proposal; stored verbatim in probe_report.
	ProbeSnapshot []byte
}

// buildProposeParams validates and normalizes a ProposeInput into create
// params. It is pure — no store access — so every validation failure is
// returned before the uniqueness check and the rules are unit-testable
// without a database.
func buildProposeParams(input ProposeInput) (gen.CreateIntelSourceParams, error) {
	if input.Name == "" {
		return gen.CreateIntelSourceParams{}, fmt.Errorf("%w: name", errpkg.ErrRequired)
	}
	if input.SourceURL == "" {
		return gen.CreateIntelSourceParams{}, fmt.Errorf("%w: source_url", errpkg.ErrRequired)
	}

	// Scheme-less input gets https:// — the probe endpoint accepts bare
	// domains (same normalization) and the form's URL check is loose, so
	// "jobberman.com" must not 400 on the submit step that follows a
	// successful probe.
	sourceURL := strings.TrimSpace(input.SourceURL)
	if !strings.Contains(sourceURL, "://") {
		sourceURL = "https://" + sourceURL
	}

	u, err := url.Parse(sourceURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return gen.CreateIntelSourceParams{}, fmt.Errorf("sources: propose: %w: invalid source_url", errpkg.ErrInvalid)
	}

	domain := input.Domain
	if domain == "" {
		domain = u.Hostname()
	}
	if domain == "" || strings.Contains(domain, ":") {
		return gen.CreateIntelSourceParams{}, fmt.Errorf("sources: propose: %w: invalid domain", errpkg.ErrInvalid)
	}

	// Multi-category proposals (intel-channels §6): at least one category is
	// required. The legacy single 'category' field still counts when the
	// array is absent.
	categories := NormalizeCategories(input.Categories)
	if len(categories) == 0 && input.Category != "" {
		categories = NormalizeCategories([]string{input.Category})
	}
	if len(categories) == 0 {
		return gen.CreateIntelSourceParams{}, fmt.Errorf("%w: categories", errpkg.ErrRequired)
	}

	// Only the submittable category subset is open to public proposals —
	// domains whose channels reach content without AI extraction
	// (job-family, news/article feeds, video). Other taxonomy values are
	// rejected until an extraction provider is configured, mirroring the
	// source_kind IsSubmittable gate below.
	for _, c := range categories {
		if !Category(c).IsSubmittable() {
			return gen.CreateIntelSourceParams{}, fmt.Errorf("sources: propose: %w: category %q is not open for proposals", errpkg.ErrInvalid, c)
		}
	}

	// source_kind drives probe/verify defaults. Absent an explicit kind, a
	// legacy 'video'/'youtube' source_type still implies channel_profile so
	// old clients keep landing on the §106 video-channel path; anything else
	// defaults to publisher.
	kind := SourceKind(input.SourceKind)
	legacyVideoType := false
	if input.SourceKind == "" {
		switch input.SourceType {
		case "video", "youtube":
			kind = SourceKindChannelProfile
			legacyVideoType = true
		default:
			kind = SourceKindPublisher
		}
	}
	if !kind.IsValid() {
		return gen.CreateIntelSourceParams{}, fmt.Errorf("sources: propose: %w: unknown source_kind %q", errpkg.ErrInvalid, input.SourceKind)
	}
	if !kind.IsSubmittable() {
		return gen.CreateIntelSourceParams{}, fmt.Errorf("sources: propose: %w: source_kind %q is reserved for entity-backed sources", errpkg.ErrInvalid, input.SourceKind)
	}

	if len(input.ProbeSnapshot) > 0 {
		var snapshot map[string]json.RawMessage
		if err := json.Unmarshal(input.ProbeSnapshot, &snapshot); err != nil || snapshot == nil {
			return gen.CreateIntelSourceParams{}, fmt.Errorf("sources: propose: %w: probe_snapshot must be a JSON object", errpkg.ErrInvalid)
		}
	}

	// Channels own crawling after approval, so proposed rows are always
	// manual/manual — EXCEPT the §106 video path: a channel_profile on a
	// supported video platform is a video_channel subscription the
	// video:sync_channels poller picks up (kind-driven, replacing the old
	// source_type-driven switch).
	sourceType := "auto"
	if kind == SourceKindChannelProfile {
		if isVideoProviderHost(u.Hostname()) {
			sourceType = string(SourceTypeVideoChannel)
		} else if legacyVideoType {
			// A legacy 'video' proposal on a non-platform host keeps its old
			// rejection — the URL cannot be a channel subscription.
			return gen.CreateIntelSourceParams{}, fmt.Errorf("sources: propose: %w: video sources must be a supported video platform URL", errpkg.ErrInvalid)
		}
	}

	arg := gen.CreateIntelSourceParams{
		Name:                input.Name,
		Domain:              domain,
		SourceUrl:           sourceURL,
		Category:            categories[0], // dual-write: categories[0] is the legacy primary category
		Categories:          categories,
		SourceType:          sourceType,
		SourceKind:          string(kind),
		TrustLevel:          5,
		CrawlMethod:         string(CrawlMethodManual),
		CrawlFrequency:      string(CrawlFrequencyManual),
		CrawlFrequencyMins:  0,
		RobotsPolicy:        []byte("{}"),
		AttributionRequired: false,
		ProviderConfig:      []byte("{}"),
		Enabled:             false,
		Description:         input.Description,
		Status:              "proposed",
	}
	if len(input.ProbeSnapshot) > 0 {
		arg.ProbeReport = input.ProbeSnapshot
	}

	return arg, nil
}

// NormalizeCategories trims, lowercases, and drops empty entries while
// preserving order and removing duplicates. Exported so the admin
// create/update handlers apply the same normalization Propose uses.
func NormalizeCategories(cats []string) []string {
	out := make([]string, 0, len(cats))
	seen := make(map[string]bool, len(cats))
	for _, c := range cats {
		c = strings.ToLower(strings.TrimSpace(c))
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}

	return out
}

// Propose creates a new source proposal from an authenticated user.
// The source is inserted with status='proposed' and enabled=false; an admin
// must approve it before it appears in the public directory.
func (s *Service) Propose(ctx context.Context, userID pgtype.UUID, input ProposeInput) (Source, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.SourceURL = strings.TrimSpace(input.SourceURL)
	input.Domain = strings.TrimSpace(input.Domain)
	input.Category = strings.TrimSpace(input.Category)
	input.SourceType = strings.TrimSpace(input.SourceType)
	input.SourceKind = strings.TrimSpace(input.SourceKind)

	arg, err := buildProposeParams(input)
	if err != nil {
		return Source{}, err
	}

	// Public URLs must be unique per platform.
	if _, err := s.store.GetByURL(ctx, arg.SourceUrl); err == nil {
		return Source{}, errpkg.ErrAlreadyExists
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Source{}, fmt.Errorf("sources: propose: check existing url: %w", err)
	}

	slug := s.uniqueSlug(ctx, slugify(input.Name))

	arg.Slug = &slug
	arg.ProposedBy = userID
	arg.ProposedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}

	return s.store.Create(ctx, arg)
}

// Oneshot system source — the shared container under which user-submitted
// single-URL fetches materialize (intel-channels Phase 2, spec §3
// oneshot). Channels hang off it with crawl_method='oneshot' and are fired
// once by POST /sources/oneshot; ListDueChannels never schedules them.
const (
	oneshotSourceSlug = "user-oneshot-submissions"
	oneshotSourceName = "User Oneshot Submissions"
	oneshotSourceURL  = "https://japaconnect.com/"
	oneshotDomain     = "japaconnect.com"
)

// SubmitOneshot records a user-submitted URL as a oneshot channel on the
// shared system source (created lazily) and returns the channel. The
// UNIQUE(source_id, entry_url, entity_types) constraint dedupes repeats —
// a re-submission returns the existing row with created=false so the
// caller can re-fire a fetch without growing channel rows.
//
// The channel's entity_types come from the caller-supplied categories via
// the category map plus an optional registered entity_hint (prepended —
// a caller-asserted type outranks category inference). Empty entity types
// are allowed — pipeline 'hybrid' tolerates it and extract falls back to
// the source category, landing unresolvable docs in the review queue.
// Callers must pre-validate entityHint against schemas.ForEntityType.
func (s *Service) SubmitOneshot(ctx context.Context, rawURL string, categories []string, entityHint string) (Channel, bool, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return Channel{}, false, fmt.Errorf("sources: oneshot: %w: url", errpkg.ErrRequired)
	}
	if !strings.Contains(rawURL, "://") {
		rawURL = "https://" + rawURL
	}
	if err := validateChannelEntryURL(rawURL, CrawlMethodOneshot, ""); err != nil {
		return Channel{}, false, fmt.Errorf("sources: oneshot: %w", err)
	}

	src, err := s.oneshotSource(ctx)
	if err != nil {
		return Channel{}, false, err
	}

	cats := NormalizeCategories(categories)
	var entityTypes []string
	if len(cats) > 0 {
		entityTypes = ResolveCategoryMap(nil, cats).EntityTypes
	}
	if entityHint != "" && !slices.Contains(entityTypes, entityHint) {
		entityTypes = append([]string{entityHint}, entityTypes...)
	}

	ch, err := s.CreateChannel(ctx, gen.CreateIntelSourceChannelParams{
		SourceID:    src.ID,
		Label:       "Oneshot",
		EntryUrl:    rawURL,
		CrawlMethod: string(CrawlMethodOneshot),
		EntityTypes: entityTypes,
		Pipeline:    string(PipelineHybrid),
		Enabled:     true,
		Origin:      string(ChannelOriginUser),
	})
	if err == nil {
		return ch, true, nil
	}
	if !errors.Is(err, errpkg.ErrAlreadyExists) {
		return Channel{}, false, fmt.Errorf("sources: oneshot: create channel: %w", err)
	}

	// Dedupe hit — find the existing row for re-fire. An explicit
	// resubmission re-arms a channel the consecutive-failures breaker
	// auto-disabled — otherwise the queued fetch_channel would skip
	// silently on !ch.Enabled while the API caller sees 202 "queued".
	channels, lerr := s.store.ListChannels(ctx, src.ID)
	if lerr != nil {
		return Channel{}, false, fmt.Errorf("sources: oneshot: list channels: %w", lerr)
	}
	for _, c := range channels {
		if c.EntryUrl == rawURL && c.CrawlMethod == string(CrawlMethodOneshot) {
			if !c.Enabled {
				if uerr := s.store.UpdateChannelEnabled(ctx, c.ID, true); uerr != nil {
					return Channel{}, false, fmt.Errorf("sources: oneshot: re-enable channel: %w", uerr)
				}
				c.Enabled = true
			}
			return c, false, nil
		}
	}

	return Channel{}, false, fmt.Errorf("sources: oneshot: channel exists but could not be loaded")
}

// oneshotSource returns the shared system source, creating it on first
// submission. A concurrent first submission can race the insert — on a
// slug/URL conflict the row is re-fetched.
func (s *Service) oneshotSource(ctx context.Context) (Source, error) {
	if src, err := s.store.GetBySlug(ctx, oneshotSourceSlug); err == nil {
		return src, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Source{}, fmt.Errorf("sources: oneshot: load system source: %w", err)
	}

	slug := oneshotSourceSlug
	src, err := s.store.Create(ctx, gen.CreateIntelSourceParams{
		Name:       oneshotSourceName,
		Slug:       &slug,
		Domain:     oneshotDomain,
		SourceUrl:  oneshotSourceURL,
		Category:   "announcement",
		Categories: []string{"announcement"},
		// "system" + crawl_method manual → verifySkipSource skips a reprobe
		// (an admin reprobe must not probe japaconnect.com and materialize
		// channels onto the shared submissions source).
		SourceType:         "system",
		SourceKind:         string(SourceKindAggregator),
		TrustLevel:         5,
		CrawlMethod:        string(CrawlMethodManual),
		CrawlFrequency:     string(CrawlFrequencyManual),
		CrawlFrequencyMins: 0,
		RobotsPolicy:       []byte("{}"),
		ProviderConfig:     []byte("{}"),
		Enabled:            true,
		Status:             "active",
	})
	if err != nil {
		if retry, gerr := s.store.GetBySlug(ctx, oneshotSourceSlug); gerr == nil {
			return retry, nil // raced with a concurrent first submission
		}

		return Source{}, fmt.Errorf("sources: oneshot: create system source: %w", err)
	}

	return src, nil
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (s *Service) CreateAutoForVideo(ctx context.Context, userID pgtype.UUID, title, sourceURL, thumbnailURL *string) (Source, error) {
	if title == nil || *title == "" {
		return Source{}, fmt.Errorf("%w: name", errpkg.ErrRequired)
	}
	if sourceURL == nil || *sourceURL == "" {
		return Source{}, fmt.Errorf("%w: source_url", errpkg.ErrRequired)
	}

	existing, err := s.store.GetByURL(ctx, *sourceURL)
	if err == nil {
		s.queueMetadataFetch(ctx, existing)
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

	slug := s.uniqueSlug(ctx, *title)

	arg := gen.CreateIntelSourceParams{
		Name:               *title,
		Slug:               &slug,
		Domain:             domain,
		SourceUrl:          *sourceURL,
		Category:           "video",
		SourceType:         string(SourceTypeVideoChannel),
		SourceKind:         "publisher",
		TrustLevel:         2,
		CrawlMethod:        "manual",
		CrawlFrequency:     "manual",
		CrawlFrequencyMins: 0,
		Enabled:            true,
		Status:             "active",
		LogoUrl:            thumbnailURL,
	}

	return s.Create(ctx, arg)
}

func (s *Service) CreateSourceFromEntity(ctx context.Context, entityID, userID pgtype.UUID, entityType, name, slug, sourceURL, category string) (Source, error) {
	if name == "" {
		return Source{}, fmt.Errorf("%w: name", errpkg.ErrRequired)
	}
	if slug == "" {
		return Source{}, fmt.Errorf("%w: slug", errpkg.ErrRequired)
	}

	if sourceURL != "" {
		existing, err := s.store.GetByURL(ctx, sourceURL)
		if err == nil {
			s.queueMetadataFetch(ctx, existing)
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

	return s.Create(ctx, arg)
}

func (s *Service) CreateAutoForComparison(ctx context.Context, userID pgtype.UUID, sourceName, sourceURL *string, trustLevel int32) (Source, error) {
	if sourceName == nil || *sourceName == "" {
		return Source{}, fmt.Errorf("%w: source_name", errpkg.ErrRequired)
	}

	existing, err := s.store.GetByURL(ctx, derefString(sourceURL))
	if err == nil {
		s.queueMetadataFetch(ctx, existing)
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

	slug := s.uniqueSlug(ctx, *sourceName)

	arg := gen.CreateIntelSourceParams{
		Name:               *sourceName,
		Slug:               &slug,
		Domain:             domain,
		SourceUrl:          derefString(sourceURL),
		Category:           "comparison",
		SourceType:         "comparison_api",
		SourceKind:         "publisher",
		TrustLevel:         int16(max(1, min(5, trustLevel))), //nolint:gosec // G115: trust level is clamped to 1..5.
		CrawlMethod:        "api",
		CrawlFrequency:     "daily",
		CrawlFrequencyMins: 0,
		Enabled:            true,
		Status:             "active",
	}

	return s.Create(ctx, arg)
}

func (s *Service) uniqueSlug(ctx context.Context, base string) string {
	base = slugify(base)
	if base == "" || base == "untitled" {
		base = "source"
	}

	candidate := base
	for i := 0; i < 100; i++ {
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", base, i)
		}
		_, err := s.store.GetBySlug(ctx, candidate)
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
func (s *Service) ToggleSourceBlock(ctx context.Context, blockerID, sourceID pgtype.UUID) (bool, error) {
	blocked, err := s.store.IsSourceBlocked(ctx, blockerID, sourceID)
	if err != nil {
		return false, fmt.Errorf("sources: toggle block: check: %w", err)
	}

	if blocked {
		if err := s.store.UnblockSource(ctx, blockerID, sourceID); err != nil {
			return false, fmt.Errorf("sources: toggle block: unblock: %w", err)
		}
		return false, nil
	}

	_, err = s.store.BlockSource(ctx, blockerID, sourceID)
	if err != nil {
		return false, fmt.Errorf("sources: toggle block: block: %w", err)
	}
	return true, nil
}

// IsSourceBlocked returns whether the user has blocked the given source.
func (s *Service) IsSourceBlocked(ctx context.Context, blockerID, sourceID pgtype.UUID) (bool, error) {
	return s.store.IsSourceBlocked(ctx, blockerID, sourceID)
}

// ListBlockedSourceIDs returns the source IDs blocked by the given user.
func (s *Service) ListBlockedSourceIDs(ctx context.Context, blockerID pgtype.UUID) ([]pgtype.UUID, error) {
	return s.store.ListBlockedSourceIDs(ctx, blockerID)
}

// --- Channels ---
//
// A channel is one entry point (entry_url + crawl_method + pipeline) under a
// source. Validation lives here; persistence is delegated to the Store.

// CreateChannel registers a new channel under a source. Unset enum fields
// get their schema defaults (relevance_mode='filtered', origin='admin'),
// an unset pipeline takes the per-method default (defaultChannelPipeline —
// curate for itemized/inbound methods, extract otherwise — matching probe
// materialization so an admin-created email/webhook/search channel reaches
// its deterministic curator, not the LLM extractor), and provider_config
// defaults to '{}' because the column is NOT NULL — a nil []byte would
// bind SQL NULL and violate the constraint.
func (s *Service) CreateChannel(ctx context.Context, arg gen.CreateIntelSourceChannelParams) (Channel, error) {
	arg.Label = strings.TrimSpace(arg.Label)
	arg.EntryUrl = strings.TrimSpace(arg.EntryUrl)

	if arg.Pipeline == "" {
		arg.Pipeline = string(defaultChannelPipeline(CrawlMethod(arg.CrawlMethod)))
	}
	if arg.RelevanceMode == "" {
		arg.RelevanceMode = string(RelevanceFiltered)
	}
	if arg.Origin == "" {
		arg.Origin = string(ChannelOriginAdmin)
	}
	if len(arg.ProviderConfig) == 0 {
		arg.ProviderConfig = []byte("{}")
	}
	// entity_types / relevance_topics / relevance_url_patterns are
	// text[] NOT NULL DEFAULT '{}' — a nil slice binds SQL NULL and
	// violates the constraint (same failure class as provider_config in
	// Create). Normalize at this chokepoint so probe materialization and
	// admin creation both insert cleanly.
	if arg.EntityTypes == nil {
		arg.EntityTypes = []string{}
	}
	if arg.RelevanceTopics == nil {
		arg.RelevanceTopics = []string{}
	}
	if arg.RelevanceUrlPatterns == nil {
		arg.RelevanceUrlPatterns = []string{}
	}
	if arg.CrawlFrequencyMins <= 0 {
		arg.CrawlFrequencyMins = 1440
	}

	// Load the parent source: it must exist anyway (cleaner than an FK
	// error) and its host[:port] feeds the entry_url literal check — a
	// private-IP literal matching the source's own host is allowed
	// (same-host channels cannot escalate past the registered target, and
	// probe materialization on dev/test 127.0.0.1 sources depends on it);
	// a literal on any other host is rejected (SSRF, final review C1c).
	allowedLiteralHost := ""
	if parent, err := s.Get(ctx, arg.SourceID); err == nil {
		if u, uerr := url.Parse(parent.SourceUrl); uerr == nil {
			allowedLiteralHost = u.Host
		}
	}

	if err := validateChannelCreate(arg, allowedLiteralHost); err != nil {
		return Channel{}, err
	}

	return s.store.CreateChannel(ctx, arg)
}

// GetChannel retrieves a channel by ID.
func (s *Service) GetChannel(ctx context.Context, id ChannelID) (Channel, error) {
	return s.store.GetChannel(ctx, id)
}

// ListChannels returns all channels owned by a source.
func (s *Service) ListChannels(ctx context.Context, sourceID SourceID) ([]Channel, error) {
	return s.store.ListChannels(ctx, sourceID)
}

// ListDueChannels returns up to limit channels due for a crawl (enabled,
// parent source active, interval elapsed; inbound and manual excluded).
func (s *Service) ListDueChannels(ctx context.Context, limit int32) ([]Channel, error) {
	return s.store.ListDueChannels(ctx, limit)
}

// UpdateChannel applies a COALESCE-style patch to a channel's scalar fields.
// The existing row is loaded first so the patch can be checked against the
// effective state (e.g. switching pipeline to extract/curate while
// entity_types is empty is rejected) and so missing channels report through
// the store's wrapped not-found error.
func (s *Service) UpdateChannel(ctx context.Context, arg gen.UpdateIntelSourceChannelParams) (Channel, error) {
	if !arg.ID.Valid {
		return Channel{}, fmt.Errorf("sources: update channel: %w: id", errpkg.ErrRequired)
	}

	// Load the existing row first — field-level validation needs the
	// channel's current crawl_method to apply the relaxed entry_url rule
	// to inbound (email/webhook) channels.
	existing, err := s.store.GetChannel(ctx, arg.ID)
	if err != nil {
		return Channel{}, err
	}
	if err := validateChannelUpdate(arg, CrawlMethod(existing.CrawlMethod)); err != nil {
		return Channel{}, err
	}

	// Repurposing an inbound channel to a pull method must leave a fetchable
	// entry point — a mailto:/opaque entry_url that was legal for email is
	// not legal for rss/static. When the same patch also sets entry_url,
	// validateChannelUpdate already enforced the pull rule on it.
	if arg.CrawlMethod != nil && arg.EntryUrl == nil &&
		InboundMethod(CrawlMethod(existing.CrawlMethod)) && !InboundMethod(CrawlMethod(*arg.CrawlMethod)) {
		if err := validateChannelEntryURL(existing.EntryUrl, CrawlMethod(*arg.CrawlMethod), ""); err != nil {
			return Channel{}, fmt.Errorf("sources: update channel: crawl_method change requires a fetchable entry_url: %w", err)
		}
	}

	if arg.Pipeline != nil &&
		pipelineRequiresEntityTypes(Pipeline(*arg.Pipeline)) &&
		len(existing.EntityTypes) == 0 {
		return Channel{}, fmt.Errorf("sources: update channel: %w: entity_types required for %s pipeline", errpkg.ErrInvalid, *arg.Pipeline)
	}

	return s.store.UpdateChannel(ctx, arg)
}

// UpdateChannelEntityTypes replaces a channel's entity_types array. An empty
// array is an explicit clear (the runtime falls back to source-level
// entity_types then category), so no minimum-length validation applies.
func (s *Service) UpdateChannelEntityTypes(ctx context.Context, arg gen.UpdateIntelChannelEntityTypesParams) (Channel, error) {
	return s.store.UpdateChannelEntityTypes(ctx, arg)
}

// UpdateChannelRelevance replaces a channel's relevance gate fields.
// relevance_mode must be a known mode; topics/patterns may be cleared.
func (s *Service) UpdateChannelRelevance(ctx context.Context, arg gen.UpdateIntelChannelRelevanceParams) (Channel, error) {
	if !RelevanceMode(arg.RelevanceMode).IsValid() {
		return Channel{}, fmt.Errorf("sources: update channel relevance: %w: unknown relevance_mode %q", errpkg.ErrInvalid, arg.RelevanceMode)
	}

	return s.store.UpdateChannelRelevance(ctx, arg)
}

// UpdateChannelCrawlStatus records the outcome of a crawl attempt on the
// channel row (last_crawled_at, success/error stamps, consecutive_failures).
func (s *Service) UpdateChannelCrawlStatus(ctx context.Context, id ChannelID, success bool, lastErrorMessage *string) error {
	return s.store.UpdateChannelCrawlStatus(ctx, gen.UpdateIntelChannelCrawlStatusParams{
		ID:               id,
		Success:          success,
		LastErrorMessage: lastErrorMessage,
	})
}

// DeleteChannel removes a channel from a source.
func (s *Service) DeleteChannel(ctx context.Context, id ChannelID) error {
	return s.store.DeleteChannel(ctx, id)
}

// CreateInboundEvent stores a push delivery (email/WebSub) destined for an
// inbound channel (Phase 3). The content_hash is the caller-computed
// idempotency/dedup key.
func (s *Service) CreateInboundEvent(ctx context.Context, arg gen.CreateIntelInboundEventParams) (InboundEvent, error) {
	return s.store.CreateInboundEvent(ctx, arg)
}

// GetInboundEvent loads one inbound event — the intel:inbound_email task's
// hand-off read after the HTTP layer persists the delivery.
func (s *Service) GetInboundEvent(ctx context.Context, id pgtype.UUID) (InboundEvent, error) {
	return s.store.GetInboundEvent(ctx, id)
}

// MarkInboundEvent stamps an event's terminal status
// (received|processing → processed|failed|ignored) and processed_at.
func (s *Service) MarkInboundEvent(ctx context.Context, arg gen.MarkIntelInboundEventParams) error {
	return s.store.MarkInboundEvent(ctx, arg)
}

// ClaimInboundEvent atomically transitions an event received → processing;
// true = this caller owns the work now.
func (s *Service) ClaimInboundEvent(ctx context.Context, id pgtype.UUID) (bool, error) {
	return s.store.ClaimInboundEvent(ctx, id)
}

// RedriveInboundEvent resets a failed/ignored/stuck-processing event back
// to 'received' for re-processing; 'processing' events only reset once the
// claim is older than stuckSecs. false = terminal, actively claimed, or
// still waiting.
func (s *Service) RedriveInboundEvent(ctx context.Context, id pgtype.UUID, stuckSecs float64) (bool, error) {
	return s.store.RedriveInboundEvent(ctx, id, stuckSecs)
}

// PurgeInboundEvents deletes inbound events older than retentionDays —
// the intel:purge_inbound_events retention sweep (raw payloads carry
// sender PII, so retention is bounded).
func (s *Service) PurgeInboundEvents(ctx context.Context, retentionDays int32) (int64, error) {
	return s.store.PurgeInboundEvents(ctx, retentionDays)
}

// NoteChannelError flags a non-crawl error on the channel row without
// touching failure counters or crawl timestamps.
func (s *Service) NoteChannelError(ctx context.Context, arg gen.NoteIntelChannelErrorParams) error {
	return s.store.NoteChannelError(ctx, arg)
}

// GetEmailChannelByAddress resolves an inbound email delivery's recipient
// to the enabled email channel carrying it in provider_config.inbound_address
// (Phase 3 inbound routing). pgx.ErrNoRows = no such channel.
func (s *Service) GetEmailChannelByAddress(ctx context.Context, address string) (Channel, error) {
	return s.store.GetEmailChannelByAddress(ctx, address)
}
