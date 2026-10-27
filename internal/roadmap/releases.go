// Package roadmap — versioned releases domain (§113, ADR-135).
//
// roadmap_releases groups shipped items and commit-sourced
// roadmap_release_entries into named, per-surface versions: platform
// waves (free labels, no support contract) and mobile semver lines
// (eol_at/is_esr support windows). support_state is computed at read
// time — never stored — so it can't drift. Commit entries are parsed
// conventional commits deduped by SHA; only user-facing types render on
// public routes.
//
// Ref: japaconnect-engineering/sections/113-public-product-roadmap.md
//      (Versioned Releases, Commit-Sourced Entries)
package roadmap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	jcvalkey "github.com/jedafe/japa/internal/valkey"
)

// ReleaseStatus is the lifecycle state of a roadmap release.
type ReleaseStatus string

const (
	ReleaseDraft     ReleaseStatus = "draft"
	ReleasePublished ReleaseStatus = "published"
	ReleaseYanked    ReleaseStatus = "yanked"
)

// ReleaseSurface identifies which product surface a release versions.
type ReleaseSurface string

const (
	SurfacePlatform ReleaseSurface = "platform"
	SurfaceMobile   ReleaseSurface = "mobile"
)

// Support states returned by ComputeSupportState — derived, never stored.
const (
	SupportDraft     = "draft"
	SupportYanked    = "yanked"
	SupportUpcoming  = "upcoming"
	SupportLatest    = "latest"
	SupportESR       = "esr"
	SupportEOL       = "eol"
	SupportSupported = "supported"
)

// PublicCommitTypes render on public routes; everything else is
// internal-only (stored for admin triage, never rendered).
var PublicCommitTypes = []string{
	"feature", "fix", "ui", "performance",
	"security", "translation", "accessibility",
}

var allCommitTypes = map[string]bool{
	"feature": true, "fix": true, "ui": true, "performance": true,
	"security": true, "translation": true, "accessibility": true,
	"docs": true, "chore": true, "refactor": true, "test": true,
	"ci": true, "other": true,
}

// reservedReleaseSlugs are public route segments that must never become
// release slugs.
var reservedReleaseSlugs = map[string]bool{"unreleased": true, "feed.xml": true}

var (
	mobileSemverRe = regexp.MustCompile(`^v?\d+\.\d+\.\d+`)
	conventionalRe = regexp.MustCompile(`^(\w+)(?:\(([^)]*)\))?(!)?:\s*(.+)$`)
)

// commitTypeAliases maps conventional-commit prefixes to stored types.
var commitTypeAliases = map[string]string{
	"feat": "feature", "fix": "fix", "ui": "ui", "ux": "ui", "style": "ui",
	"perf": "performance", "security": "security", "sec": "security",
	"i18n": "translation", "l10n": "translation", "a11y": "accessibility",
	"docs": "docs", "refactor": "refactor", "test": "test",
	"ci": "ci", "build": "ci", "chore": "chore", "revert": "chore",
}

// releasesCacheKey is the Valkey key for the public releases payload.
// Ref: conventions/valkey-keys-catalog.md (jc:roadmap:releases).
const releasesCacheKey = "jc:roadmap:releases"

var (
	ErrItemNotShipped        = errors.New("item_not_shipped")
	ErrInvalidReleaseVersion = errors.New("invalid_version")
	ErrInvalidCommitType     = errors.New("invalid_commit_type")
)

// validReleaseTransitions — no-op transitions permitted via from == to.
var validReleaseTransitions = map[ReleaseStatus][]ReleaseStatus{
	ReleaseDraft:     {ReleasePublished},
	ReleasePublished: {ReleaseYanked},
	ReleaseYanked:    {ReleasePublished},
}

