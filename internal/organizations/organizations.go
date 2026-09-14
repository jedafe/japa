// Package organizations implements the Organization Profiles system for
// JapaConnect (Section 07).
// // Organizations represent structured entities — businesses, NGOs, government
// bodies, embassies, universities, media, or communities — that users own
// and manage. Members can be invited with specific roles, and unclaimed
// listings can be claimed via the ownership claim flow.
package organizations

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/worker"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// Organization status values.
const (
	StatusActive              = "active"
	StatusSuspended           = "suspended"
	StatusPendingVerification = "pending_verification"
)

// Organization type values.
const (
	OrgTypeBusiness   = "business"
	OrgTypeNGO        = "ngo"
	OrgTypeGovernment = "government"
	OrgTypeEmbassy    = "embassy"
	OrgTypeUniversity = "university"
	OrgTypeMedia      = "media"
	OrgTypeCommunity  = "community"
)

// Member role values.
const (
	MemberRoleOwner  = "owner"
	MemberRoleAdmin  = "admin"
	MemberRoleEditor = "editor"
	MemberRoleMember = "member"
)

// Invitation role values.
const (
	InvitationRoleAdmin  = "admin"
	InvitationRoleEditor = "editor"
	InvitationRoleMember = "member"
)

// Ownership claim status values.
const (
	ClaimStatusPending   = "pending"
	ClaimStatusApproved  = "approved"
	ClaimStatusRejected  = "rejected"
	ClaimStatusWithdrawn = "withdrawn"
)

// Pagination defaults.
const (
	DefaultPageSize int32 = 20
	MaxPageSize     int32 = 100
)

// Cache constants for org public profile/list caching (valkey-keys-catalog.md).
const (
	orgProfileCacheTTL = 60 * time.Second
	orgListCacheTTL    = 60 * time.Second
)

// TierResolver resolves a user's effective unified tier (regular/pro/business).
type TierResolver interface {
	ResolveUserTier(ctx context.Context, userID pgtype.UUID) (string, error)
}

// SettingsReader is the subset of config.Settings used by the org service.
type SettingsReader interface {
	GetSettingInt(ctx context.Context, key string) (int, error)
	GetSettingBool(ctx context.Context, key string) (bool, error)
}

// webhookEmitter fans an organization.* / claim.* event out to all subscribed
// webhook endpoints for the tenant (fire-and-forget). Ref: Section 12 (webhooks).
type webhookEmitter interface {
	EmitEvent(ctx context.Context, eventType string, payload any)
}

// sourceManager creates canonical intel_sources rows for entity profiles
// (v2 source directory, Task 5).
type sourceManager interface {
	CreateSourceFromEntity(ctx context.Context, tenantID, entityID, userID pgtype.UUID, entityType, name, slug, sourceURL, category string) (gen.IntelSource, error)
}

// Service provides organization business logic.
type Service struct {
	store        *Store
	db           *db.DB
	settings     SettingsReader
	tierResolver TierResolver
	vk           valkeycompat.Cmdable
	webhooks     webhookEmitter
	asynq        *asynq.Client
	siteURL      string
	sources      sourceManager
}

// NewService creates a Service backed by the given database.
func NewService(database *db.DB) *Service {
	return &Service{
		store: NewStore(gen.New(database.Reader), gen.New(database.Writer), database.Writer),
		db:    database,
	}
}

// NewServiceWithSettings creates a Service with a settings reader for
// system_settings-backed gates (e.g. organizations.max_per_user).
func NewServiceWithSettings(database *db.DB, settings SettingsReader) *Service {
	return &Service{
		store:    NewStore(gen.New(database.Reader), gen.New(database.Writer), database.Writer),
		db:       database,
		settings: settings,
	}
}

// SetValkeyCache injects a Valkey client for read-through caching of public
// org profile and list endpoints (jc:org:profile, jc:org:list per
// valkey-keys-catalog.md). nil disables caching (degraded mode).
func (s *Service) SetValkeyCache(vk valkeycompat.Cmdable) {
	s.vk = vk
}

// SetWebhookEmitter injects the organization.* / claim.* event fan-out
// (Section 12).
func (s *Service) SetWebhookEmitter(w webhookEmitter) { s.webhooks = w }

// SetAsynqClient injects the Asynq client for enqueueing email:send tasks
// (BUG-018: invitation email notification). nil disables email delivery
// (degraded mode — the invitation is still created, just not emailed).
func (s *Service) SetAsynqClient(c *asynq.Client) { s.asynq = c }

