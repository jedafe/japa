// Package spaces implements the Geo-First Space System for JapaConnect
// (Section 05).
//
// Spaces are the primary organizational unit for content. They form a
// hierarchy: country -> state -> city, with custom spaces for non-geo
// communities. Each space has five permission axes (visibility, posting,
// replying, commenting, joining) that control who can do what. Seven
// default permission presets are seeded and admin-editable.
//
// Space types (Country, City, University, Community) optionally link to
// custom field groups from Section 06 for type-specific fields.
package spaces

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/worker"
	valkeycompat "github.com/valkey-io/valkey-go/valkeycompat"
)

// Space hierarchy levels. Country is the root, state and city nest under
// it, custom covers non-geographic communities (universities, clubs),
// and global is for spaces not tied to any geo location at all.
const (
	LevelCountry = "country"
	LevelState   = "state"
	LevelCity    = "city"
	LevelCustom  = "custom"
	LevelGlobal  = "global"
)

// Visibility axis values. Public spaces are listed to everyone, members
// spaces only to logged-in members, and private spaces only to invited
// members.
const (
	VisibilityPublic  = "public"
	VisibilityMembers = "members"
	VisibilityPrivate = "private"
)

// Permission axis values shared by the posting, replying, and commenting
// axes. They describe the minimum role required to perform the action.
const (
	PermOpen       = "open"
	PermMembers    = "members"
	PermVerified   = "verified"
	PermModerators = "moderators"
	PermAdmins     = "admins"
)

// Joining axis values. Open lets anyone join immediately, request requires
// moderator approval, and invite means only invited users can join.
const (
	JoiningOpen    = "open"
	JoiningRequest = "request"
	JoiningInvite  = "invite"
)

// Space lifecycle status values.
const (
	StatusActive   = "active"
	StatusArchived = "archived"
	StatusDraft    = "draft"
)

// SpacePermAxes holds the five permission axes for a space. Each axis is
// a text value enforced by the permission-checking logic. A space moderator
// can override individual axes without changing the preset label.
type SpacePermAxes struct {
	Visibility string
	Posting    string
	Replying   string
	Commenting string
	Joining    string
}

// validLevels is the set of recognized space hierarchy levels.
var validLevels = map[string]bool{
	LevelCountry: true, LevelState: true, LevelCity: true, LevelCustom: true, LevelGlobal: true,
}

// IsValidLevel returns true if the given string is a recognized space level.
func IsValidLevel(level string) bool {
	return validLevels[level]
}

// validVisibility is the set of recognized visibility axis values.
var validVisibility = map[string]bool{
	VisibilityPublic: true, VisibilityMembers: true, VisibilityPrivate: true,
}

// validPostingPerm is the set of recognized posting, replying, and
// commenting axis values.
var validPostingPerm = map[string]bool{
	PermOpen: true, PermMembers: true, PermVerified: true,
	PermModerators: true, PermAdmins: true,
}

// validJoiningPerm is the set of recognized joining axis values.
var validJoiningPerm = map[string]bool{
	JoiningOpen: true, JoiningRequest: true, JoiningInvite: true,
}

// ValidPermissionAxes returns true if all five axis values are recognized.
// Posting, replying, and commenting share the same value set.
func ValidPermissionAxes(a SpacePermAxes) bool {
	return validVisibility[a.Visibility] &&
		validPostingPerm[a.Posting] &&
		validPostingPerm[a.Replying] &&
		validPostingPerm[a.Commenting] &&
		validJoiningPerm[a.Joining]
}

// DefaultPermAxes returns the default permission axes applied to a new
// space when no preset is specified: public visibility, members-only
// posting/replying/commenting, and open joining.
func DefaultPermAxes() SpacePermAxes {
	return SpacePermAxes{
		Visibility: VisibilityPublic,
		Posting:    PermMembers,
		Replying:   PermMembers,
		Commenting: PermMembers,
		Joining:    JoiningOpen,
	}
}

// Service provides space operations with reader/writer pool separation.
// Reads go through the reader pool; writes go through the writer pool.
// This follows the same pattern as the existing db.DB struct.
type Service struct {
	read     *gen.Queries
	write    *gen.Queries
	db       *db.DB
	enqueuer spaceEnqueuer
	vk       valkeycompat.Cmdable // nil disables caching (tests/development)
}

// SetValkey wires the Valkey client for mute SET cache invalidation
// (Section 52 / VALKEY-001, BUG-003: tenant-scoped). Key:
// jc:muted_spaces:{tenant_id}:{user_id}.
func (s *Service) SetValkey(vk valkeycompat.Cmdable) { s.vk = vk }