// IsValidReleaseTransition reports whether a lifecycle jump is allowed.
func IsValidReleaseTransition(from, to ReleaseStatus) bool {
	if from == to {
		return true
	}
	for _, s := range validReleaseTransitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

// ValidReleaseStatus reports whether s is a lifecycle status.
func ValidReleaseStatus(s string) bool {
	switch ReleaseStatus(s) {
	case ReleaseDraft, ReleasePublished, ReleaseYanked:
		return true
	}
	return false
}

// ComputeSupportState derives the display state at read time.
// supersededAt is the next published release's released_at in the same
// surface (LEAD window — nil for the newest or non-public rows).
func ComputeSupportState(r gen.RoadmapRelease, supersededAt *time.Time, now time.Time) string {
	switch ReleaseStatus(r.Status) {
	case ReleaseDraft:
		return SupportDraft
	case ReleaseYanked:
		return SupportYanked
	}
	if !r.ReleasedAt.Valid || r.ReleasedAt.Time.After(now) {
		return SupportUpcoming
	}
	if r.IsEsr && (!r.EolAt.Valid || r.EolAt.Time.After(now)) {
		return SupportESR
	}
	if r.EolAt.Valid && !r.EolAt.Time.After(now) {
		return SupportEOL
	}
	if supersededAt == nil {
		return SupportLatest
	}
	return SupportSupported
}

// ReleaseDTO is the public/admin release shape.
type ReleaseDTO struct {
	ID            string     `json:"id"`
	Surface       string     `json:"surface"`
	Version       string     `json:"version"`
	Slug          string     `json:"slug"`
	Title         *string    `json:"title,omitempty"`
	Summary       *string    `json:"summary,omitempty"`
	Status        string     `json:"status"`
	SupportState  string     `json:"support_state"`
	IsESR         bool       `json:"is_esr"`
	ItemCount     int64      `json:"item_count"`
	ReleasedAt    *time.Time `json:"released_at,omitempty"`
	EOLAt         *time.Time `json:"eol_at,omitempty"`
	SupersededAt  *time.Time `json:"superseded_at,omitempty"`
}

// ReleaseListResponse is the GET /roadmap/releases payload.
type ReleaseListResponse struct {
	Releases        []ReleaseDTO `json:"releases"`
	UnreleasedCount int64        `json:"unreleased_count"`
}

// EntryDTO is one commit-sourced changelog entry.
type EntryDTO struct {
	ID          string     `json:"id"`
	CommitType  string     `json:"commit_type"`
	Scope       *string    `json:"scope,omitempty"`
	IsBreaking  bool       `json:"is_breaking"`
	Title       string     `json:"title"`
	CommitSHA   *string    `json:"commit_sha,omitempty"`
	CommitURL   *string    `json:"commit_url,omitempty"`
	AuthorName  *string    `json:"author_name,omitempty"`
	CommittedAt *time.Time `json:"committed_at,omitempty"`
	ItemID      *string    `json:"item_id,omitempty"`
	ReleaseID   *string    `json:"release_id,omitempty"`
}

// ReleaseDetail is the GET /roadmap/releases/{slug} payload.
type ReleaseDetail struct {
	ReleaseDTO
	Body    *string                          `json:"body,omitempty"`
	Items   []gen.ListRoadmapReleaseItemsRow `json:"items"`
	Entries []EntryDTO                       `json:"entries"`
}

// GitHubCommit mirrors the GitHub push-event commit object so
// github.event.commits decodes verbatim (id is the sha).
type GitHubCommit struct {
	ID        string   `json:"id"`
	Message   string   `json:"message"`
	URL       string   `json:"url"`
	Timestamp string   `json:"timestamp"`
	Author    struct {
		Name string `json:"name"`
	} `json:"author"`
	Added    []string `json:"added"`
	Modified []string `json:"modified"`
	Removed  []string `json:"removed"`
}

// ReleaseInput carries create/patch fields — pointer fields distinguish
// "absent" from "set" on PATCH.
type ReleaseInput struct {
	Version    *string
	Slug       *string
	Title      *string
	Summary    *string
	Body       *string
	Surface    *string
	IsESR      *bool
	ReleasedAt *string
	EOLAt      *string
	SortOrder  *int32
	Status     *string
}

// EntryFilter scopes the admin entries queue list.
type EntryFilter struct {
	Unreleased bool
	CommitType string
	Scope      string
	PageSize   int32
	PageOffset int32
}

// EntryListResponse is the paginated admin entries payload.
type EntryListResponse struct {
	Entries    []gen.RoadmapReleaseEntry `json:"entries"`
	Total      int64                     `json:"total"`
	Page       int32                     `json:"page"`
	TotalPages int32                     `json:"total_pages"`
}

// AdminReleaseListFilter scopes the admin releases DataTable.
type AdminReleaseListFilter struct {
	Surface        string
	Status         string
	IncludeDeleted bool
	PageSize       int32
	PageOffset     int32
}

// AdminReleaseListResponse is the paginated admin releases payload.
type AdminReleaseListResponse struct {
	Releases   []gen.ListAdminRoadmapReleasesRow `json:"releases"`
	Total      int64                             `json:"total"`
	Page       int32                             `json:"page"`
	TotalPages int32                             `json:"total_pages"`
}

// ---------------------------------------------------------------------------
// Commit parsing + scope resolution
// ---------------------------------------------------------------------------

// ParseConventionalCommit splits `type(scope)!: subject` into its parts.
// Non-conforming subjects return commitType "other" with the raw subject.
func ParseConventionalCommit(subject string) (commitType, scope, title string, breaking bool) {
	m := conventionalRe.FindStringSubmatch(strings.TrimSpace(subject))
	if m == nil {
		return "other", "", strings.TrimSpace(subject), false
	}
	t, ok := commitTypeAliases[strings.ToLower(m[1])]
	if !ok {
		t = "other"
	}
	return t, m[2], strings.TrimSpace(m[4]), m[3] == "!"
}

// ResolveScope maps a commit's scope/files to a feature-flag key where
// possible. Order: exact flag key → {scope}.enabled → feature.{scope} →
// longest-prefix file-path map (system_settings roadmap.commit_scope_paths)
// → raw scope string (display-only chip).
// flagKeys is the preloaded set of existing flag keys — pass nil to load
// on demand (single-call use; IngestCommits preloads once per batch to
// avoid an N+1 of per-commit flag lookups).
func (s *Service) ResolveScope(ctx context.Context, scope string, files []string, flagKeys map[string]bool) string {
	if scope != "" {
		for _, candidate := range []string{scope, scope + ".enabled", "feature." + scope} {
			if flagKeys != nil {
				if flagKeys[candidate] {
					return candidate
				}
			} else if _, err := s.store.read.GetFeatureFlagRowByKey(ctx, candidate); err == nil {
				return candidate
			}
		}
	}
	if len(files) > 0 {
		if mapped := s.scopeFromPaths(ctx, files); mapped != "" {
			return mapped
		}
	}
	return scope
}

// scopeFromPaths walks the commit's file list against the JSON prefix map
// in roadmap.commit_scope_paths; longest matching prefix wins.
func (s *Service) scopeFromPaths(ctx context.Context, files []string) string {
	if s.settings == nil {
		return ""
	}
	raw, err := s.settings.GetSetting(ctx, "roadmap.commit_scope_paths")
	if err != nil || raw == "" {
		return ""
	}
	var pathMap map[string]string
	if err := json.Unmarshal([]byte(raw), &pathMap); err != nil {
		return ""
	}
	return matchScopePaths(pathMap, files)
}

// matchScopePaths walks the commit's file list against the prefix map;
// the longest matching prefix wins. Prefixes match on path boundaries —
// "internal/blog" hits "internal/blog/x" and "internal/blog.go" but not
// "internal/blogfoo/x".
func matchScopePaths(pathMap map[string]string, files []string) string {
	best, bestLen := "", 0
	for _, f := range files {
		for prefix, scope := range pathMap {
			boundary := strings.HasPrefix(f, prefix+"/") ||
				strings.HasPrefix(f, prefix+".") || f == prefix
			if boundary && len(prefix) > bestLen {
				best, bestLen = scope, len(prefix)
			}
		}
	}
	return best
}

// ---------------------------------------------------------------------------
// Cache invalidation
// ---------------------------------------------------------------------------

// invalidateReleases drops the cached public releases payload after any
// release, release-entry, or item-release mutation. Best-effort.
func (s *Service) invalidateReleases(ctx context.Context) {
	if s.vk == nil {
		return
	}
	if err := s.vk.Del(ctx, releasesCacheKey).Err(); err != nil {
		slog.Warn("roadmap releases cache invalidation failed", "error", err)
	}
}

// ---------------------------------------------------------------------------
// Public reads
// ---------------------------------------------------------------------------

// ListPublicReleases returns the published release list + unreleased
// count — Valkey-cached under jc:roadmap:releases (same
// GetOrComputeCompat pattern as the board).
func (s *Service) ListPublicReleases(ctx context.Context) (ReleaseListResponse, error) {
	ttl := time.Duration(s.settingsInt(ctx, "roadmap.board_cache_ttl_seconds", 60)) * time.Second

	fetch := func(ctx context.Context) (ReleaseListResponse, error) {
		rows, err := s.store.read.ListPublicRoadmapReleases(ctx)
		if err != nil {
			return ReleaseListResponse{}, fmt.Errorf("listing public releases: %w", err)
		}
		unreleased, err := s.store.read.CountUnreleasedEntries(ctx)
		if err != nil {
			return ReleaseListResponse{}, fmt.Errorf("counting unreleased entries: %w", err)
		}

		now := time.Now()
		out := ReleaseListResponse{Releases: make([]ReleaseDTO, 0, len(rows)), UnreleasedCount: unreleased}
		for _, r := range rows {
			dto := releaseDTO(gen.RoadmapRelease{
				ID: r.ID, Surface: r.Surface, Version: r.Version, Slug: r.Slug,
				Title: r.Title, Summary: r.Summary, Status: r.Status, IsEsr: r.IsEsr,
				ReleasedAt: r.ReleasedAt, EolAt: r.EolAt,
			}, tsToPtr(supersededToTime(r.SupersededAt)), now)
			dto.ItemCount = r.ItemCount
			out.Releases = append(out.Releases, dto)
		}
		return out, nil
	}

	if s.vk != nil {
		return jcvalkey.GetOrComputeCompat(ctx, s.vk, releasesCacheKey, ttl, fetch)
	}

	return fetch(ctx)
}

// GetReleaseBySlug returns one published/yanked release with its shipped
// items and public-type entries. ?scope= filters entries (and items via
// entry linkage).
func (s *Service) GetReleaseBySlug(ctx context.Context, slug, scope string) (ReleaseDetail, error) {
	r, err := s.store.read.GetRoadmapReleaseBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ReleaseDetail{}, ErrNotFound
		}
		return ReleaseDetail{}, fmt.Errorf("getting release by slug: %w", err)
	}
	items, err := s.store.read.ListRoadmapReleaseItems(ctx, gen.ListRoadmapReleaseItemsParams{
		ReleaseID: r.ID, Scope: scope,
	})
	if err != nil {
		return ReleaseDetail{}, fmt.Errorf("listing release items: %w", err)
	}
	entries, err := s.store.read.ListRoadmapReleaseEntries(ctx, gen.ListRoadmapReleaseEntriesParams{
		ReleaseID: r.ID, Scope: scope,
	})
	if err != nil {
		return ReleaseDetail{}, fmt.Errorf("listing release entries: %w", err)
	}
	now := time.Now()
	if items == nil {
		items = make([]gen.ListRoadmapReleaseItemsRow, 0) // null-safe JSON — the detail page reads items.length
	}
	out := ReleaseDetail{
		ReleaseDTO: releaseDTO(r, s.successorReleasedAt(ctx, r), now),
		Body:       r.Body,
		Items:      items,
		Entries:    make([]EntryDTO, 0, len(entries)),
	}
	for _, e := range entries {
		out.Entries = append(out.Entries, entryDTO(e))
	}
	return out, nil
}