// SetSiteURL injects the public site base URL (JC_SITE_URL) used to build
// absolute frontend links in invitation emails. Must never be derived from
// the request Host (CWE-601, same rule as auth email links).
func (s *Service) SetSiteURL(u string) { s.siteURL = strings.TrimRight(u, "/") }

// SetTierResolver injects the unified tier resolver used to read
// per-tier organization creation limits.
func (s *Service) SetTierResolver(r TierResolver) { s.tierResolver = r }

// SetSourceManager links the organization service to the canonical source
// registry (v2 source directory, Task 5).
func (s *Service) SetSourceManager(m sourceManager) { s.sources = m }

// emitWebhook fires an organization.* / claim.* event fire-and-forget
// (Rule 8). Errors are logged but never returned to the caller — webhook
// delivery is async.
func (s *Service) emitWebhook(ctx context.Context, eventType string, payload any) {
	if s.webhooks == nil {
		return
	}

	s.webhooks.EmitEvent(ctx, eventType, payload)
}

// enqueueInvitationEmail enqueues an email:send task to notify the invited
// user of their organization invitation (BUG-018). Fire-and-forget per Rule 8
// — errors are logged but never returned to the caller. The invitation is
// already persisted; email delivery is best-effort.
// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
// Ref: japaconnect-engineering/sections/20-notification-channels.md.
func (s *Service) enqueueInvitationEmail(ctx context.Context, orgName, inviteeEmail, role, token string) {
	if s.asynq == nil {
		return
	}

	// The accept link must point at the frontend accept page — not the API
	// route (which is POST-only). When no site URL is configured we still send
	// the email but with the relative path as a degraded fallback.
	acceptPath := "/org/invite/" + token
	acceptURL := acceptPath
	if s.siteURL != "" {
		acceptURL = s.siteURL + acceptPath
	}

	subject := fmt.Sprintf("You've been invited to join %s on JapaConnect", orgName)
	body := fmt.Sprintf(
		"You have been invited to join %s as a %s.\n\n"+
			"To accept this invitation, click the link below:\n"+
			"%s\n\n"+
			"If you did not expect this invitation, you can safely ignore this email.",
		orgName, role, acceptURL,
	)

	payload, err := json.Marshal(map[string]any{
		"to":      inviteeEmail,
		"subject": subject,
		"body":    body,
		"type":    "organization_invitation",
	})
	if err != nil {
		slog.Error("organizations: marshal invitation email payload", "error", err)
		return
	}

	task := asynq.NewTask(worker.TypeEmailSend, payload)
	if _, err := s.asynq.Enqueue(task, asynq.Queue(worker.QueueForTask(worker.TypeEmailSend))); err != nil {
		slog.Error("organizations: enqueue invitation email",
			"invitee_email", inviteeEmail, "error", err)
		return
	}

	slog.Info("organizations: invitation email enqueued",
		"invitee_email", inviteeEmail, "org_name", orgName)
}

// orgProfileCacheKey builds jc:org:profile:{}:{slug}.
func orgProfileCacheKey(tenantID pgtype.UUID, slug string) string {
	return valkey.MustKey("org:profile:%s:%s", tenantID.String(), slug)
}

// orgListCacheKey builds jc:org:list:{}:{page}.
func orgListCacheKey(tenantID pgtype.UUID, page int32) string {
	return valkey.MustKey("org:list:%s:%d", tenantID.String(), page)
}

// InvalidateOrgCache flushes all cached org entries for a tenant. Call after
// any org CRUD mutation (create, update, delete, verify, unverify).
func (s *Service) InvalidateOrgCache(ctx context.Context, tenantID pgtype.UUID) {
	if s.vk == nil {
		return
	}

	prefix := valkey.MustKey("org:*:%s:*", tenantID.String())
	// Also flush the list cache for this tenant.
	listPrefix := valkey.MustKey("org:list:%s:*", tenantID.String())

	for _, pattern := range []string{prefix, listPrefix} {
		var cursor uint64

		for {
			keys, nextCursor, err := s.vk.Scan(ctx, cursor, pattern, 100).Result()
			if err != nil {
				slog.Warn("org: failed to scan valkey for invalidation", "pattern", pattern, "error", err)
				break
			}

			if len(keys) > 0 {
				_ = s.vk.Del(ctx, keys...).Err()
			}

			cursor = nextCursor
			if cursor == 0 {
				break
			}
		}
	}
}