// invalidateMutedSpacesCache deletes the tenant-scoped muted-spaces SET cache
// for the user. Called after mute/unmute so the next read refetches from the
// DB.
func (s *Service) invalidateMutedSpacesCache(ctx context.Context, tenantID, userID pgtype.UUID) {
	if s.vk == nil {
		return
	}

	key := valkey.MustKey("muted_spaces:%s:%s", tenantID.String(), userID.String())
	if err := s.vk.Del(ctx, key).Err(); err != nil {
		slog.Warn("invalidate muted_spaces cache", "error", err)
	}
}

// spaceEnqueuer is the subset of worker.Enqueuer that spaces needs.
type spaceEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// SetEnqueuer injects the Asynq enqueuer for async space tasks.
func (s *Service) SetEnqueuer(e spaceEnqueuer) {
	s.enqueuer = e
}

// EnqueueAutoPopulate enqueues the space:auto_populate task if an enqueuer
// is wired. Best-effort — logs errors but never fails the caller.
func (s *Service) EnqueueAutoPopulate(ctx context.Context, spaceID, tenantID pgtype.UUID) {
	if s.enqueuer == nil {
		return
	}

	payload, _ := json.Marshal(map[string]string{
		"space_id":  spaceID.String(),
		"tenant_id": tenantID.String(),
	})
	if err := s.enqueuer.Enqueue(worker.TypeSpaceAutoPopulate, payload); err != nil {
		slog.Warn("spaces: failed to enqueue auto_populate", "space_id", spaceID.String(), "error", err)
	}
}

// EnqueueSearchIndex enqueues a search:index_entity task to sync the space to
// the Meilisearch index (BL-004). operation is "index" (create/update) or
// "delete". Best-effort — logs errors but never fails the caller.
func (s *Service) EnqueueSearchIndex(ctx context.Context, spaceID, tenantID pgtype.UUID, operation string) {
	if s.enqueuer == nil {
		return
	}

	payload, _ := json.Marshal(map[string]string{
		"entity_id":   spaceID.String(),
		"entity_type": "space",
		"tenant_id":   tenantID.String(),
		"operation":   operation,
	})
	if err := s.enqueuer.Enqueue(worker.TypeSearchIndexSync, payload); err != nil {
		slog.Warn("spaces: failed to enqueue search:index_entity",
			"space_id", spaceID.String(), "operation", operation, "error", err)
	}
}

var spaceKeywordStopWords = map[string]struct{}{
	"a": {}, "an": {}, "the": {}, "in": {}, "of": {}, "for": {}, "and": {}, "or": {},
	"community": {}, "global": {}, "world": {}, "university": {}, "college": {},
}

var spaceKeywordSeparators = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// extractSpaceKeywords tokenises a short space name into normalised n-grams
// (1-2 words) for keyword matching against primary geo spaces. Stop words
// are dropped only when they appear as single-word grams.
func extractSpaceKeywords(name string) []string {
	if name == "" {
		return nil
	}

	normalized := spaceKeywordSeparators.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), " ")
	raw := strings.Fields(normalized)
	words := make([]string, 0, len(raw))

	for _, w := range raw {
		if len(w) >= 2 {
			words = append(words, w)
		}
	}

	if len(words) == 0 {
		return nil
	}

	seen := make(map[string]struct{})
	out := make([]string, 0, len(words)*2)

	maxN := 2
	if len(words) < maxN {
		maxN = len(words)
	}

	for n := 1; n <= maxN; n++ {
		for i := 0; i+n <= len(words); i++ {
			phrase := strings.Join(words[i:i+n], " ")
			if n == 1 {
				if _, ok := spaceKeywordStopWords[phrase]; ok {
					continue
				}
			}

			if _, ok := seen[phrase]; ok {
				continue
			}

			seen[phrase] = struct{}{}

			out = append(out, phrase)
		}
	}

	return out
}

// FindSpacesByKeywords finds public primary (country/state/city) and public
// education spaces whose configured keywords, full name, or university
// initials overlap with the given keyword list. Used by the content
// auto-assign feature to link content to spaces based on title matching.
// Ref: sections/05-geo-first-space-system.md, sections/14-content-type-system.md.
func (s *Service) FindSpacesByKeywords(ctx context.Context, tenantID pgtype.UUID, keywords []string) ([]gen.FindSpacesByKeywordsRow, error) {
	return s.read.FindSpacesByKeywords(ctx, gen.FindSpacesByKeywordsParams{
		TenantID: tenantID,
		Keywords: keywords,
	})
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
		db:    database,
	}
}