// successorReleasedAt finds the release that superseded r — the oldest
// published release in the same surface released after r — matching the
// LEAD() window semantics on the timeline list. Uses the (cached) public
// list; returns nil when r is the newest.
func (s *Service) successorReleasedAt(ctx context.Context, r gen.RoadmapRelease) *time.Time {
	if !r.ReleasedAt.Valid {
		return nil
	}
	list, err := s.ListPublicReleases(ctx)
	if err != nil {
		return nil
	}
	var best *time.Time
	for _, rel := range list.Releases {
		if rel.Surface != r.Surface || rel.ID == r.ID.String() || rel.ReleasedAt == nil {
			continue
		}
		if rel.ReleasedAt.After(r.ReleasedAt.Time) &&
			(best == nil || rel.ReleasedAt.Before(*best)) {
			best = rel.ReleasedAt
		}
	}
	return best
}

// ListUnreleasedEntries returns the public-type queue (the
// "In development" strip).
func (s *Service) ListUnreleasedEntries(ctx context.Context, scope string) ([]EntryDTO, error) {
	rows, err := s.store.read.ListPublicUnreleasedEntries(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("listing unreleased entries: %w", err)
	}
	out := make([]EntryDTO, 0, len(rows))
	for _, e := range rows {
		out = append(out, entryDTO(e))
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Admin writes
// ---------------------------------------------------------------------------

// AdminListReleases returns the paginated DataTable payload.
func (s *Service) AdminListReleases(ctx context.Context, f AdminReleaseListFilter) (AdminReleaseListResponse, error) {
	pageSize := f.PageSize
	if pageSize <= 0 || pageSize > MaxPageSize {
		pageSize = DefaultPageSize
	}
	rows, err := s.store.read.ListAdminRoadmapReleases(ctx, gen.ListAdminRoadmapReleasesParams{
		IncludeDeleted: f.IncludeDeleted, Surface: f.Surface, Status: f.Status,
		PageSize: pageSize, PageOffset: f.PageOffset,
	})
	if err != nil {
		return AdminReleaseListResponse{}, fmt.Errorf("listing releases: %w", err)
	}
	total, err := s.store.read.CountAdminRoadmapReleases(ctx, gen.CountAdminRoadmapReleasesParams{
		IncludeDeleted: f.IncludeDeleted, Surface: f.Surface, Status: f.Status,
	})
	if err != nil {
		return AdminReleaseListResponse{}, fmt.Errorf("counting releases: %w", err)
	}
	page := f.PageOffset/pageSize + 1
	totalPages := int32((total + int64(pageSize) - 1) / int64(pageSize))
	return AdminReleaseListResponse{Releases: rows, Total: total, Page: page, TotalPages: totalPages}, nil
}

// AdminGetRelease returns one release row (any status, incl. deleted).
func (s *Service) AdminGetRelease(ctx context.Context, id pgtype.UUID) (gen.RoadmapRelease, error) {
	r, err := s.store.read.GetRoadmapReleaseByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.RoadmapRelease{}, ErrNotFound
		}
		return gen.RoadmapRelease{}, fmt.Errorf("getting release: %w", err)
	}
	return r, nil
}