// InvalidateOrgCacheByCmdable is a standalone invalidation function for
// callers (e.g. admin handlers) that have a valkeycompat.Cmdable but not a
// reference to the org Service. It flushes all jc:org:* keys for the tenant.
func InvalidateOrgCacheByCmdable(ctx context.Context, vk valkeycompat.Cmdable, tenantID pgtype.UUID) {
	if vk == nil {
		return
	}

	for _, pattern := range []string{
		valkey.MustKey("org:profile:%s:*", tenantID.String()),
		valkey.MustKey("org:list:%s:*", tenantID.String()),
	} {
		var cursor uint64

		for {
			keys, nextCursor, err := vk.Scan(ctx, cursor, pattern, 100).Result()
			if err != nil {
				slog.Warn("org: failed to scan valkey for invalidation", "pattern", pattern, "error", err)
				break
			}

			if len(keys) > 0 {
				_ = vk.Del(ctx, keys...).Err()
			}

			cursor = nextCursor
			if cursor == 0 {
				break
			}
		}
	}
}

// cachedPublicOrg wraps a cached public org profile for JSON round-trip.
type cachedPublicOrg struct {
	Org  gen.Organization `json:"org"`
	Time time.Time        `json:"cached_at"`
}

// cachedPublicOrgList wraps a cached public org list response for JSON round-trip.
type cachedPublicOrgList struct {
	Rows  []gen.ListPublicOrganizationsRow `json:"rows"`
	Total int64                            `json:"total"`
	Time  time.Time                        `json:"cached_at"`
}

// getOrSetOrgProfile implements read-through cache for GetPublicOrganizationBySlug.
// On cache miss, fetches from the store and populates the cache. On cache hit,
// returns the cached org. Returns the org and a cache-hit flag for observability.
func (s *Service) getOrSetOrgProfile(ctx context.Context, tenantID pgtype.UUID, slug string) (gen.Organization, error) {
	if s.vk == nil {
		return s.store.GetPublicOrganizationBySlug(ctx, slug)
	}

	cacheKey := orgProfileCacheKey(tenantID, slug)
	if cached, err := s.vk.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
		var entry cachedPublicOrg
		if json.Unmarshal([]byte(cached), &entry) == nil {
			return entry.Org, nil
		}
	}

	org, err := s.store.GetPublicOrganizationBySlug(ctx, slug)
	if err != nil {
		return org, err
	}

	entry := cachedPublicOrg{Org: org, Time: time.Now()}
	if data, err := json.Marshal(entry); err == nil {
		if err := s.vk.Set(ctx, cacheKey, data, orgProfileCacheTTL).Err(); err != nil {
			slog.Warn("org: failed to cache profile", "slug", slug, "error", err)
		}
	}

	return org, nil
}

// getOrSetOrgList implements read-through cache for ListPublicOrganizations.
// The cache key includes the page number; filter variations (search, orgType,
// verified) are part of the key to avoid returning stale filtered results.
func (s *Service) getOrSetOrgList(ctx context.Context, tenantID pgtype.UUID, search, orgType, verified string, limit, offset int32) ([]gen.ListPublicOrganizationsRow, int64, error) {
	if s.vk == nil {
		return s.fetchOrgList(ctx, search, orgType, verified, limit, offset)
	}

	page := offset / max(limit, 1)
	cacheKey := orgListCacheKey(tenantID, page)
	// Include filter params in the key to avoid cross-filter cache pollution.
	cacheKey = fmt.Sprintf("%s:%s:%s:%s:%d:%d", cacheKey, search, orgType, verified, limit, offset)

	if cached, err := s.vk.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
		var entry cachedPublicOrgList
		if json.Unmarshal([]byte(cached), &entry) == nil {
			return entry.Rows, entry.Total, nil
		}
	}

	rows, total, err := s.fetchOrgList(ctx, search, orgType, verified, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	entry := cachedPublicOrgList{Rows: rows, Total: total, Time: time.Now()}
	if data, err := json.Marshal(entry); err == nil {
		if err := s.vk.Set(ctx, cacheKey, data, orgListCacheTTL).Err(); err != nil {
			slog.Warn("org: failed to cache list", "error", err)
		}
	}

	return rows, total, nil
}

// fetchOrgList is the uncached DB fetch for ListPublicOrganizations.
func (s *Service) fetchOrgList(ctx context.Context, search, orgType, verified string, limit, offset int32) ([]gen.ListPublicOrganizationsRow, int64, error) {
	rows, err := s.store.ListPublicOrganizations(ctx, search, orgType, verified, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	total, err := s.store.CountPublicOrganizations(ctx, search, orgType, verified)
	if err != nil {
		return nil, 0, err
	}

	return rows, total, nil
}