// AdminCreateRelease creates a draft (or directly-published) release.
func (s *Service) AdminCreateRelease(ctx context.Context, in ReleaseInput) (gen.RoadmapRelease, error) {
	params, err := s.buildReleaseParams(ctx, gen.RoadmapRelease{}, in, true)
	if err != nil {
		return gen.RoadmapRelease{}, err
	}
	status := string(ReleaseDraft)
	if in.Status != nil && *in.Status != "" {
		if !ValidReleaseStatus(*in.Status) || ReleaseStatus(*in.Status) == ReleaseYanked {
			return gen.RoadmapRelease{}, errpkg.ErrInvalid
		}
		status = *in.Status
	}
	row, err := s.store.write.CreateRoadmapRelease(ctx, gen.CreateRoadmapReleaseParams{
		Surface: params.Surface, Version: params.Version, Slug: params.Slug,
		Title: params.Title, Summary: params.Summary, Body: params.Body,
		Status: status, IsEsr: params.IsEsr, ReleasedAt: params.ReleasedAt,
		EolAt: params.EolAt, SortOrder: params.SortOrder,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return gen.RoadmapRelease{}, errpkg.ErrDuplicate
		}
		return gen.RoadmapRelease{}, fmt.Errorf("creating release: %w", err)
	}
	s.invalidateReleases(ctx)
	return row, nil
}

// AdminUpdateRelease patches a release row.
func (s *Service) AdminUpdateRelease(ctx context.Context, id pgtype.UUID, in ReleaseInput) (gen.RoadmapRelease, error) {
	current, err := s.AdminGetRelease(ctx, id)
	if err != nil {
		return gen.RoadmapRelease{}, err
	}
	if current.DeletedAt.Valid {
		return gen.RoadmapRelease{}, fmt.Errorf("%w: release is deleted", ErrNotFound)
	}
	params, err := s.buildReleaseParams(ctx, current, in, false)
	if err != nil {
		return gen.RoadmapRelease{}, err
	}
	row, err := s.store.write.UpdateRoadmapRelease(ctx, gen.UpdateRoadmapReleaseParams{
		Version: params.Version, Slug: params.Slug, Title: params.Title,
		Summary: params.Summary, Body: params.Body, Surface: params.Surface,
		IsEsr: params.IsEsr, ReleasedAt: params.ReleasedAt, EolAt: params.EolAt,
		SortOrder: params.SortOrder, ID: id,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return gen.RoadmapRelease{}, errpkg.ErrDuplicate
		}
		return gen.RoadmapRelease{}, fmt.Errorf("updating release: %w", err)
	}
	s.invalidateReleases(ctx)
	return row, nil
}

// buildReleaseParams merges input over the current row (or defaults on
// create) and validates surface/version/slug.
func (s *Service) buildReleaseParams(ctx context.Context, cur gen.RoadmapRelease, in ReleaseInput, create bool) (gen.UpdateRoadmapReleaseParams, error) {
	p := gen.UpdateRoadmapReleaseParams{
		Version: cur.Version, Slug: cur.Slug, Title: cur.Title,
		Summary: cur.Summary, Body: cur.Body, Surface: cur.Surface,
		IsEsr: cur.IsEsr, ReleasedAt: cur.ReleasedAt, EolAt: cur.EolAt,
		SortOrder: cur.SortOrder,
	}
	if create {
		p.Surface = string(SurfacePlatform)
	}
	if in.Surface != nil {
		if ReleaseSurface(*in.Surface) != SurfacePlatform && ReleaseSurface(*in.Surface) != SurfaceMobile {
			return p, errpkg.ErrInvalid
		}
		p.Surface = *in.Surface
	}
	if in.Version != nil {
		p.Version = strings.TrimSpace(*in.Version)
	}
	if p.Version == "" {
		return p, errpkg.ErrInvalid
	}
	if p.Surface == string(SurfaceMobile) && !mobileSemverRe.MatchString(p.Version) {
		return p, ErrInvalidReleaseVersion
	}
	if in.Slug != nil && *in.Slug != "" {
		p.Slug = content.Slugify(*in.Slug)
	} else if create || p.Slug == "" {
		slug, err := s.uniqueReleaseSlug(ctx, content.Slugify(p.Version))
		if err != nil {
			return p, err
		}
		p.Slug = slug
	}
	if reservedReleaseSlugs[p.Slug] {
		return p, errpkg.ErrInvalid
	}
	if in.Title != nil {
		p.Title = strPtrOrNil(*in.Title)
	}
	if in.Summary != nil {
		p.Summary = strPtrOrNil(*in.Summary)
	}
	if in.Body != nil {
		p.Body = strPtrOrNil(*in.Body)
	}
	if in.IsESR != nil {
		p.IsEsr = *in.IsESR
	}
	var err error
	if in.ReleasedAt != nil {
		p.ReleasedAt, err = parseOptionalTime(*in.ReleasedAt)
		if err != nil {
			return p, errpkg.ErrInvalid
		}
	}
	if in.EOLAt != nil {
		p.EolAt, err = parseOptionalTime(*in.EOLAt)
		if err != nil {
			return p, errpkg.ErrInvalid
		}
	}
	if in.SortOrder != nil {
		p.SortOrder = *in.SortOrder
	}
	return p, nil
}

// AdminSetReleaseStatus drives the lifecycle; on → published with
// absorbUnreleased the public-type unreleased queue sweeps into the
// release inside one tx — internal-only types stay queued for triage.
func (s *Service) AdminSetReleaseStatus(ctx context.Context, id pgtype.UUID, to ReleaseStatus, absorbUnreleased bool) (gen.RoadmapRelease, error) {
	current, err := s.AdminGetRelease(ctx, id)
	if err != nil {
		return gen.RoadmapRelease{}, err
	}
	if current.DeletedAt.Valid {
		return gen.RoadmapRelease{}, fmt.Errorf("%w: release is deleted", ErrNotFound)
	}
	if !IsValidReleaseTransition(ReleaseStatus(current.Status), to) {
		return gen.RoadmapRelease{}, ErrInvalidTransition
	}
	row, err := db.WithTxResult(ctx, s.db.Writer, func(q *gen.Queries) (gen.RoadmapRelease, error) {
		r, err := q.SetRoadmapReleaseStatus(ctx, gen.SetRoadmapReleaseStatusParams{
			Status: string(to), ID: id,
		})
		if err != nil {
			return gen.RoadmapRelease{}, err
		}
		if to == ReleasePublished && absorbUnreleased {
			if _, err := q.AssignUnreleasedEntries(ctx, id); err != nil {
				return gen.RoadmapRelease{}, err
			}
		}
		return r, nil
	})
	if err != nil {
		return gen.RoadmapRelease{}, fmt.Errorf("setting release status: %w", err)
	}
	s.invalidateReleases(ctx)
	return row, nil
}

// AdminDeleteRelease soft-deletes; force=true hard-deletes an
// already-soft-deleted row (Rule 11 — the handler gates admin.hard_delete).
func (s *Service) AdminDeleteRelease(ctx context.Context, id pgtype.UUID, force bool) error {
	r, err := s.AdminGetRelease(ctx, id)
	if err != nil {
		return err
	}
	if force {
		if !r.DeletedAt.Valid {
			return errpkg.ErrInvalid
		}
		if err := s.store.write.HardDeleteRoadmapRelease(ctx, id); err != nil {
			return fmt.Errorf("hard-deleting release: %w", err)
		}
	} else {
		if r.DeletedAt.Valid {
			return fmt.Errorf("%w: release is already deleted", ErrNotFound)
		}
		if err := s.store.write.SoftDeleteRoadmapRelease(ctx, id); err != nil {
			return fmt.Errorf("soft-deleting release: %w", err)
		}
	}
	s.invalidateReleases(ctx)
	return nil
}

// AdminRestoreRelease clears deleted_at.
func (s *Service) AdminRestoreRelease(ctx context.Context, id pgtype.UUID) error {
	r, err := s.AdminGetRelease(ctx, id)
	if err != nil {
		return err
	}
	if !r.DeletedAt.Valid {
		return errpkg.ErrNotDeleted
	}
	if err := s.store.write.RestoreRoadmapRelease(ctx, id); err != nil {
		return fmt.Errorf("restoring release: %w", err)
	}
	s.invalidateReleases(ctx)
	return nil
}

// ReplaceReleaseItems replaces a release's shipped-item set —
// shipped-only; any non-shipped/missing id → ErrItemNotShipped.
func (s *Service) ReplaceReleaseItems(ctx context.Context, releaseID pgtype.UUID, itemIDs []pgtype.UUID) error {
	rel, err := s.AdminGetRelease(ctx, releaseID)
	if err != nil {
		return err
	}
	if rel.DeletedAt.Valid {
		return fmt.Errorf("%w: release is deleted", ErrNotFound)
	}
	if err := db.WithTx(ctx, s.db.Writer, func(q *gen.Queries) error {
		if err := q.ClearRoadmapReleaseItems(ctx, releaseID); err != nil {
			return err
		}
		for _, itemID := range itemIDs {
			n, err := q.SetRoadmapItemRelease(ctx, gen.SetRoadmapItemReleaseParams{
				ReleaseID: releaseID, ItemID: itemID,
			})
			if err != nil {
				return err
			}
			if n == 0 {
				return ErrItemNotShipped
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("replacing release items: %w", err)
	}
	s.invalidateReleases(ctx)
	return nil
}

// AttachItemsByWindow resolves a shipped_at window into item IDs then
// replaces the attachment set (agent tool window mode).
func (s *Service) AttachItemsByWindow(ctx context.Context, releaseID pgtype.UUID, since, until *time.Time) error {
	var sinceTS, untilTS pgtype.Timestamptz
	if since != nil {
		sinceTS = pgtype.Timestamptz{Time: *since, Valid: true}
	}
	if until != nil {
		untilTS = pgtype.Timestamptz{Time: *until, Valid: true}
	}
	ids, err := s.store.read.ListShippedItemIDsInWindow(ctx, gen.ListShippedItemIDsInWindowParams{
		Since: sinceTS, Until: untilTS,
	})
	if err != nil {
		return fmt.Errorf("listing shipped items in window: %w", err)
	}
	return s.ReplaceReleaseItems(ctx, releaseID, ids)
}

// ---------------------------------------------------------------------------
// Entries — ingest + admin queue
// ---------------------------------------------------------------------------

// IngestCommits parses each commit, resolves scope, and inserts a deduped
// entry into the unreleased queue. Returns created/skipped counts.
func (s *Service) IngestCommits(ctx context.Context, commits []GitHubCommit) (created, skipped int, err error) {
	// Preload the flag-key set once — ResolveScope would otherwise run up
	// to three feature_flags lookups per commit (~300 queries per batch).
	var flagKeys map[string]bool
	if flags, ferr := s.store.read.ListFeatureFlags(ctx); ferr == nil {
		flagKeys = make(map[string]bool, len(flags))
		for _, f := range flags {
			flagKeys[f.Key] = true
		}
	}

	for _, c := range commits {
		subject := c.Message
		if idx := strings.Index(subject, "\n"); idx >= 0 {
			subject = subject[:idx]
		}
		commitType, scope, title, breaking := ParseConventionalCommit(subject)
		if strings.Contains(c.Message, "BREAKING CHANGE") || strings.Contains(c.Message, "BREAKING-CHANGE") {
			breaking = true
		}
		if title == "" {
			skipped++
			continue
		}
		files := append(append(append([]string{}, c.Added...), c.Modified...), c.Removed...)
		resolved := s.ResolveScope(ctx, scope, files, flagKeys)

		var sha *string
		if c.ID != "" {
			sha = &c.ID
		}
		var committedAt pgtype.Timestamptz
		if t, perr := time.Parse(time.RFC3339, c.Timestamp); perr == nil {
			committedAt = pgtype.Timestamptz{Time: t, Valid: true}
		}
		n, ierr := s.store.write.InsertReleaseEntry(ctx, gen.InsertReleaseEntryParams{
			CommitSha: sha, CommitUrl: strPtrOrNil(c.URL), CommitType: commitType,
			Scope: strPtrOrNil(resolved), IsBreaking: breaking, Title: title,
			AuthorName: strPtrOrNil(c.Author.Name), CommittedAt: committedAt,
		})
		if ierr != nil {
			return created, skipped, fmt.Errorf("inserting release entry: %w", ierr)
		}
		if n == 0 {
			skipped++
		} else {
			created++
		}
	}
	if created > 0 {
		s.invalidateReleases(ctx)
	}
	return created, skipped, nil
}

// AdminListEntries returns the paginated entries queue.
func (s *Service) AdminListEntries(ctx context.Context, f EntryFilter) (EntryListResponse, error) {
	pageSize := f.PageSize
	if pageSize <= 0 || pageSize > MaxPageSize {
		pageSize = DefaultPageSize
	}
	rows, err := s.store.read.ListAdminReleaseEntries(ctx, gen.ListAdminReleaseEntriesParams{
		Unreleased: f.Unreleased, CommitType: f.CommitType, Scope: f.Scope,
		PageSize: pageSize, PageOffset: f.PageOffset,
	})
	if err != nil {
		return EntryListResponse{}, fmt.Errorf("listing entries: %w", err)
	}
	total, err := s.store.read.CountAdminReleaseEntries(ctx, gen.CountAdminReleaseEntriesParams{
		Unreleased: f.Unreleased, CommitType: f.CommitType, Scope: f.Scope,
	})
	if err != nil {
		return EntryListResponse{}, fmt.Errorf("counting entries: %w", err)
	}
	page := f.PageOffset/pageSize + 1
	totalPages := int32((total + int64(pageSize) - 1) / int64(pageSize))
	return EntryListResponse{Entries: rows, Total: total, Page: page, TotalPages: totalPages}, nil
}

// EntryUpdate carries the admin-editable entry fields.
type EntryUpdate struct {
	CommitType *string
	Title      *string
	Scope      *string
	ItemID     *string
}

// AdminUpdateEntry retypes/retitles/relinks an entry (triage path for
// promoting an 'other' commit to a public type).
func (s *Service) AdminUpdateEntry(ctx context.Context, id pgtype.UUID, in EntryUpdate) (gen.RoadmapReleaseEntry, error) {
	cur, err := s.store.read.GetReleaseEntryByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.RoadmapReleaseEntry{}, ErrNotFound
		}
		return gen.RoadmapReleaseEntry{}, fmt.Errorf("getting entry: %w", err)
	}
	commitType, title, scope, itemID := cur.CommitType, cur.Title, cur.Scope, cur.ItemID
	if in.CommitType != nil {
		if !allCommitTypes[*in.CommitType] {
			return gen.RoadmapReleaseEntry{}, ErrInvalidCommitType
		}
		commitType = *in.CommitType
	}
	if in.Title != nil {
		if strings.TrimSpace(*in.Title) == "" {
			return gen.RoadmapReleaseEntry{}, errpkg.ErrInvalid
		}
		title = strings.TrimSpace(*in.Title)
	}
	if in.Scope != nil {
		scope = strPtrOrNil(*in.Scope)
	}
	if in.ItemID != nil {
		parsed, perr := parseUUID(*in.ItemID)
		if perr != nil {
			return gen.RoadmapReleaseEntry{}, errpkg.ErrInvalid
		}
		itemID = parsed
	}
	row, err := s.store.write.UpdateReleaseEntry(ctx, gen.UpdateReleaseEntryParams{
		CommitType: commitType, Title: title, Scope: scope, ItemID: itemID, ID: id,
	})
	if err != nil {
		return gen.RoadmapReleaseEntry{}, mapWriteErr(err)
	}
	s.invalidateReleases(ctx)
	return row, nil
}

// AdminDeleteEntry hard-deletes an entry (operational data, not user
// content — Rule 11 soft-delete does not apply).
func (s *Service) AdminDeleteEntry(ctx context.Context, id pgtype.UUID) error {
	if _, err := s.store.read.GetReleaseEntryByID(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("getting entry: %w", err)
	}
	if err := s.store.write.DeleteReleaseEntry(ctx, id); err != nil {
		return fmt.Errorf("deleting entry: %w", err)
	}
	s.invalidateReleases(ctx)
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// uniqueReleaseSlug finds an unused roadmap_releases slug by appending
// -2, -3, … — the dedupe check sees soft-deleted rows too since the
// UNIQUE constraint covers them.
func (s *Service) uniqueReleaseSlug(ctx context.Context, base string) (string, error) {
	if base == "" {
		base = "release"
	}
	candidate := base
	for i := 2; ; i++ {
		_, err := s.store.read.GetRoadmapReleaseIDBySlug(ctx, candidate)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return candidate, nil
			}
			return "", err
		}
		if i > content.MaxSlugAttempts {
			return "", fmt.Errorf("%w: could not find unique slug for %q", errpkg.ErrInvalid, base)
		}
		candidate = fmt.Sprintf("%s-%d", base, i)
	}
}

func releaseDTO(r gen.RoadmapRelease, supersededAt *time.Time, now time.Time) ReleaseDTO {
	return ReleaseDTO{
		ID: r.ID.String(), Surface: r.Surface, Version: r.Version,
		Slug: r.Slug, Title: r.Title, Summary: r.Summary, Status: r.Status,
		SupportState: ComputeSupportState(r, supersededAt, now),
		IsESR:        r.IsEsr,
		ReleasedAt:   tsToPtr(r.ReleasedAt), EOLAt: tsToPtr(r.EolAt),
		SupersededAt: supersededAt,
	}
}

func entryDTO(e gen.RoadmapReleaseEntry) EntryDTO {
	d := EntryDTO{
		ID: e.ID.String(), CommitType: e.CommitType, Scope: e.Scope,
		IsBreaking: e.IsBreaking, Title: e.Title, CommitSHA: e.CommitSha,
		CommitURL: e.CommitUrl, AuthorName: e.AuthorName,
		CommittedAt: tsToPtr(e.CommittedAt),
	}
	if e.ItemID.Valid {
		s := e.ItemID.String()
		d.ItemID = &s
	}
	if e.ReleaseID.Valid {
		s := e.ReleaseID.String()
		d.ReleaseID = &s
	}
	return d
}

// supersededToTime normalizes the LEAD() window result — sqlc emits
// interface{} for window functions; nil/invalid become nil.
func supersededToTime(v interface{}) pgtype.Timestamptz {
	if t, ok := v.(time.Time); ok {
		return pgtype.Timestamptz{Time: t, Valid: true}
	}
	return pgtype.Timestamptz{}
}

func tsToPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

func parseUUID(s string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if s == "" {
		return id, nil
	}
	if err := id.Scan(s); err != nil {
		return id, err
	}
	return id, nil
}

// parseOptionalTime accepts RFC 3339 plus the timezone-less shapes
// HTML datetime-local/date inputs produce ("2006-01-02T15:04",
// "2006-01-02") — treated as UTC.
func parseOptionalTime(s string) (pgtype.Timestamptz, error) {
	var out pgtype.Timestamptz
	if s == "" {
		return out, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return pgtype.Timestamptz{Time: t, Valid: true}, nil
		}
	}
	return out, fmt.Errorf("unparseable timestamp %q", s)
}

