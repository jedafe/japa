package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/achievement"
	"github.com/jedafe/japa/internal/admin"
	"github.com/jedafe/japa/internal/adsselfserve"
	"github.com/jedafe/japa/internal/advancedanalytics"
	"github.com/jedafe/japa/internal/advertising"
	"github.com/jedafe/japa/internal/affiliate"
	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/ama"
	"github.com/jedafe/japa/internal/analytics"
	"github.com/jedafe/japa/internal/appointmentalert"
	"github.com/jedafe/japa/internal/arrivalchecklist"
	"github.com/jedafe/japa/internal/assessment"
	"github.com/jedafe/japa/internal/attestationguide"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/blog"
	"github.com/jedafe/japa/internal/bookmarkfolders"
	"github.com/jedafe/japa/internal/bots"
	"github.com/jedafe/japa/internal/calendar"
	"github.com/jedafe/japa/internal/checklist"
	"github.com/jedafe/japa/internal/comments"
	"github.com/jedafe/japa/internal/communitylocator"
	"github.com/jedafe/japa/internal/comparison"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/contentimport"
	"github.com/jedafe/japa/internal/control"
	"github.com/jedafe/japa/internal/costofliving"
	"github.com/jedafe/japa/internal/coupons"
	"github.com/jedafe/japa/internal/credentialchecker"
	"github.com/jedafe/japa/internal/csrf"
	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/cvformatter"
	"github.com/jedafe/japa/internal/dataexport"
	"github.com/jedafe/japa/internal/dataimport"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/devices"
	"github.com/jedafe/japa/internal/digest"
	"github.com/jedafe/japa/internal/docs"
	"github.com/jedafe/japa/internal/email"
	emailtemplates "github.com/jedafe/japa/internal/email/templates"
	errtracking "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/features"
	"github.com/jedafe/japa/internal/feed"
	"github.com/jedafe/japa/internal/follows"
	"github.com/jedafe/japa/internal/forum"
	"github.com/jedafe/japa/internal/gamification"
	"github.com/jedafe/japa/internal/gdpr"
	"github.com/jedafe/japa/internal/geoip"
	"github.com/jedafe/japa/internal/honcho"
	"github.com/jedafe/japa/internal/housingguide"
	"github.com/jedafe/japa/internal/i18n"
	"github.com/jedafe/japa/internal/impersonation"
	"github.com/jedafe/japa/internal/intelligence/dedup"
	"github.com/jedafe/japa/internal/intelligence/events"
	"github.com/jedafe/japa/internal/intelligence/extractor"
	"github.com/jedafe/japa/internal/intelligence/extractor/schemas"
	intel_fetcher "github.com/jedafe/japa/internal/intelligence/fetcher"
	intel_handlers "github.com/jedafe/japa/internal/intelligence/handlers"
	"github.com/jedafe/japa/internal/intelligence/promoter"
	"github.com/jedafe/japa/internal/intelligence/scheduler"
	"github.com/jedafe/japa/internal/intelligence/sources"
	intel_stores "github.com/jedafe/japa/internal/intelligence/stores"
	"github.com/jedafe/japa/internal/interactions"
	"github.com/jedafe/japa/internal/japastage"
	"github.com/jedafe/japa/internal/jobs"
	"github.com/jedafe/japa/internal/languagetesthub"
	"github.com/jedafe/japa/internal/learning"
	"github.com/jedafe/japa/internal/linkpreview"
	"github.com/jedafe/japa/internal/media"
	"github.com/jedafe/japa/internal/mentions"
	"github.com/jedafe/japa/internal/mentorship"
	"github.com/jedafe/japa/internal/menus"
	mw "github.com/jedafe/japa/internal/middleware"
	"github.com/jedafe/japa/internal/mutualaid"
	"github.com/jedafe/japa/internal/newsletter"
	"github.com/jedafe/japa/internal/notifications"
	"github.com/jedafe/japa/internal/onboarding"
	"github.com/jedafe/japa/internal/organizations"
	"github.com/jedafe/japa/internal/pagebuilder"
	"github.com/jedafe/japa/internal/participation"
	"github.com/jedafe/japa/internal/payments"
	"github.com/jedafe/japa/internal/personas"
	"github.com/jedafe/japa/internal/planninggroups"
	"github.com/jedafe/japa/internal/professional"
	"github.com/jedafe/japa/internal/publicapi"
	"github.com/jedafe/japa/internal/rag"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/jedafe/japa/internal/recommendations"
	"github.com/jedafe/japa/internal/redirects"
	"github.com/jedafe/japa/internal/referral"
	"github.com/jedafe/japa/internal/remittance"
	"github.com/jedafe/japa/internal/renderer"
	"github.com/jedafe/japa/internal/reports"
	"github.com/jedafe/japa/internal/reviews"
	rsspkg "github.com/jedafe/japa/internal/rss"
	"github.com/jedafe/japa/internal/scamalert"
	"github.com/jedafe/japa/internal/scholarships"
	"github.com/jedafe/japa/internal/search"
	"github.com/jedafe/japa/internal/seo"
	"github.com/jedafe/japa/internal/servicedir"
	"github.com/jedafe/japa/internal/sidebars"
	"github.com/jedafe/japa/internal/spaces"
	"github.com/jedafe/japa/internal/spam"
	"github.com/jedafe/japa/internal/stock"
	"github.com/jedafe/japa/internal/story"
	"github.com/jedafe/japa/internal/studio"
	"github.com/jedafe/japa/internal/tags"
	"github.com/jedafe/japa/internal/taxcalculator"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/tickets"
	"github.com/jedafe/japa/internal/travel"
	"github.com/jedafe/japa/internal/trending"
	"github.com/jedafe/japa/internal/universities"
	"github.com/jedafe/japa/internal/university"
	"github.com/jedafe/japa/internal/users"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/visaroutes"
	"github.com/jedafe/japa/internal/visatimeline"
	"github.com/jedafe/japa/internal/visitor"
	"github.com/jedafe/japa/internal/wallet"
	"github.com/jedafe/japa/internal/webhooks"
	"github.com/jedafe/japa/internal/websocket"
	"github.com/jedafe/japa/internal/widgets"
	"github.com/jedafe/japa/internal/wiki"
	"github.com/jedafe/japa/internal/worker"
	"github.com/jedafe/japa/internal/workerv2"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// closers holds cleanup functions registered during NewServer that must be
// invoked during application shutdown to release background goroutines
// (e.g. the settings cache cleanup loop). Shutdown() drains them once.
var closers []func()

// Shutdown invokes all registered cleanup functions. Call this during
// application shutdown after the HTTP server has stopped accepting requests.
// It is safe to call multiple times.
var shutdownOnce sync.Once

func Shutdown() {
	shutdownOnce.Do(func() {
		for _, fn := range closers {
			fn()
		}
	})
}

// NewServer creates a fully configured Chi router with middleware and routes.
func NewServer(database *db.DB, cfg *config.Config) *chi.Mux {
	// Initialize error tracking (Section 27 — GlitchTip).
	errtracking.InitGlitchTip(cfg.GlitchtipDSN, cfg.ReleaseSHA)

	r := chi.NewRouter()
	RegisterMiddleware(r)

	// Open GeoIP database (graceful if missing - server still works).
	// The middleware runs on every request so downstream handlers can
	// read geo context via geoip.FromContext. Wired from day one to
	// avoid retrofitting every handler later.
	geoipReader, err := geoip.NewReader(cfg.GeoIPDBPath)
	if err != nil {
		slog.Warn("geoip database not loaded", "error", err)
	}

	r.Use(geoip.Middleware(geoipReader))

	// GeoIP updater for the maintenance: update_geoip_db task.
	// Created here and registered with the worker so the Asynq handler
	// can atomically swap in a fresh MaxMind database weekly.
	geoipUpdater := geoip.NewUpdater(cfg.MaxmindAccountID, cfg.MaxmindLicenseKey, cfg.GeoIPDBPath, geoipReader)
	worker.RegisterGeoIPUpdater(geoipUpdater)

	// Register site name for worker handlers (RSS feeds, LLMS.txt, emails).
	worker.RegisterSiteName(cfg.SiteName)

	// IP block middleware — reject blocked IPs before they reach any handler.
	// Resolves the client IP internally via clientip.FromRequest (trusted-proxy
	// model, JEOS Section 37) and runs before all other middleware and route
	// handlers.
	r.Use(spam.IPBlockMiddleware(database))

	h := NewHandler(database)
	tenantStore := tenant.NewDBStore(database)

	// Resolve the default tenant ID once for boot-time system_settings reads
	// that happen outside a request context (where tenant middleware would
	// populate it). All system_settings are tenant-scoped; using a zero UUID
	// would miss every row. Falls back to the well-known default tenant UUID
	// if the lookup fails so the server still boots on a fresh database.
	defaultTenantID := pgtype.UUID{Bytes: [16]byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
	}, Valid: true}
	// defaultTenant is resolved once at boot and reused by SingleTenantMiddleware
	// for every request, eliminating the per-request GetTenantByHost DB lookup
	// (ADR-004 amendment — single-tenant freeze).
	defaultTenant := &tenant.Tenant{ID: defaultTenantID}
	if dt, err := tenantStore.GetDefaultTenant(context.Background()); err == nil && dt != nil {
		defaultTenantID = dt.ID
		defaultTenant = dt
	}

	// JWT key rotation — keys are persisted in the jwt_signing_keys table
	// (§02) so they survive server restarts. On first boot (empty table),
	// the key is seeded from JC_JWT_SECRET (or a random dev key) and
	// persisted. On subsequent boots, keys are loaded from the DB.
	var signingKey []byte
	if cfg.JWTSecret != "" {
		signingKey = []byte(cfg.JWTSecret)
	} else {
		signingKey = make([]byte, 32)
		if _, err := rand.Read(signingKey); err != nil {
			slog.Error("generating random JWT key", "error", err)
		}

		slog.Warn("using random JWT key — sessions will not persist across restarts", "key_id", cfg.JWTKeyID)
	}

	kr := auth.NewKeyRotation(cfg.JWTKeyID, signingKey)

	// Load persisted signing keys from the database. If the table has
	// active keys, they replace the env-derived key. If the table is empty
	// (first boot), the env-derived key is persisted for future restarts.
	jwtKeyStore := auth.NewJWTKeyStore(database)
	if err := kr.LoadFromDB(context.Background(), jwtKeyStore); err != nil {
		slog.Error("loading JWT signing keys from DB, using env key", "error", err)
	} else if len(kr.Keys) > 0 {
		slog.Info("loaded JWT signing keys from DB", "count", len(kr.Keys), "current_kid", kr.CurrentKeyID)
	} else {
		slog.Info("no JWT signing keys in DB, persisting env key", "kid", cfg.JWTKeyID)
		if err := kr.PersistKey(context.Background(), jwtKeyStore, cfg.JWTKeyID, signingKey); err != nil {
			slog.Error("persisting JWT signing key to DB", "error", err)
		}
	}

	// Valkey client for cache, rate limiting, edit locks, brute-force
	// tracking, and space unread counters. Created with graceful fallback
	// so the server boots even when Valkey is unreachable. Password, DB
	// number, pool size, timeouts, and TLS are configurable via JC_ env
	// vars (Rule 3) so the client can authenticate to a protected Valkey
	// instance in production.
	vkClient := valkey.NewWithFallbackConfig(valkey.ClientConfig{
		Addr:          cfg.RedisAddr,
		Password:      cfg.RedisPassword,
		DB:            cfg.RedisDB,
		PoolSize:      cfg.RedisPoolSize,
		DialTimeoutMs: cfg.RedisDialTimeoutMs,
		ReadTimeoutMs: cfg.RedisReadTimeoutMs,
		TLSEnabled:    cfg.RedisTLSEnabled,
	})
	slog.Info("valkey client created", "addr", cfg.RedisAddr)

	// Spam pipeline — multi-layer content safety checks (IP reputation,
	// rate limiting, content hash, censored words, link analysis, user
	// age, content length). Used by content creation and DM handlers.
	spamStore := spam.NewStore(database)
	spamPipeline := spam.NewPipeline(spamStore, vkClient, database)

	// Auth handlers and DB store
	authStore := auth.NewDBStore(database)
	bruteForce := auth.NewValkeyBruteForceStore(vkClient.Redis(), auth.DefaultFailWindow)
	authHandlers := &auth.AuthHandlers{
		DB:            authStore,
		Queries:       authStore.Queries(),
		Database:      database,
		KeyRotation:   kr,
		BruteForce:    bruteForce,
		Policy:        auth.DefaultPasswordPolicy(),
		VKClient:      vkClient,
		SiteName:      cfg.SiteName,
		SiteURL:       cfg.SiteURL,
		SupportURL:    cfg.SiteURL + "/support",
		CookieDomain:  cfg.CookieDomain,
		SecureCookies: cfg.IsProduction(),
	}

	// TOTP 2FA handlers — uses the same auth store for TOTP credential storage.
	totpStore := auth.NewTOTPStore(database)
	totpHandlers := &auth.TOTPHandlers{Store: totpStore}

	// OAuth social login handlers — provider config from environment.
	// Read Apple private key content from file if configured.
	var applePrivateKeyPEM string

	if cfg.OAuthApplePrivateKeyPath != "" {
		data, err := os.ReadFile(cfg.OAuthApplePrivateKeyPath)
		if err != nil {
			slog.Warn("failed to read Apple private key file", "path", cfg.OAuthApplePrivateKeyPath, "error", err)
		} else {
			applePrivateKeyPEM = string(data)
		}
	}

	oauthCfg := auth.OAuthConfig{
		GoogleClientID:     cfg.OAuthGoogleID,
		GoogleClientSecret: cfg.OAuthGoogleSecret,
		GitHubClientID:     cfg.OAuthGitHubID,
		GitHubClientSecret: cfg.OAuthGitHubSecret,
		AppleClientID:      cfg.OAuthAppleClientID,
		AppleTeamID:        cfg.OAuthAppleTeamID,
		AppleKeyID:         cfg.OAuthAppleKeyID,
		ApplePrivateKeyPEM: applePrivateKeyPEM,
		FacebookAppID:      cfg.OAuthFacebookAppID,
		FacebookAppSecret:  cfg.OAuthFacebookAppSecret,
		RedirectBase:       cfg.OAuthRedirectBase,
	}
	oauthStore := auth.NewOAuthStore(database)
	authQueries := gen.New(database.Writer)
	oauthHandlers := &auth.OAuthHandlers{
		Config:          oauthCfg,
		Store:           oauthStore,
		Queries:         authQueries,
		KeyRot:          kr,
		FrontendBaseURL: cfg.SiteURL,
		CookieDomain:    cfg.CookieDomain,
		SecureCookies:   cfg.IsProduction(),
	}

	// Feature flag checker — uses Valkey L2 cache for cross-instance
	// consistency (Section 42: jc:flags:{tenant_id}:{flag_key}, TTL 5min).
	featureChecker := features.New(database, vkClient.Redis())
	closers = append(closers, featureChecker.Close)

	// RBAC resolver and store for permission checks at the router level.
	// The resolver uses Valkey as L2 cache (shared across server instances)
	// with in-memory L1 as the fastest path.
	rbacResolver := rbac.New(database, vkClient.Redis())
	rbacStore := rbac.NewStore(database)
	rbacStore.SetRBAC(rbacResolver)

	// RBAC admin management handlers.
	rbacHandlers := rbac.NewHandlers(rbacStore, rbacResolver)

	// Permission bootstrap is now handled by migration 000099_seed_data.sql.
	// The runtime bootstrap path was removed per AUDIT-BE-039 to avoid
	// maintaining two independent permission seeds.

	// Custom field and form subsystem (Section 06).
	cfService := customfields.NewService(database)
	cfService.SetCache(vkClient.Redis()) // BUG-016: cache field-level RBAC checks
	embedResolver := customfields.NewEmbedResolver()
	cfHandlers := customfields.NewHandlers(cfService, embedResolver)
	cfHandlers.SetRBAC(rbacResolver) // SEC-005: field-level RBAC enforcement

	// Content subsystem (Section 14 — Content Type System). Service created
	// early because spaces/blog handlers below need it for their
	// content-related sub-resources (space content/pinned, blog post CRUD).
	contentService := content.NewService(database)

	// Spaces subsystem (Section 05 — Geo-First Space System).
	spaceService := spaces.NewService(database)
	spaceUnread := spaces.NewUnreadCounter(vkClient)
	spaceHandlers := spaces.NewHandlers(spaceService, spaceUnread, contentService)

	// Register space auto-populate processor for the worker's space:auto_populate task.
	spaceAutoPopulateAdapter := spaces.NewProcessorAdapter(spaceService)
	worker.RegisterSpaceAutoPopulateProcessor(spaceAutoPopulateAdapter)

	// Geo HTTP handlers — /api/v1/geo/*. The hierarchy endpoints reuse
	// spaceService (countries/states/cities are seeded as spaces).
	geoHandlers := geoip.NewHandlers(geoipReader, spaceService)

	// Feature flags handlers — declared in the outer scope so both the
	// /api/v1 and /control route groups can use them.
	featureHandlers := features.NewHandlers(featureChecker)

	// Universities subsystem (Section 39) — read-view over spaces of the
	// University space-type + their custom field values, no dedicated table.
	universitiesHandlers := universities.NewHandlers(spaceService, cfService)

	// Bookmark Folders subsystem — first-class named folders (distinct from
	// bookmarks.folder, a free-text column).
	bookmarkFoldersService := bookmarkfolders.NewService(database)
	bookmarkFoldersHandlers := bookmarkfolders.NewHandlers(bookmarkFoldersService)

	// Create Asynq client for enqueuing background tasks (push delivery, etc.)
	asynqClient := worker.NewClient(worker.DefaultConfig())
	// Shared adapter so feature services enqueue real tasks instead of the
	// per-package noopEnqueuer that silently drops them (AUDIT-BE-001).
	taskEnqueuer := worker.NewEnqueuer(asynqClient)
	// Wire the enqueuer into the worker package so task handlers (e.g.
	// payment webhook) can enqueue follow-up tasks like affiliate commission.
	worker.RegisterTaskEnqueuer(taskEnqueuer)

	// Wire enqueuer into services that need async tasks.
	spaceService.SetEnqueuer(taskEnqueuer)
	spaceService.SetValkey(vkClient.Redis()) // VALKEY-001: mute key cache invalidation (Section 52)
	// AUDIT-C4: enqueue university:fetch_logo when an admin sets/changes a
	// university's logo_url via POST/PATCH /admin/universities.
	universitiesHandlers.SetEnqueuer(taskEnqueuer)

	// Wire the ProcessorAdapter for university enrichment (Section 39):
	// - customfields service: upserts founded_year, student_population,
	//   official_website_url from Wikidata data
	// - enqueuer: triggers university:fetch_logo when Wikidata returns a logo URL
	// - system user ID: attributes downloaded logos to the super admin
	spaceAutoPopulateAdapter.SetCustomFieldsEnricher(cfService)
	spaceAutoPopulateAdapter.SetUniversityLogoEnqueuer(taskEnqueuer)
	if sysUID, err := lookupSystemUserID(context.Background(), database); err == nil {
		spaceAutoPopulateAdapter.SetSystemUserID(sysUID)
	} else {
		slog.Warn("server: could not resolve system user for university logo attribution",
			"error", err)
	}

	// Hot-reloadable system settings (AUDIT-BE-009). Previously config.NewSettings
	// was never instantiated, so documented operational limits were dead and
	// services used compiled-in constants. Instantiate once and inject where limits
	// are read.
	settingsService := config.NewSettings(database, vkClient.Redis())
	closers = append(closers, settingsService.Close)
	contentService.SetSettings(settingsService)

	// BL-014: apply the configurable iframe embed allowlist from system_settings
	// at startup. The content sanitizer reads this list at render time under a
	// read lock, so a later settings change requires a process restart or a
	// dedicated reload hook. Empty/unset restores the compiled-in defaults.
	iframeHostsCSV := settingsService.GetStringOr(context.Background(), defaultTenantID, "content.sanitization.allowed_iframe_hosts", "")
	if iframeHostsCSV != "" {
		content.SetAllowedIframeHosts(strings.Split(iframeHostsCSV, ","))
	}

	// Content Reports & Flagging subsystem (Section 51).
	reportsService := reports.NewService(database, settingsService, vkClient.Redis())
	reportsHandlers := reports.NewHandlers(reportsService)
	reportsHandlers.SetEnqueuer(taskEnqueuer)

	// Content HTTP handlers (Service created earlier, above spaceService).
	contentHandlers := content.NewHandlers(contentService)
	contentHandlers.SetSpamPipeline(spamPipeline)
	contentHandlers.SetSettings(settingsService)

	// Renderer subsystem (Section 15 — Block Editor SSR).
	// The script path is validated against the allowlist root
	// (JC_RENDERER_SCRIPT_ROOT or ./scripts/tiptap-ssr) inside renderer.New.
	// A misconfigured path is logged and the renderer is left nil; the
	// route below registers a 503 stub handler so SSR requests fail
	// gracefully instead of nil-dereferencing (BUG-002).
	rendererInstance, err := renderer.New("scripts/tiptap-ssr.ts", vkClient)
	if err != nil {
		slog.Error("renderer init failed, SSR disabled", "error", err)
		rendererInstance = nil
	}
	var rendererHandlers *renderer.Handlers
	if rendererInstance != nil {
		rendererHandlers = renderer.NewHandlers(rendererInstance)
	}

	// Link preview subsystem (Section 15 — Link Preview Fetch).
	linkPreviewFetcher := linkpreview.NewFetcher(nil, vkClient)
	linkPreviewFetcher.SetSettings(settingsService) // SEC-013: configurable fetch timeout
	linkPreviewHandlers := linkpreview.NewHandlers(linkPreviewFetcher)

	// Register link preview processor for the worker's link_preview_fetch task.
	linkPreviewAdapter := linkpreview.NewProcessorAdapter(linkPreviewFetcher)
	worker.RegisterLinkPreviewProcessor(linkPreviewAdapter)

	// Follow System subsystem (Section 31 — Follow System).
	followService := follows.NewService(database)
	followHandlers := follows.NewHandlers(followService)
	// L3: wire Valkey for is_following read-through cache (60s TTL).
	followService.SetValkey(vkClient.Redis())
	// H3: route space follow through the generic follows service for
	// consistent auto-join (BL-010) and notification dispatch.
	spaceHandlers.SetFollowToggleService(followService)
	spaceHandlers.SetPermCacheInvalidator(rbacResolver)

	// BUG-003: wire the space posting-permission checker into the content
	// handler so CreateContent enforces the space's posting_perm axis
	// (Section 05 lines 145-154). The PermChecker is wired after
	// usersService is created (the "verified" axis needs email status).
	spacePostChecker := spaces.NewPostCheckerAdapter(spaceService)
	spacePostChecker.PermChecker = rbacResolver
	contentHandlers.SetSpacePostChecker(spacePostChecker)

	// Wire the space keyword matcher for auto-assigning content to spaces
	// based on title keyword matching (Section 05 — geo-first space system).
	contentHandlers.SetSpaceMatcher(spaceService)

	// General Tags subsystem (Sections 08, 49) — distinct from forum_tags.
	// Composes contentService + followService directly.
	tagsService := tags.NewService(database)
	tagsHandlers := tags.NewHandlers(tagsService, contentService, followService)
	// Wire the RBAC resolver so AddTagModerator/RemoveTagModerator can flush
	// the TARGET user's permission cache after the mutation (Rule 6).
	tagsHandlers.SetPermCacheInvalidator(rbacResolver)
	// Wire the tag slug resolver so content handlers can accept bulk {tag_slugs} (BUG-006).
	contentHandlers.SetTagResolver(tagSlugResolverAdapter{svc: tagsService})
	// Wire the guide category lister so ListContentTypes returns admin-configured categories.
	contentHandlers.SetGuideCategoryLister(guideCategoryListerAdapter{svc: cfService})

	// Notification System subsystem (Section 35 — Notification System).
	// Rule 3: VAPID keys come from the centralized config.Config (JC_VAPID_*)
	// rather than ad-hoc os.Getenv inside the notifications package.
	notificationService := notifications.NewServiceWithConfig(database, notifications.PushConfig{
		PrivateKey: cfg.VAPIDPrivateKey,
		PublicKey:  cfg.VAPIDPublicKey,
		Subject:    cfg.VAPIDSubject,
	})
	notificationHandlers := notifications.NewHandlers(notificationService)
	// C3: wire follow.received notification dispatch (Section 35). Fire-and-forget
	// per Rule 8 — the adapter ignores the DispatchResult and any error.
	followService.SetNotificationDispatcher(followNotificationAdapter{svc: notificationService})
	// BL-002: wire content mention notification dispatcher (fire-and-forget, Rule 8).
	contentHandlers.SetMentionNotifier(commentNotificationAdapter{svc: notificationService})
	// BUG-004: wire DB-backed privacy checkers for mention filtering
	// (blocks, erased users, private-space membership). Section 15.
	mentionCheckers := mentions.NewDBCheckers(database)
	contentHandlers.SetMentionCheckers(mentionCheckers)

	// Comments subsystem (Section 49 — Unified Comments).
	commentsService := comments.NewService(database)
	commentsService.SetSettings(settingsService) // BL-009: configurable comment edit window
	commentsHandlers := comments.NewHandlers(commentsService)
	commentsHandlers.SetSpamPipeline(spamPipeline)
	// BL-001: wire comment notification dispatcher (fire-and-forget, Rule 8).
	commentsHandlers.SetNotifier(commentNotificationAdapter{svc: notificationService})
	// Wire the Asynq client so comment creation enqueues a content engagement
	// count update (keeps content_scores.comment_count in sync for feed
	// ranking). Ref: Section 32, Guide §21.
	commentsHandlers.SetAsynqClient(asynqClient)
	// BUG-004: wire DB-backed privacy checkers for comment mention filtering.
	commentsService.SetMentionCheckers(mentionCheckers)
	// Wire erased-mention redaction for GDPR/ADR-029 compliance in comment
	// responses (ListComments / GetComment / UpdateComment).
	commentsHandlers.SetMentionCheckers(mentionCheckers)
	// BUG-003: wire the RBAC resolver so DeleteComment can distinguish
	// comments.delete_own (author) from comments.delete_any (moderator).
	commentsHandlers.SetPermissionChecker(rbacResolver)
	// BUG-002: wire the space permission checker so CreateComment enforces
	// the space's commenting_perm (top-level comments) and replying_perm
	// (replies) axes (Section 05 lines 145-154). Reuses the same adapter
	// already wired for content posting checks.
	commentsHandlers.SetSpaceChecker(spacePostChecker)

	// Interactions subsystem (Sections 41, 52 — Blocks & Mutes).
	interactionsService := interactions.NewService(database)
	interactionsService.SetValkey(vkClient.Redis()) // VALKEY-001: mute key cache invalidation
	interactionsHandlers := interactions.NewHandlers(interactionsService)

	worker.RegisterSettings(settingsService)
	worker.RegisterRedis(vkClient.Redis()) // TASK-003: RSS feed caching in Valkey
	// BUG-005: configure the Wiki API rate limiter interval from
	// spaces.wiki_rate_interval_ms (default 2000ms). Hot-reloadable — the
	// worker re-reads the setting on each wiki enrichment task.
	spaces.SetWikiRateInterval(time.Duration(
		settingsService.GetIntOr(context.Background(), defaultTenantID, "spaces.wiki_rate_interval_ms", 2000),
	) * time.Millisecond)

	// Initialize email provider gateway (ADR-015 — multi-provider failover)
	emailDispatcher := email.NewEmailDispatcher()
	if smtpProvider, err := email.NewSMTPProvider(); err != nil {
		slog.Warn("email: smtp provider not available", "error", err)
	} else if smtpProvider != nil {
		emailDispatcher.RegisterProvider("transactional", smtpProvider)
		emailDispatcher.RegisterProvider("marketing", smtpProvider)
		slog.Info("email: registered smtp provider")
	}
	worker.RegisterEmailGateway(emailDispatcher)

	// Inject into services with hot-reloadable moderation/ops limits (AUDIT-BE-009).
	spamPipeline.SetSettings(settingsService)
	notificationService.SetAsynqClient(asynqClient)
	notificationService.SetSettings(settingsService)
	notificationService.SetValkey(vkClient)
	authHandlers.AsynqClient = asynqClient
	authHandlers.Settings = settingsService
	oauthHandlers.Settings = settingsService
	kr.Settings = settingsService
	contentHandlers.AsynqClient = asynqClient
	// Wire the Asynq client into customfields so SubmitForm can enqueue
	// form:fire_submission_triggers tasks (Section 06, BUG-002).
	cfHandlers.SetAsynqClient(asynqClient)
	// Wire password policy from system_settings (BE-054).
	if defaultTenantPW, err := tenantStore.GetDefaultTenant(context.Background()); err == nil && defaultTenantPW != nil {
		authHandlers.Policy = auth.PasswordPolicyFromSettings(context.Background(), settingsService, defaultTenantPW.ID)
	}
	// Wire link preview blacklist fallback from system_settings (AUDIT-BE-010,
	// BE-037, BUG-001). The blacklist is a JSON array of domains stored under
	// 'content.link_preview.blacklist'. This startup load is a fallback for
	// when the settings reader is not wired per-request; the Fetcher also
	// resolves the blacklist per-request via resolveBlacklist (BUG-004).
	if defaultTenant, err := tenantStore.GetDefaultTenant(context.Background()); err == nil && defaultTenant != nil {
		if blacklistStr, err := settingsService.GetSetting(context.Background(), defaultTenant.ID, "content.link_preview.blacklist"); err == nil && blacklistStr != "" {
			blacklist := linkpreview.ParseBlacklistJSON(blacklistStr)
			if len(blacklist) > 0 {
				linkPreviewFetcher.SetBlacklist(blacklist)
			}
			slog.Info("link preview: blacklist loaded from system_settings", "count", len(blacklist))
		}
	} else {
		slog.Warn("link preview: could not resolve default tenant for blacklist", "error", err)
	}

	// Feed System subsystem (Section 32 — Activity Feed / Timeline).
	feedService := feed.NewService(database, vkClient.Redis())
	feedService.SetSettings(settingsService) // BUG-FEED-004: For-You feed reads feed.for_you_window_days, feed.max_followed_spaces_in_feed, and geo_personalization.enabled from system_settings (Rule 3).
	feedHandlers := feed.NewHandlers(feedService)
	feedHandlers.SetSettings(settingsService)
	feedHandlers.SetCache(vkClient.Valkey()) // BUG-006: explore feed caching

	// Trending & Discovery subsystem (Phase 28).
	trendingStore := trending.NewStore(database)
	trendingHandlers := trending.NewHandlers(trendingStore)
	trendingHandlers.SetCache(vkClient.Valkey())

	// Recommendation engine (ADR-064a) — geo-aware tool/feature suggestions.
	recommendationsHandlers := recommendations.NewHandlers(settingsService, vkClient.Redis(), trendingStore)

	// Register trending processor with worker for background tasks.
	trendingProc := trending.NewProcessor(database, slog.Default())
	trendingProc.SetSettings(settingsService) // BUG-016: configurable scoring weights
	trendingAdapter := trending.NewWorkerAdapter(trendingProc)
	worker.RegisterTrendingProcessor(trendingAdapter)

	// Blog Extension subsystem (Section 24 — Blog Extension).
	blogService := blog.NewService(database)
	blogHandlers := blog.NewHandlers(blogService, contentService, cfg.SiteURL, cfg.SiteName)
	// Wire mention notification dispatch for blog posts (Section 15 / ADR-029).
	// Fire-and-forget per Rule 8.
	blogHandlers.SetMentionNotifier(commentNotificationAdapter{svc: notificationService})
	// Wire erased-mention redaction for GDPR/ADR-029 compliance.
	blogHandlers.SetMentionCheckers(mentionCheckers)
	// Wire RSS cache + on-publish regeneration (Section 24 — rss:regenerate
	// task is enqueued on publish and the live /feed.xml route reads the
	// cached XML from jc:cache:feed:rss:{tenant_id}). Without these the
	// handler's AsynqClient/Redis are nil, so enqueue is a no-op and every
	// feed request hits the database.
	blogHandlers.SetAsynqClient(asynqClient)
	blogHandlers.SetRedis(vkClient.Redis())

	// Reviews subsystem (Section 29 — Universal Ratings & Reviews).
	reviewService := reviews.NewService(database)
	reviewHandlers := reviews.NewHandlers(reviewService)
	reviewHandlers.AsynqClient = asynqClient

	// Comparison subsystem (Section 30 — Generic Comparison System).
	comparisonService := comparison.NewService(database)
	comparisonHandlers := comparison.NewHandlers(comparisonService)

	// Professional Profiles subsystem (Section 33 — Professional Profiles).
	professionalService := professional.NewService(database)
	professionalHandlers := professional.NewHandlers(professionalService, rbacResolver, followService)

	// Organization system (Section 07).
	orgService := organizations.NewServiceWithSettings(database, settingsService)
	orgService.SetValkeyCache(vkClient.Redis()) // jc:org:profile / jc:org:list read-through cache
	orgService.SetAsynqClient(asynqClient)      // BUG-018: invitation email:send tasks
	orgHandlers := organizations.NewHandlers(orgService, followService, contentService)
	orgHandlers.SetCustomFieldLookup(cfService) // org_type options from custom_fields (Section 07 + 08)
	orgHandlers.SetAsynqClient(asynqClient)     // §12 stale-index fix: org CRUD enqueues search:index_entity

	// Wallet & Credits subsystem (Section 28 — Wallet & Credits).
	walletService := wallet.NewService(database)
	walletService.SetSettings(settingsService) // SETTING-007/008: global conversion rate + min convert points
	walletHandlers := wallet.NewHandlers(walletService)
	walletService.SetEnqueuer(taskEnqueuer)

	// Currency resolver for geo-aware currency resolution at checkout
	// (Section 58, ADR-065). The default currency is read from
	// system_settings wallet.default_currency (fallback NGN). The default
	// tenant ID is used because this read happens at boot time, outside any
	// request context. The resolver also has SetDefaultCurrency for
	// hot-reload when the setting changes.
	defaultCurrency := settingsService.GetStringOr(context.Background(), defaultTenantID, "wallet.default_currency", "NGN")
	currencyResolver := wallet.NewCurrencyResolver(walletService.Store(), defaultCurrency)
	walletService.SetCurrencyResolver(currencyResolver)

	// Wire the Valkey cache invalidator for the currency cache
	// (Section 58, Rule 4). After any currency CRUD operation, the
	// jc:wallet:currencies:{tenant_id} key is flushed.
	walletService.SetCacheInvalidator(&walletCacheInvalidator{vk: vkClient.Redis()})

	// Register wallet processor with worker for background tasks.
	worker.RegisterWalletProcessor(walletService)

	// Payment Core subsystem (Section 17 — Payment Core).
	paymentService := payments.NewService(database, &walletCreditAdapter{svc: walletService})
	paymentService.SetEnqueuer(taskEnqueuer)

	// Wire the payments service back into the wallet so credit purchases can
	// create payment rows (Section 58). The adapter converts between the
	// wallet.PaymentCreator interface and payments.Service.CreatePayment.
	walletService.SetPaymentsService(&walletPaymentAdapter{svc: paymentService})

	// Register payment providers so InitiatePayment can call real provider APIs.
	paymentService.SetProvider(payments.NewPaystackProvider(cfg.PaystackSecretKey))
	stripeProvider := payments.NewStripeProvider(cfg.StripeSecretKey)
	stripeProvider.SetWebhookSecret(cfg.StripeWebhookSecret)
	paymentService.SetProvider(stripeProvider)
	paymentService.SetProvider(payments.NewNOWPaymentsProvider(cfg.NOWPaymentsIPNSecret))
	paymentService.SetProvider(payments.NewManualTransferProvider())

	paymentHandlers := payments.NewHandlers(paymentService)
	paymentHandlers.SetSettings(settingsService) // SETTING-001: configurable pricing
	paymentHandlers.StripeWebhookSecret = cfg.StripeWebhookSecret
	paymentHandlers.NOWPaymentsIPNKey = cfg.NOWPaymentsIPNSecret

	// Wire wallet, payments, and settings into customfields so SubmitForm can
	// handle paid form submissions (Section 06 Paid Form Submissions).
	cfHandlers.SetWalletService(walletService)
	cfHandlers.SetPaymentsService(paymentService)
	cfHandlers.SetSettings(settingsService)
	// BUG-001: wire the HTML sanitizer so rich_text field values are scrubbed
	// before storage (stored XSS prevention). content.SanitizeHTML is the
	// whitelist-based sanitizer shared across the platform.
	cfHandlers.SetRichTextSanitizer(content.SanitizeHTML)

	// Advertising subsystem (Section 25 — Ad Placement).
	adService := advertising.NewService(database, settingsService)
	adHandlers := advertising.NewHandlers(adService)
	adHandlers.AsynqClient = asynqClient

	// Ticket System subsystem (Section 38 — Ticket System).
	ticketService := tickets.NewServiceWithEnqueuer(database, taskEnqueuer)
	ticketService.SetSettings(settingsService)
	ticketHandlers := tickets.NewHandlers(ticketService)

	// Coupons & External Affiliate Providers subsystem (Section 55).
	couponService := coupons.NewServiceWithEnqueuer(database, taskEnqueuer)
	couponService.SetCache(vkClient.Redis()) // VALKEY-005: jc:coupon:{id} + jc:coupon_list:{tenant} caching (§55)
	couponHandlers := coupons.NewHandlers(couponService)

	// Wire the coupon service into the renderer for render-time coupon_embed
	// resolution and affiliate link href rewriting (Section 55 P4, BUG-004/005).
	if rendererInstance != nil {
		rendererInstance.SetCouponResolver(&couponRenderAdapter{svc: couponService})
		rendererInstance.SetAffiliateLinkResolver(&affiliateLinkRenderAdapter{svc: couponService})
	}

	// User Affiliate Program subsystem (Section 56).
	affiliateService := affiliate.NewServiceWithEnqueuer(database, taskEnqueuer)
	affiliateService.SetSettings(settingsService) // SETTING-002/003: configurable commission/limits
	affiliateHandlers := affiliate.NewHandlers(affiliateService)
	affiliateHandlers.AsynqClient = asynqClient
	affiliateHandlers.Cache = vkClient.Redis() // jc:aff_dedup + jc:aff_click (Section 56)

	// i18n Translation System.
	i18nService := i18n.NewService(database)
	i18nService.SetValkey(vkClient.Redis()) // VALKEY-002: distributed i18n cache
	i18nHandlers := i18n.NewHandlers(i18nService)
	i18nHandlers.AsynqClient = asynqClient

	// Register AI translate processor for the worker's i18n:translate_language_pack task.
	i18nTranslateAdapter := i18n.NewProcessorAdapter(i18nService)
	worker.RegisterAITranslateProcessor(i18nTranslateAdapter)

	// GDPR / NDPR Compliance subsystem (Section 19).
	gdprService := gdpr.NewServiceWithEnqueuer(database, taskEnqueuer, cfg.SiteName)
	gdprService.SetHonchoEraser(honcho.NewClient(cfg.HonchoURL))
	gdprService.SetSessionDeleter(gdpr.NewValkeySessionDeleter(vkClient.Redis()))
	gdprService.SetSettings(settingsService)
	// GAP-GDPR-A: explicit LocalStorage wiring so the data export file is
	// persisted to data/gdpr-exports/{tenantID}/. Swap for an S3/SeaweedFS
	// adapter (ADR-048) in production via gdprService.SetStorage(...).
	gdprService.SetStorage(gdpr.NewLocalStorage(""))
	gdprHandlers := gdpr.NewHandlers(gdprService)

	// Register GDPR processor with worker for background tasks.
	gdprAdapter := gdpr.NewWorkerAdapter(gdprService)
	worker.RegisterGDPRProcessor(gdprAdapter)

	// Menus — Navigation menus with RBAC-gated items (ADR-054).
	// Wired with the RBAC resolver so the public GET /menus/{slug} endpoint
	// prunes items whose permission_node is not held by the authenticated
	// user (ADR-054 "Menu item visibility rules").
	menusService := menus.NewServiceWithPerms(database, rbacResolver)
	menusHandlers := menus.NewHandlersWithPerms(menusService, rbacResolver)

	// Sidebars — Sidebar Management System (Section 59, ADR-066).
	// Replaces the deprecated pagebuilder sidebar approach.
	sidebarService := sidebars.NewServiceWithPerms(database, vkClient.Redis(), rbacResolver)
	sidebarHandlers := sidebars.NewHandlers(sidebarService)

	// Email Templates — Tenant-customizable transactional emails (ADR-055).
	emailTemplatesService := emailtemplates.NewService(database)
	emailTemplatesHandlers := emailtemplates.NewHandlers(emailTemplatesService, asynqClient)
	emailTemplatesHandlers.SetRedisClient(vkClient.Redis())

	// Redirects — Admin-managed URL redirect rules (ADR-056).
	redirectsService := redirects.NewService(database, vkClient.Redis())
	redirectsService.SetEnqueuer(taskEnqueuer)
	redirectsHandlers := redirects.NewHandlers(redirectsService)

	// Redirect middleware (ADR-056) — resolves admin-managed URL redirects
	// before the request reaches route handlers. Runs on the root router
	// (after standard middleware + IP block) but before Tenant/Auth groups.
	// Skips /api/, /health, /ready, and /seaweedfs/ paths.
	r.Use(redirects.Middleware(redirectsService, tenantStore))

	// Media subsystem (Section 34 — Media System).
	// Rule 3: S3 storage config comes from the centralized config.Config
	// (JC_S3_*) rather than ad-hoc os.Getenv inside the media package.
	mediaService := media.NewServiceWithConfig(database, media.S3Config{
		Endpoint:  cfg.S3Endpoint,
		AccessKey: cfg.S3AccessKey,
		SecretKey: cfg.S3SecretKey,
		Region:    cfg.S3Region,
		Bucket:    cfg.S3Bucket,
	})
	// Inject system_settings provider for Rule 3 compliance
	// (media.max_upload_mb, media.allowed_video_types, media.cdn.base_url).
	mediaService.SetSettings(settingsService)
	// Rebuild the S3 client from media.storage.* system_settings when the
	// operator has configured storage via the Control Panel (Section 34).
	// When the settings are absent the env-var-based client is retained.
	mediaService.ReconfigureS3FromSettings(context.Background(), defaultTenantID)
	// Wire the admin-configurable CDN base URL (media.cdn.base_url) into the
	// package-level media.MediaURL so all callers (tags, blog, feed, widgets,
	// seo) honour the system_setting without each needing a *media.Service.
	// Section 34 — CDN & S3-Compatible Object Storage. Resolved against the
	// default tenant because the CDN prefix is platform-wide.
	media.SetCDNBaseURLResolver(func() string {
		return settingsService.GetStringOr(context.Background(), defaultTenantID, "media.cdn.base_url", "")
	})
	mediaHandlers := media.NewHandlers(mediaService)
	// Inject RBAC resolver for in-handler permission checks (media.video.upload).
	mediaHandlers.SetPermissionChecker(rbacResolver)
	// Resolve logo_media_id to an imgproxy-signed logo_url in university
	// and space responses (Section 34 / ADR-039 — audit finding H5).
	universitiesHandlers.SetMediaResolver(mediaService)
	spaceHandlers.SetMediaResolver(mediaService)

	// Stock photo service — provider search and import (Unsplash, Pexels,
	// Pixabay, Openverse). ReloadProviders seeds provider rows from env vars
	// and constructs enabled provider instances.
	stockService := stock.NewService(database, mediaService, cfg)
	if err := stockService.ReloadProviders(context.Background()); err != nil {
		slog.Warn("server: failed to load stock providers on startup", "error", err)
	}
	stockHandlers := stock.NewStockHandlers(stockService)

	// Register image processor for the worker's content:process_image task.
	mediaImageAdapter := media.NewProcessorAdapter(mediaService)
	worker.RegisterImageProcessor(mediaImageAdapter)

	// ClamAV virus scanner — connects to the ClamAV daemon for media
	// scanning. The ScanProcessor orchestrates the full scan flow: read
	// media record → open file → send to ClamAV → update status.
	clamavClient := media.NewClamAVClient(cfg.ClamAVAddr)
	mediaScanProcessor := media.NewScanProcessor(mediaService, clamavClient)
	worker.RegisterMediaScanProcessor(mediaScanProcessor)

	// Video processor — ffmpeg-based transcoding, thumbnail generation,
	// and compression for video uploads. Registered with the worker so
	// the video_processing handler can invoke it asynchronously.
	worker.RegisterVideoProcessor(media.NewVideoProcessor())

	// Webhook Delivery subsystem — registers the processor so the Asynq
	// webhook_delivery handler can fetch, sign, and deliver outgoing webhooks.
	webhookService := webhooks.NewService(database, cfg.SiteName)
	webhookService.SetSettings(settingsService)
	webhookService.SetEnqueuer(taskEnqueuer)
	webhookHandlers := webhooks.NewHandlers(webhookService)
	webhookAdapter := webhooks.NewWorkerAdapter(webhookService)
	worker.RegisterWebhookProcessor(webhookAdapter)

	// Push Delivery — registers the VAPID push delivery implementation
	// so the Asynq push_delivery handler can send web push notifications.
	worker.RegisterPushDelivery(notificationService.PushDelivery())
	worker.RegisterNotificationBatchDispatcher(notificationService)

	// Form submission triggers (Section 06, BUG-002): register the
	// TriggerDispatcher so the form:fire_submission_triggers worker handler
	// can dispatch assign_user_group / send_email / send_webhook /
	// create_ticket / notify_admin triggers after a form submission.
	formTriggerDispatcher := customfields.NewTriggerDispatcher(cfService)
	formTriggerDispatcher.SetRBACAssigner(func(ctx context.Context, tenantID, userID, groupID pgtype.UUID) error {
		return rbacStore.AddUserToGroup(ctx, tenantID, userID, groupID)
	})
	formTriggerDispatcher.SetNotifyAdmin(func(ctx context.Context, tenantID pgtype.UUID, title, body, actionURL string) error {
		// Fan out an in-app notification to every active user in the
		// tenant's system super_admin group (Section 06 notify_admin
		// trigger). Best-effort: a failure for one admin does not abort
		// the others; the first error is returned so the trigger
		// dispatcher can log it.
		queries := gen.New(database.Reader)
		adminIDs, err := queries.ListTenantAdminIDs(ctx, tenantID)
		if err != nil {
			slog.Error("customfields: notify_admin form trigger, listing tenant admins",
				"tenant_id", tenantID.String(), "error", err)
			return fmt.Errorf("notify_admin: listing tenant admins: %w", err)
		}
		slog.Info("customfields: notify_admin form trigger",
			"tenant_id", tenantID.String(), "title", title, "admin_count", len(adminIDs))
		var systemID pgtype.UUID
		var firstErr error
		for _, adminID := range adminIDs {
			if err := notificationService.CreateNotification(ctx, tenantID, adminID,
				"admin_notification", "system", systemID,
				"form_submission", systemID,
				title, body, actionURL); err != nil {
				slog.Warn("customfields: notify_admin delivery failed",
					"tenant_id", tenantID.String(), "admin_id", adminID.String(), "error", err)
				if firstErr == nil {
					firstErr = err
				}
			}
		}
		return firstErr
	})
	// Wire the create_ticket trigger (Section 06 → Section 38): form
	// submissions with a create_ticket trigger create a support ticket
	// via the ticket service. Without this hook the trigger silently skips.
	formTriggerDispatcher.SetTicketCreator(func(ctx context.Context, tenantID pgtype.UUID, subject, description string, submitterUserID *pgtype.UUID) error {
		input := tickets.CreateTicketInput{
			Subject:     subject,
			Description: description,
			Priority:    string(tickets.PriorityNormal),
		}
		if submitterUserID != nil {
			input.SubmitterUserID = *submitterUserID
		}
		_, err := ticketService.CreateTicket(ctx, tenantID, input)
		return err
	})
	worker.RegisterFormTriggerProcessor(formTriggerDispatcher)

	// Documentation System subsystem (Section 44).
	docsService := docs.NewService(database)
	docsService.SetCache(vkClient.Redis()) // VALKEY-003: cache doc pages + nav
	docsHandlers := docs.NewHandlers(docsService, cfg.SiteURL, cfg.SiteName)
	docsHandlers.AsynqClient = asynqClient // SEARCH-001: enqueue Meilisearch index sync
	docsHandlers.SetRBAC(rbacResolver)     // §44: audience RBAC gating

	// Admin Settings subsystem.
	adminHandlers := admin.NewHandlers(database, gen.New(database.Writer))
	adminHandlers.SetRBAC(rbacResolver)
	adminHandlers.SetValkey(vkClient.Redis())
	adminHandlers.SetSettings(settingsService)
	adminHandlers.SetGDPR(gdprService)
	adminHandlers.SetOrgVerifier(orgService)
	adminHandlers.SetTaskEnqueuer(taskEnqueuer)

	// Bot & Crawler Management subsystem (Section 43).
	botStore := bots.NewStoreWithDB(database)
	botService := bots.NewServiceWithValkey(botStore, vkClient)
	botHandlers := bots.NewHandlers(botService)

	// Spam Prevention & Content Safety admin handlers (Section 41).
	// Wired with the Valkey client so censored-word mutations invalidate the
	// jc:spam:censored:{tenant_id} cache immediately (BUG-002).
	spamHandlers := spam.NewHandlersWithCache(spamStore, vkClient)

	// Platform Analytics & Site Reports subsystem (Section 40).
	analyticsService := analytics.NewServiceWithEnqueuer(database, taskEnqueuer)
	analyticsHandlers := analytics.NewHandlers(analyticsService)

	// Learning Platform subsystem (Section 36).
	learningStore := learning.NewStore(database)
	learningHandlers := learning.NewHandlers(learningStore, contentService)
	learningHandlers.AsynqClient = asynqClient

	// Register learning processor for the worker's learning tasks.
	learningAdapter := learning.NewProcessorAdapter(learningStore)
	worker.RegisterLearningProcessor(learningAdapter)

	// Japa University creator-economy subsystem (ADR-063 — upgrade to Learn).
	// The handlers/service/store already exist; this wires them into the router
	// and the worker so the 24 catalogued routes stop 404ing (ROUTE-001/002).
	universityStore := university.NewStore(database)
	universityService := university.NewService(universityStore, settingsService, taskEnqueuer)
	universityService.SetValkey(vkClient.Redis())
	universityService.SetMediaUploader(mediaService)
	universityService.SetSpaceLogoUpdater(spaceService)
	universityService.SetNotifier(notificationService)
	universityHandlers := university.NewHandlers(universityService)
	// Register university processor for the worker's 5 university Asynq tasks.
	universityProcessor := university.NewProcessor(universityService)
	worker.RegisterUniversityProcessor(universityProcessor)

	// Newsletter subsystem (Section 33).
	newsletterService := newsletter.NewService(database)
	newsletterService.SetAsynqClient(asynqClient)
	newsletterService.SetSettings(settingsService)
	newsletterHandlers := newsletter.NewHandlers(newsletterService)

	// Referral Program subsystem (Section 34 / §60, ADR-078).
	referralService := referral.NewService(database)
	referralService.SetSettings(settingsService)
	referralService.SetValkey(vkClient.Redis()) // jc:referral_attr attribution key
	referralService.SetEnqueuer(taskEnqueuer)   // referral:record_click + wallet:award_points
	referralHandlers := referral.NewHandlers(referralService)
	// Wire referral capture into the auth signup flow (§60 — ?ref=CODE at
	// registration creates a pending referrals row). Injected here because
	// referral.Service is constructed after AuthHandlers.
	authHandlers.SetReferralCapturer(referralService)

	// V2 feature subsystems (Sections 61–76, ADRs 079–085).
	// Each follows the same Store→Service→Handlers layering as v1 packages.
	// Ref: japaconnect-engineering/sections/63-jobs-board.md (ADR-081)
	jobsStore := jobs.NewStore(database)
	jobsService := jobs.NewService(jobsStore, settingsService)
	jobsService.SetEnqueuer(taskEnqueuer) // jobs:notify_application_status (§63, ADR-081)
	jobsHandlers := jobs.NewHandlers(jobsService, settingsService)

	// Ref: japaconnect-engineering/sections/64-gamification.md (ADR-082)
	gamificationStore := gamification.NewStore(database)
	gamificationService := gamification.NewService(gamificationStore, settingsService)
	gamificationHandlers := gamification.NewHandlers(gamificationService, settingsService)
	// Wire the wallet service as the gamification PointsAwarder so challenge
	// completion credits reward_points to the user's wallet (BUG-GAMMA-002).
	// The wallet service enqueues wallet:award_points fire-and-forget (Rule 8).
	gamificationService.SetPointsAwarder(walletService)
	// Wire the notification service as the gamification Notifier so badge
	// awards and challenge completions send user notifications
	// (BUG-GAMMA-003, ADR-082 §2). Fire-and-forget per Rule 8.
	gamificationService.SetNotifier(gamificationNotifierAdapter{svc: notificationService})
	// Wire the gamification evaluator into the wallet service so
	// wallet:process_activity_event evaluates activity_count badges,
	// lifetime_points badges, and challenge progress after each successful
	// ledger earn (BUG-GAMMA-001, ADR-082 §2).
	walletService.SetGamificationEvaluator(gamificationService)

	// Ref: japaconnect-engineering/sections/65-content-import.md (ADR-083)
	contentImportStore := contentimport.NewStore(database)
	contentImportService := contentimport.NewService(contentImportStore, settingsService)
	// Wire the Asynq task enqueuer so StartImport and RetryImport can
	// enqueue the import:process_source and import:fetch_media tasks
	// fire-and-forget (Rule 8). Without this, imports are created with
	// status='pending' but never processed — the worker never picks them up.
	contentImportService.SetEnqueuer(taskEnqueuer)
	// Wire the content creator adapter so ProcessSource can create draft
	// contents rows. Without this, importSingleItem returns "no content
	// creator configured" and every import fails.
	contentImportService.SetContentCreator(newContentCreatorAdapter(contentService))
	// Wire the media fetcher adapter so import:fetch_media downloads
	// external images and replaces their URLs with stored media URLs.
	contentImportService.SetMediaFetcher(newMediaFetcherAdapter(contentService, mediaService))
	contentImportHandlers := contentimport.NewHandlers(contentImportService, settingsService)

	// Ref: japaconnect-engineering/sections/67-platform-data-portability.md
	// Platform data export/import (admin-initiated, separate from GDPR
	// per-user exports). Supports CSV/JSON/XLSX export and CSV/JSON import
	// of users, posts, comments, orders, transactions, analytics, moderation.
	platformDataExportService := dataexport.NewService(database)
	platformDataExportService.SetEnqueuer(taskEnqueuer)
	platformDataExportHandlers := dataexport.NewHandlers(platformDataExportService)
	worker.RegisterPlatformDataExportProcessor(platformDataExportService)

	platformDataImportService := dataimport.NewService(database)
	platformDataImportService.SetEnqueuer(taskEnqueuer)
	platformDataImportHandlers := dataimport.NewHandlers(platformDataImportService)
	worker.RegisterPlatformDataImportProcessor(platformDataImportService)

	// Ref: japaconnect-engineering/sections/66-advanced-analytics.md (ADR-084)
	advancedAnalyticsStore := advancedanalytics.NewStore(database)
	advancedAnalyticsService := advancedanalytics.NewService(advancedAnalyticsStore, settingsService)
	advancedAnalyticsHandlers := advancedanalytics.NewHandlers(advancedAnalyticsService)

	// Ref: japaconnect-engineering/sections/67-travel-search.md (ADR-085)
	travelStore := travel.NewStore(database)
	travelService := travel.NewService(travelStore, settingsService)
	travelService.SetCache(vkClient.Redis()) // VALKEY-006: jc:travel:search:{hash} result cache (§67)
	travelHandlers := travel.NewHandlers(travelService, settingsService)

	// Ref: japaconnect-engineering/sections/68-ai-personas.md (ADR-068)
	personasStore := personas.NewStore(database)
	personasService := personas.NewService(personasStore, settingsService, taskEnqueuer)
	personasHandlers := personas.NewHandlers(personasService)

	// Ref: japaconnect-engineering/sections/70-visa-checklist.md (ADR-070)
	checklistStore := checklist.NewStore(database)
	checklistService := checklist.NewService(checklistStore, settingsService)
	checklistService.SetEnqueuer(taskEnqueuer) // wallet:award_points on full completion (§28)
	checklistHandlers := checklist.NewHandlers(checklistService, settingsService)

	// Ref: japaconnect-engineering/sections/79-community-story-wall.md (ADR-087)
	storyStore := story.NewStore(database)
	storyService := story.NewService(storyStore, contentService, settingsService)
	storyService.SetFeatureChecker(featureChecker)
	storyHandlers := story.NewHandlers(storyService, settingsService)

	// Ref: japaconnect-engineering/sections/80-destination-wiki.md (ADR-088)
	wikiStore := wiki.NewStore(database)
	wikiService := wiki.NewService(wikiStore, settingsService)
	wikiService.SetFeatureChecker(featureChecker)
	wikiHandlers := wiki.NewHandlers(wikiService, settingsService)
	// B4/B5: wire the reputation (points) checker and RBAC resolver so the
	// wiki.edit submission gate can enforce wiki.min_reputation_to_edit and
	// wiki.community_edits (§80 business-logic gates).
	wikiHandlers.SetPointsChecker(walletService)
	wikiHandlers.SetPermissionChecker(rbacResolver)

	// Ref: japaconnect-engineering/sections/81-scam-alert-board.md (ADR-089)
	scamAlertStore := scamalert.NewStore(database)
	scamAlertService := scamalert.NewService(scamAlertStore, settingsService)
	scamAlertService.SetFeatureChecker(featureChecker)
	scamAlertHandlers := scamalert.NewHandlers(scamAlertService, settingsService)
	// Wire the Asynq enqueuer for fire-and-forget notification fan-out on
	// report confirmation (Rule 8), the notification service for
	// subscriber dispatch (§35), and the gamification service for the
	// Community Guardian badge (§64). Register the service as the
	// scam_alert:notify_subscribers processor (§81, ADR-089).
	scamAlertService.SetEnqueuer(taskEnqueuer)
	scamAlertService.SetNotifier(notificationService)
	scamAlertService.SetBadgeAwarder(gamificationService)
	worker.RegisterScamAlertProcessor(scamAlertService)

	// Ref: japaconnect-engineering/sections/82-japa-planning-groups.md (ADR-090)
	planningGroupStore := planninggroups.NewStore(database)
	planningGroupService := planninggroups.NewService(planningGroupStore, settingsService).
		WithNotifier(notificationService)
	planningGroupHandlers := planninggroups.NewHandlers(planningGroupService, settingsService)
	// Register the service as the planning_group:expire /
	// planning_group:checkin_reminder processor so the Asynq tasks are
	// handled (§82, ADR-090).
	worker.RegisterPlanningGroupProcessor(planningGroupService)

	// Ref: japaconnect-engineering/sections/83-visa-timeline-tracker.md (ADR-091)
	visaTimelineStore := visatimeline.NewStore(database)
	visaTimelineService := visatimeline.NewService(visaTimelineStore, settingsService)
	visaTimelineHandlers := visatimeline.NewHandlers(visaTimelineService, settingsService)
	// Wire the wallet service for the anti-gaming reputation gate on shared
	// timelines (§83: contribute to aggregate requires reputation ≥
	// visa_timeline.min_reputation).
	visaTimelineHandlers.SetPointsChecker(walletService)
	// Register the service as the visa_timeline:refresh_stats /
	// visa_timeline:prompt_update processor so the Asynq tasks are
	// handled (§83, ADR-091).
	worker.RegisterVisaTimelineProcessor(visaTimelineService)

	// Ref: japaconnect-engineering/sections/84-community-digest-widget.md (ADR-092)
	digestStore := digest.NewStore(database)
	digestService := digest.NewService(digestStore, settingsService)
	digestService.SetFeatureChecker(featureChecker)
	digestService.SetEnqueuer(taskEnqueuer)
	digestService.SetValkey(vkClient.Redis())
	digestService.SetEmailDispatcher(emailDispatcher)
	digestHandlers := digest.NewHandlers(digestService, settingsService)
	// Register the service as the digest:generate / digest:send_email
	// processor so the Asynq tasks are handled (§84, ADR-092).
	worker.RegisterDigestProcessor(digestService)

	// Ref: japaconnect-engineering/sections/85-mutual-aid-board.md (ADR-093)
	mutualAidStore := mutualaid.NewStore(database)
	mutualAidService := mutualaid.NewService(mutualAidStore, settingsService)
	mutualAidService.SetNotifier(notificationService)
	mutualAidHandlers := mutualaid.NewHandlers(mutualAidService, settingsService)
	// Wire RBAC resolver for post_type-specific permission checks
	// (create_need, create_skill_swap) and wallet service for the
	// reputation gate on need posts.
	mutualAidHandlers.SetPermissionChecker(rbacResolver)
	mutualAidHandlers.SetPointsChecker(walletService)
	// DM adapter is wired later, after wsDMService is created.

	// Ref: japaconnect-engineering/sections/86-community-calendar.md (ADR-094)
	calendarStore := calendar.NewStore(database)
	calendarService := calendar.NewService(calendarStore, settingsService)
	calendarService.SetNotifier(notificationService)
	calendarHandlers := calendar.NewHandlers(calendarService, settingsService)
	// Register the service as the calendar processor so the
	// calendar:staleness_check / calendar:send_reminders Asynq tasks are
	// handled (§86, ADR-094).
	worker.RegisterCalendarProcessor(calendarService)

	// Register the mutual aid service as the mutual_aid:expire processor
	// so the nightly Asynq task is handled (§85, ADR-093).
	worker.RegisterMutualAidProcessor(mutualAidService)

	// Ref: japaconnect-engineering/sections/87-japa-stage-profiles.md (ADR-095)
	japaStageStore := japastage.NewStore(database)
	japaStageService := japastage.NewService(japaStageStore, settingsService).
		WithNotifier(notificationService).
		WithHoncho(honcho.NewClient(cfg.HonchoURL))
	japaStageHandlers := japastage.NewHandlers(japaStageService, settingsService, asynqClient).WithRedis(vkClient.Redis())

	// Ref: japaconnect-engineering/sections/88-ask-an-expat.md (ADR-096)
	amaStore := ama.NewStore(database)
	amaService := ama.NewService(amaStore, settingsService)
	amaService.SetRBAC(rbacResolver)
	amaService.SetWebhookEmitter(&forumWebhookEmitter{svc: webhookService})
	amaService.SetValkey(vkClient.Redis())
	amaHandlers := ama.NewHandlers(amaService)

	// Ref: japaconnect-engineering/sections/89-achievement-feed.md (ADR-097)
	achievementStore := achievement.NewStore(database)
	achievementService := achievement.NewService(achievementStore, contentService, settingsService).
		WithActivityEmitter(&achievementActivityEmitter{svc: walletService})
	achievementHandlers := achievement.NewHandlers(achievementService, settingsService)

	// Ref: japaconnect-engineering/sections/98-service-directory.md (ADR-106)
	serviceDirStore := servicedir.NewStore(database)
	serviceDirService := servicedir.NewService(serviceDirStore, contentService, settingsService)
	// BUG-014: §98 spec says GET /services/:slug returns "service detail +
	// reviews". The adapter bridges the §29 reviews subsystem to the
	// servicedir.ReviewFetcher interface using the read pool.
	serviceDirReviewFetcher := serviceReviewAdapter{read: gen.New(database.Reader)}
	serviceDirHandlers := servicedir.NewHandlers(serviceDirService, settingsService, rbacResolver, serviceDirReviewFetcher)

	// Phase 49 — V2 Functional Tools (§90–§97, §99–§101)
	// Ref: japaconnect-engineering/sections/90-97,99-101
	visaRoutesStore := visaroutes.NewStore(database)
	visaRoutesService := visaroutes.NewService(visaRoutesStore, settingsService)
	visaRoutesHandlers := visaroutes.NewHandlers(visaRoutesService, settingsService)

	appointmentAlertStore := appointmentalert.NewStore(database)
	appointmentAlertService := appointmentalert.NewService(appointmentAlertStore, settingsService)
	appointmentAlertService.SetEnqueuer(taskEnqueuer)
	appointmentAlertService.SetNotifier(notificationService)
	appointmentAlertService.SetCache(vkClient.Redis())
	appointmentAlertHandlers := appointmentalert.NewHandlers(appointmentAlertService, settingsService)
	// Register the service as the embassy alerter processor so the
	// embassy:poll / embassy:dispatch_alerts / embassy:expire_checks Asynq
	// tasks are handled (§91, ADR-099).
	worker.RegisterEmbassyAlerterProcessor(appointmentAlertService)

	arrivalChecklistStore := arrivalchecklist.NewStore(database)
	arrivalChecklistService := arrivalchecklist.NewService(arrivalChecklistStore, settingsService)
	arrivalChecklistHandlers := arrivalchecklist.NewHandlers(arrivalChecklistService, settingsService)

	credentialCheckerStore := credentialchecker.NewStore(database)
	credentialCheckerService := credentialchecker.NewService(credentialCheckerStore, settingsService)
	credentialCheckerHandlers := credentialchecker.NewHandlers(credentialCheckerService, settingsService)

	cvFormatterStore := cvformatter.NewStore(database)
	cvFormatterService := cvformatter.NewService(cvFormatterStore, settingsService)
	cvFormatterService.SetRBAC(rbacResolver)
	cvFormatterService.SetEnqueuer(taskEnqueuer)
	cvFormatterService.SetCache(&cvFormatterCache{rdb: vkClient.Redis()})
	cvFormatterHandlers := cvformatter.NewHandlers(cvFormatterService, settingsService)

	housingGuideStore := housingguide.NewStore(database)
	housingGuideService := housingguide.NewService(housingGuideStore, settingsService)
	housingGuideService.SetEnqueuer(taskEnqueuer)
	housingGuideHandlers := housingguide.NewHandlers(housingGuideService, settingsService)

	attestationGuideStore := attestationguide.NewStore(database)
	attestationGuideService := attestationguide.NewService(attestationGuideStore, settingsService)
	attestationGuideHandlers := attestationguide.NewHandlers(attestationGuideService, settingsService)

	languageTestHubStore := languagetesthub.NewStore(database)
	languageTestHubService := languagetesthub.NewService(languageTestHubStore, settingsService)
	languageTestHubHandlers := languagetesthub.NewHandlers(languageTestHubService, settingsService)

	taxCalculatorStore := taxcalculator.NewStore(database)
	taxCalculatorService := taxcalculator.NewService(taxCalculatorStore, settingsService)
	taxCalculatorHandlers := taxcalculator.NewHandlers(taxCalculatorService, settingsService)

	communityLocatorStore := communitylocator.NewStore(database)
	communityLocatorService := communitylocator.NewService(communityLocatorStore, settingsService)
	communityLocatorHandlers := communitylocator.NewHandlers(communityLocatorService, settingsService)

	// Ref: japaconnect-engineering/sections/71-readiness-assessment.md (ADR-071)
	assessmentStore := assessment.NewStore(database)
	assessmentService := assessment.NewService(assessmentStore, settingsService)
	assessmentService.SetEnqueuer(taskEnqueuer)
	assessmentHandlers := assessment.NewHandlers(assessmentService, settingsService)

	// Wire the Readiness Assessment checker into the japa stage service and
	// register the japa_stage:recommend processor (§87, ADR-095). The
	// assessment store satisfies japastage.AssessmentChecker; the service
	// satisfies worker.JapaStageProcessor.
	japaStageService.WithAssessmentChecker(assessmentStore)
	worker.RegisterJapaStageProcessor(japaStageService)

	// Ref: japaconnect-engineering/sections/72-cost-of-living.md (ADR-072)
	costOfLivingStore := costofliving.NewStore(database)
	costOfLivingService := costofliving.NewService(costOfLivingStore, settingsService)
	costOfLivingHandlers := costofliving.NewHandlers(costOfLivingService, settingsService)

	// Ref: japaconnect-engineering/sections/73-scholarship-finder.md (ADR-073)
	scholarshipsStore := scholarships.NewStore(database)
	scholarshipsService := scholarships.NewService(scholarshipsStore, settingsService)
	scholarshipsService.SetEnqueuer(taskEnqueuer)
	scholarshipsHandlers := scholarships.NewHandlers(scholarshipsService)

	// Ref: japaconnect-engineering/sections/74-remittance-tracker.md (ADR-075)
	remittanceStore := remittance.NewStore(database)
	remittanceService := remittance.NewService(remittanceStore, settingsService)
	remittanceService.SetCache(vkClient.Redis())       // VALKEY-004: jc:remit:* corridor rate cache
	remittanceService.SetEnqueuer(taskEnqueuer)        // admin fetch trigger + alert check tasks
	remittanceService.SetNotifier(notificationService) // rate-alert notification dispatch (ADR-075 §4)
	remittanceHandlers := remittance.NewHandlers(remittanceService, settingsService)

	// Ref: japaconnect-engineering/sections/61-self-serve-ads.md (ADR-079)
	adsSelfServeStore := adsselfserve.NewStore(database)
	adsSelfServeService := adsselfserve.NewService(adsSelfServeStore, settingsService)
	adsSelfServeService.SetEnqueuer(taskEnqueuer)
	adsSelfServeService.SetNotifier(adsNotificationAdapter{svc: notificationService})
	adsSelfServeService.SetBalanceChecker(adsBalanceChecker{wallet: walletService, db: database})
	adsSelfServeHandlers := adsselfserve.NewHandlers(adsSelfServeService)

	// Ref: japaconnect-engineering/sections/77-flutter-mobile-app.md (ADR-074)
	// Mobile push device registration + admin push test.
	devicesStore := devices.NewStore(database)
	devicesService := devices.NewService(devicesStore, settingsService)
	// Wire the FCM/APNs CompositeDispatcher. It reads credentials per-tenant
	// from system_settings (hot-reloadable per Rule 3). When credentials are
	// not configured, Dispatch returns a clear error per-token (§77).
	devicesService.SetDispatcher(devices.NewCompositeDispatcher(settingsService))
	devicesHandlers := devices.NewHandlers(devicesService)

	// Register all v2 background-task processors with the worker. Without
	// this call every v2 Asynq task handler logs "no processor registered,
	// skipping" and returns nil — all v2 background work is a silent no-op.
	// Ref: sections 60-78, ADRs 061-086.
	workerv2.RegisterV2Processors(database,
		jobsService,
		gamificationService,
		contentImportService,
		assessmentService,
		scholarshipsService,
		remittanceService,
		referralService,
		adsSelfServeService,
		devicesService,
		notificationService,
	)

	// AI Content Studio (Section 75, ADR-076).
	studioStore := studio.NewStore(database)
	studioService := studio.NewService(studioStore, settingsService)
	studioService.SetEnqueuer(taskEnqueuer)
	studioHandlers := studio.NewHandlers(studioService)
	studioHandlers.SetPersonaService(personasService)
	// The full §75 generation pipeline adapter (persona voice → AI gateway →
	// draft content → HITL review / auto-publish) is registered as the
	// StudioProcessor after aiStore/aiDispatcher are created (see below).

	// Participation Orchestration Engine (Section 76, ADR-077).
	participationService := participation.NewService(database, settingsService, vkClient.Redis())
	participationService.SetEnqueuer(taskEnqueuer)
	participationService.SetEventTracker(analyticsService)        // ADR-068 §4: synthetic event tracking
	personas.SetHighStakesGate(participation.IsHighStakesContent) // BUG-015: single canonical gate
	worker.RegisterPersonaProcessor(participationService)

	// Mentorship subsystem (Section 35, §102 ADR-110).
	mentorshipService := mentorship.NewService(database)
	mentorshipService.SetEnqueuer(taskEnqueuer)
	mentorshipService.SetSettingsProvider(settingsService)
	mentorshipService.SetEventTracker(analyticsEventTrackerAdapter{analyticsService})
	mentorshipHandlers := mentorship.NewHandlers(mentorshipService)

	// Page Builder subsystem (Section 16 — Page Builder admin CRUD).
	pagebuilderService := pagebuilder.NewService(database)
	pagebuilderService.SetRedis(vkClient.Redis())
	pagebuilderHandlers := pagebuilder.NewHandlers(pagebuilderService)

	// Widget configurations subsystem (Section 16 — data-source-driven dynamic
	// blocks with Valkey-cached rendering).
	widgetStore := widgets.NewStore(database)
	widgetRenderer := widgets.NewRenderer(database.Reader)
	widgetService := widgets.NewService(widgetStore, widgetRenderer, vkClient.Redis())
	widgetHandlers := widgets.NewHandlers(widgetService)

	// Wire the widget resolver into the page builder so GetPublishedPage
	// can server-side resolve widget blocks (Section 16: "blocks + resolved
	// widget data").
	pagebuilderHandlers.SetWidgetResolver(widgetService)

	// Register analytics processor with worker for background tasks.
	analyticsAdapter := analytics.NewWorkerAdapter(analyticsService)
	worker.RegisterAnalyticsProcessor(analyticsAdapter)

	// AI Core subsystem (Section 20 — AI Core).
	aiDispatcher := ai.NewDispatcher()

	// §102: wire the AI embedder into the mentorship service for
	// mentor:generate_embedding tasks (ADR-110).
	mentorshipService.SetEmbedder(ai.NewRAGEmbedder(aiDispatcher))

	// Register Xiaomi MiMo as the primary content generation + chat provider.
	// The "chat" category is required by HandleChat (JapaBot) — without it,
	// every /ai/chat request fails with "no providers registered for category
	// \"chat\"". The same Xiaomi provider serves both categories.
	// "moderation" is used by the spam pipeline's AI classification layer
	// (checkAIClassification) for semantic spam detection.
	if cfg.XiaomiAPIKey != "" {
		xiaomiProvider := ai.NewXiaomiProvider(cfg.XiaomiAPIKey, "")
		aiDispatcher.RegisterProvider("content_generation", xiaomiProvider)
		aiDispatcher.RegisterProvider("chat", xiaomiProvider)
		aiDispatcher.RegisterProvider("moderation", xiaomiProvider)
		slog.Info("ai: registered xiaomi provider", "model", ai.XiaomiModelFlash,
			"categories", []string{"content_generation", "chat", "moderation"})
	}

	// Wire the AI classifier into the spam pipeline so the
	// checkAIClassification layer can call the moderation category for
	// semantic spam detection. When no moderation provider is registered,
	// the classifier returns a zero result and the pipeline degrades to
	// rule-only mode.
	spamPipeline.SetClassifier(spam.NewAIClassifier(aiDispatcher))

	aiStore := ai.NewStore(database.Reader, database.Writer)
	intelStores := intel_stores.NewStore(database)

	// Register the full §75 generation pipeline adapter (persona voice →
	// AI gateway → draft content → HITL review / auto-publish) as the
	// StudioProcessor so content:generate_draft, studio:run_batch, and
	// studio:retry_failed run the real pipeline instead of a no-op.
	worker.RegisterStudioProcessor(&studioDraftProcessorAdapter{
		studio:          studioService,
		contentSvc:      contentService,
		aiStore:         aiStore,
		personaStore:    personasStore,
		dispatcher:      aiDispatcher,
		intelStore:      intelStores,
		arrivalSvc:      arrivalChecklistService,
		communityOrgSvc: communityLocatorService,
		attestationSvc:  attestationGuideService,
		languageTestSvc: languageTestHubService,
		taxSvc:          taxCalculatorService,
		credentialSvc:   credentialCheckerService,
	})

	// Wire the intelligence store into the Studio handlers so the
	// create-job endpoint can validate intel_record_id ownership (ADR-076
	// Amendment 1). Without this, the Promote-to-Studio flow accepts
	// intel_record_id values without verifying they belong to the tenant.
	studioHandlers.SetIntelStore(intelStores)

	// §97: wire the AI Gateway dispatcher into the attestation guide so
	// GenerateDraft can populate pathway_steps via the content_generation
	// category (ADR-088). The dispatcher is optional; GenerateDraft falls
	// back to empty steps if it is nil or the call fails.
	attestationGuideService.SetDispatcher(aiDispatcher)
	arrivalChecklistService.SetDispatcher(aiDispatcher)
	languageTestHubService.SetDispatcher(aiDispatcher)
	// §99: wire the Asynq client so the approve handler can enqueue
	// Meilisearch index sync for JapaBot RAG retrieval (ADR-088).
	languageTestHubHandlers.SetAsynqClient(asynqClient)

	aiHandlers := ai.NewHandlers(aiDispatcher, aiStore)
	aiHandlers.SetHonchoClient(honcho.NewClient(cfg.HonchoURL))
	aiHandlers.SetSettings(settingsService) // Rule 3: ai.japabot_enabled global toggle
	aiHandlers.SetRBAC(rbacResolver)        // ADR-025: ai.context.* category node checks
	aiHandlers.AsynqClient = asynqClient

	// Wire the AI dispatcher into the i18n handlers + processor adapter so
	// POST /admin/i18n/strings/{id}/translate and the
	// i18n:translate_language_pack Asynq task actually translate via the AI
	// gateway instead of copying source values verbatim (BE-i18n-001/002/003).
	i18nHandlers.AIDispatcher = aiDispatcher
	i18nTranslateAdapter.SetAIDispatcher(aiDispatcher)

	aiToolRegistry := ai.NewToolRegistry(database)

	aiToolHandlers := ai.NewToolHandlers(aiToolRegistry)

	// Content Embedder — generates vector embeddings via the AI dispatcher
	// and stores them in content_scores.relevance_embedding. Registered with
	// the worker so the ai:generate_content_embedding handler can invoke it.
	contentEmbedder := ai.NewContentEmbedder(aiDispatcher, database.Writer)
	worker.RegisterContentEmbedder(contentEmbedder)

	// RAG Knowledge Layer (Section 69, ADR-069) — chunked embedding storage
	// in content_embeddings (pgvector HNSW) + cosine retrieval at chat time.
	ragStore := rag.NewStore(database)
	ragService := rag.NewService(ragStore, settingsService)
	ragService.SetEmbedder(ai.NewRAGEmbedder(aiDispatcher))
	ragService.SetEnqueuer(taskEnqueuer)
	ragService.SetCache(vkClient.Redis())
	ragHandlers := rag.NewHandlers(ragService, settingsService)
	// Inject the RAG service into the AI handlers so HandleChat can run the
	// retrieval step before the LLM call when rag.enabled is true.
	aiHandlers.SetRAGService(ragService)
	// §19/ADR-014: wire the GDPR consent checker so HandleChat verifies
	// ai_processing consent before sending user prompts to external providers.
	aiHandlers.SetConsentChecker(gdprService)

	// B6/B8: wire Valkey (edit lock) and RAG indexer (embedding on approval)
	// into the wiki service/handlers. These are wired here because ragService
	// is created above; the wiki handlers were created earlier at line ~1028.
	wikiService.SetValkey(vkClient.Redis())
	wikiHandlers.SetRAGIndexer(ragService)
	// BUG-001 fix: wire RAG indexer into the spaces and learning handlers so
	// spaces and learning tracks are embedded into content_embeddings
	// (§69/ADR-069). These handlers were created earlier; ragService is
	// created above.
	spaceHandlers.SetRAGIndexer(ragService)
	learningHandlers.SetRAGIndexer(ragService)
	// BUG-002 (rag-audit): inject the settings reader so the spaces and
	// learning handlers can resolve i18n.default_language for RAG indexing
	// instead of hardcoding "en".
	spaceHandlers.SetSettings(settingsService)
	learningHandlers.SetSettings(settingsService)

	// AI Agent Executor (Section 103, ADR-111) — Phase 6 wiring.
	// The agent handler ties together the gateway function-calling extension
	// (Phase 1), the tool executor (Phase 3), the core tool catalog
	// (Phase 4), and the pre-search + agent loop (Phase 5). The HTTP
	// handlers (Phase 6) expose it via SSE streaming + REST routes.
	agentRAG := &agentRAGAdapter{svc: ragService}
	agentCache := &agentCacheAdapter{rdb: vkClient.Redis()}
	agentRateLimiter := &agentRateLimiterAdapter{rdb: vkClient.Redis()}
	contentCreatorBridge := &agentContentCreatorBridge{svc: contentService}
	contentUpdaterBridge := &agentContentUpdaterBridge{svc: contentService}

	// Phase 9: the users service is needed for the user.* AI tools. It is
	// constructed here (before the agent deps wiring) because the agent
	// deps block runs before the User Profile & Preferences subsystem
	// section below. The full usersService wiring (MediaResolver, wallet
	// points provider, etc.) happens later — only the base service is
	// needed for the AI agent's GetProfile/UpdateProfile calls.
	agentUsersSvc := users.NewService(database)

	agentDeps := ai.NewToolDeps().
		WithContent(contentService).
		WithCustomFields(cfService).
		WithRAG(agentRAG).
		WithComparison(&agentComparisonAdapter{svc: comparisonService}).
		WithPrivacy(ai.NewPrivacyChecker(gen.New(database.Reader))).
		WithSettings(settingsService).
		WithCache(agentCache)
	agentDeps.ContentCreator = contentCreatorBridge.create
	agentDeps.ContentUpdater = contentUpdaterBridge.update
	// Phase 8: persona resolver + HITL store for content.create_draft.
	agentDeps.Persona = &agentPersonaAdapter{store: personasStore}
	agentDeps.HITL = aiStore
	// Phase 9: extended tool catalog (Section 103). Spaces, custom field
	// definitions, and content lifecycle services match the ai package
	// interfaces 1:1 and are wired directly. Learning, docs, menus, and
	// users use mirror types in the ai package to avoid import cycles, so
	// adapters are wired here.
	agentDeps.Spaces = spaceService
	agentDeps.Learning = &agentLearningAdapter{store: learningStore}
	agentDeps.Docs = &agentDocsAdapter{svc: docsService}
	agentDeps.Menus = &agentMenusAdapter{svc: menusService}
	agentDeps.Users = &agentUsersAdapter{svc: agentUsersSvc}
	agentDeps.CustomFieldDefs = cfService
	agentDeps.ContentLifecycle = contentService
	// Phase 9: tools registry for tools.list / tools.read handlers. The
	// adapter queries per-tool tables directly via the reader pool so the
	// ai package does not import each tool package.
	agentDeps.ToolsRegistry = newAgentToolsRegistry(database.Reader)

	agentRegistry := ai.NewToolHandlerRegistry()
	ai.RegisterPhase4Tools(agentRegistry, agentDeps)
	ai.RegisterPhase9Tools(agentRegistry, agentDeps)
	// Load tool configs from the ai_tools table (migration 000388 seed).
	// Errors are non-fatal — the agent degrades to no tools if the table
	// is empty or the configs are unparseable.
	if err := agentRegistry.LoadFromDB(context.Background(), aiToolRegistry, defaultTenantID); err != nil {
		slog.Warn("server: loading agent tool configs from DB", "error", err)
	}

	agentExecutor := ai.NewExecutor(agentRegistry,
		ai.WithPrivacyChecker(ai.NewPrivacyChecker(gen.New(database.Reader))),
		ai.WithPermissionChecker(rbacResolver),
		ai.WithStore(aiStore),
		ai.WithWriteQueries(gen.New(database.Writer)),
	)

	agentPreSearcher := ai.NewPreSearcher(agentDeps)

	agentHandler := ai.NewAgentHandler(aiDispatcher, agentExecutor, agentRegistry, agentPreSearcher,
		ai.WithAgentPerms(rbacResolver),
		ai.WithAgentSettings(settingsService),
		ai.WithAgentSessions(gen.New(database.Writer)),
		ai.WithAgentGeoResolver(gen.New(database.Reader)),
		ai.WithAgentRateLimiter(agentRateLimiter),
		ai.WithAgentCache(agentCache),
		ai.WithAgentPersona(&agentPersonaAdapter{store: personasStore}),
	)

	agentHTTPHandlers := ai.NewAgentHTTPHandlers(agentHandler, gen.New(database.Reader),
		ai.WithAgentRegistry(agentRegistry),
		ai.WithAgentHTTPPerms(rbacResolver),
		ai.WithAgentContentStore(gen.New(database.Writer)),
		ai.WithAgentHTTPCache(agentCache),
		ai.WithAgentComparisonEngine(ai.NewComparisonEngine(agentDeps)),
		ai.WithAgentPreSearcher(agentPreSearcher),
	)

	// Inject the AI dispatcher + RAG retriever into the assessment service
	// for AI-assisted roadmap generation (ADR-071 §3). Gated by
	// assessment.ai_roadmap_enabled at runtime; falls back to curated-only.
	assessmentService.SetAIDispatcher(&assessmentAIDispatcher{dispatcher: aiDispatcher})
	assessmentService.SetRAGRetriever(&assessmentRAGRetriever{svc: ragService})
	// Register the RAG indexer with the worker so content:generate_embedding
	// also chunks and upserts into content_embeddings (Section 69, ADR-069).
	worker.RegisterRAGIndexer(ragService)

	// Cache flusher for admin-only cache invalidation.
	cacheFlusher := analytics.NewCacheFlusher(vkClient)
	analyticsHandlers.SetCacheFlusher(cacheFlusher)
	analyticsHandlers.SetRBAC(rbacResolver)      // BUG-ANALYTICS-03: inline owner-or-admin check for /content/{id}/analytics
	analyticsHandlers.SetCache(vkClient.Redis()) // BUG-010: jc:cache:analytics:* read-through cache

	// Register AI and moderation processor adapters so the ai:process_task
	// and content:scan_moderation background handlers use real implementations
	// instead of noops. Must come before RegisterNoopProcessors() so the noop
	// guard (if nil → register noop) sees the real adapters.
	aiProcessorAdapter := ai.NewWorkerAdapter(aiDispatcher)
	aiProcessorAdapter.SetSettings(settingsService) // honor ai.humanize.* for background generation
	worker.RegisterAIProcessor(aiProcessorAdapter)

	// Phase 8: register the agent draft processor so the
	// agent:create_draft_request Asynq task creates draft content + a HITL
	// review with source='agent_draft_request'. The adapter closes over the
	// content service + AI store + persona store + AI dispatcher so the
	// worker package does not import internal/ai or internal/content.
	worker.RegisterAgentDraftProcessor(&agentDraftProcessorAdapter{
		contentSvc:   contentService,
		aiStore:      aiStore,
		personaStore: personasStore,
		dispatcher:   &agentDraftDispatcherAdapter{dispatcher: aiDispatcher},
	})

	// §95 CV Formatter — wire the AI dispatcher into the service and
	// register the service as the cv_format:process / cv_format_sessions:purge
	// Asynq task processor (ADR-103).
	cvFormatterService.SetAIDispatcher(&cvFormatterAIDispatcher{dispatcher: aiDispatcher})
	worker.RegisterCvFormatterProcessor(cvFormatterService)

	// Wire the AI Gateway dispatcher into the credential checker service
	// so GeneratePathwayDraft can call the Content Agent (§20, ADR-088).
	credentialCheckerService.SetAIDispatcher(&credentialCheckerAIDispatcher{dispatcher: aiDispatcher})

	moderationProcessorAdapter := spam.NewWorkerAdapter(spamPipeline)
	worker.RegisterModerationProcessor(moderationProcessorAdapter)

	// Register no-op implementations for any remaining processor interface
	// that hasn't been wired to a real service yet. This ensures all Asynq
	// task handlers degrade gracefully instead of crashing on a nil interface.
	worker.RegisterNoopProcessors()

	// Intelligence System (Section INTEL) — construct the spine stores,
	// services, and HTTP handlers, then register the Asynq worker
	// dependencies (including the extractor + promoter port adapters that
	// break the import cycle documented in internal/worker/intelligence.go).
	intelSourcesStore := sources.NewStore(database)
	intelSourcesService := sources.NewService(intelSourcesStore)
	intelScheduler := scheduler.NewScheduler(&intelSchedulerSourceAdapter{store: intelSourcesStore, tenantID: defaultTenantID}, vkClient.Redis(), defaultTenantID)
	intelDedupService := dedup.NewService(intelStores, defaultTenantID)
	intelEventPublisher := events.NewAsynqEventPublisher(asynqClient)

	// Extractor port adapter: wraps *extractor.Extractor so the worker
	// package can call Extract without importing internal/ai (which would
	// close the extractor → ai → auth → worker cycle). The adapter looks
	// up the entity schema by entityType from the schemas registry; the
	// worker derives entityType from the source's category.
	// Read the AI extraction model from system_settings (Rule 3, INTEL-007).
	// This model is stamped on every AI-generated content row created by the
	// promoters (ADR-088, INTEL-003).
	intelExtractionModel := settingsService.GetStringOr(context.Background(), defaultTenantID, "intel.extraction_model", "gpt-4o-mini")
	// Read max retries from system_settings (Rule 3, INTEL-012 fix).
	intelExtractionMaxRetries := settingsService.GetIntOr(context.Background(), defaultTenantID, "intel.extraction_max_retries", 3)

	intelExtractor := extractor.NewExtractor(aiDispatcher,
		extractor.WithTenantID(defaultTenantID),
		extractor.WithModel(intelExtractionModel),
		extractor.WithMaxRetries(intelExtractionMaxRetries),
	)
	intelExtractorAdapter := &intelExtractorPortAdapter{ext: intelExtractor}

	// Resolve the fallback author ID for intelligence promoters. The
	// intel.default_persona_id system setting points to an AI persona whose
	// backing bot user (is_bot=true) is stamped as contents.author_id when a
	// source has no explicit persona_id. Falls back to the super admin user
	// if the setting is missing or the persona lookup fails — the fallback
	// must be a valid users(id) row to avoid the FK violation on
	// contents.author_id (BUG: the prior code passed defaultTenantID, which
	// is a tenant UUID, not a user UUID).
	intelDefaultAuthorID := defaultTenantID // worst-case fallback (fixed below)
	if personaIDStr := settingsService.GetStringOr(context.Background(), defaultTenantID, "intel.default_persona_id", ""); personaIDStr != "" {
		var personaID pgtype.UUID
		if err := personaID.Scan(personaIDStr); err == nil && personaID.Valid {
			if userID, err := intelStores.GetPersonaUserID(context.Background(), defaultTenantID, personaID); err == nil && userID.Valid {
				intelDefaultAuthorID = userID
			} else {
				slog.Warn("server: intel.default_persona_id resolve failed, falling back to super admin", "error", err)
			}
		}
	}
	if intelDefaultAuthorID == defaultTenantID {
		// Final fallback: use the super admin user (seeded by migration 000124).
		if sysUID, err := lookupSystemUserID(context.Background(), database); err == nil && sysUID.Valid {
			intelDefaultAuthorID = sysUID
		} else {
			slog.Error("server: could not resolve super admin user for intel promoter fallback", "error", err)
		}
	}

	// Promoter registry: one adapter per domain, keyed by entity_type.
	// The worker's HandleIntelPromote dispatches to the correct promoter
	// by the record's entity_type. Each domain has its own store adapter
	// because the WithTx callback type differs per domain (the TxStore
	// interface is domain-specific). The underlying *stores.Store is
	// shared; only the WithTx bridge differs.
	intelPromoters := map[string]worker.IntelPromoterPort{
		schemas.EntityTypeScholarship: &intelPromoterPortAdapter{
			prom: promoter.NewScholarshipPromoter(&intelPromoterStoreAdapter{Store: intelStores}, defaultTenantID, intelDefaultAuthorID, promoter.WithAIModel(intelExtractionModel)),
		},
		schemas.EntityTypeJob: &intelPromoterPortAdapter{
			prom: promoter.NewJobPromoter(&intelJobPromoterStoreAdapter{Store: intelStores}, defaultTenantID, intelDefaultAuthorID, promoter.WithJobAIModel(intelExtractionModel)),
		},
		schemas.EntityTypeVisa: &intelPromoterPortAdapter{
			prom: promoter.NewVisaPromoter(&intelVisaPromoterStoreAdapter{Store: intelStores}, defaultTenantID, intelDefaultAuthorID, promoter.WithVisaAIModel(intelExtractionModel)),
		},
		schemas.EntityTypeHousing: &intelPromoterPortAdapter{
			prom: promoter.NewHousingPromoter(&intelHousingPromoterStoreAdapter{Store: intelStores}, defaultTenantID, intelDefaultAuthorID, promoter.WithHousingAIModel(intelExtractionModel)),
		},
		schemas.EntityTypeRemittance: &intelPromoterPortAdapter{
			prom: promoter.NewRemittancePromoter(&intelRemittancePromoterStoreAdapter{Store: intelStores}, defaultTenantID, intelDefaultAuthorID),
		},
		schemas.EntityTypeCostOfLiving: &intelPromoterPortAdapter{
			prom: promoter.NewCostOfLivingPromoter(&intelCostOfLivingPromoterStoreAdapter{Store: intelStores}, defaultTenantID, intelDefaultAuthorID),
		},
		// 7 pure guide content type domains — each creates a contents
		// guide row with the appropriate guide_category. They share the
		// GuideContentPromoter implementation with different entityType
		// + guideCategory parameters.
		schemas.EntityTypeTravelRoutes: &intelPromoterPortAdapter{
			prom: promoter.NewGuideContentPromoter(&intelGuidePromoterStoreAdapter{Store: intelStores}, defaultTenantID, intelDefaultAuthorID, "travel_routes", "travel_routes", promoter.WithGuideContentAIModel(intelExtractionModel)),
		},
		schemas.EntityTypeGrants: &intelPromoterPortAdapter{
			prom: promoter.NewGuideContentPromoter(&intelGuidePromoterStoreAdapter{Store: intelStores}, defaultTenantID, intelDefaultAuthorID, "grants", "grants", promoter.WithGuideContentAIModel(intelExtractionModel)),
		},
		schemas.EntityTypeAdmission: &intelPromoterPortAdapter{
			prom: promoter.NewGuideContentPromoter(&intelGuidePromoterStoreAdapter{Store: intelStores}, defaultTenantID, intelDefaultAuthorID, "admission", "admission", promoter.WithGuideContentAIModel(intelExtractionModel)),
		},
		schemas.EntityTypeBanking: &intelPromoterPortAdapter{
			prom: promoter.NewGuideContentPromoter(&intelGuidePromoterStoreAdapter{Store: intelStores}, defaultTenantID, intelDefaultAuthorID, "banking", "banking", promoter.WithGuideContentAIModel(intelExtractionModel)),
		},
		schemas.EntityTypeWorkCareer: &intelPromoterPortAdapter{
			prom: promoter.NewGuideContentPromoter(&intelGuidePromoterStoreAdapter{Store: intelStores}, defaultTenantID, intelDefaultAuthorID, "work_career", "work_career", promoter.WithGuideContentAIModel(intelExtractionModel)),
		},
		schemas.EntityTypeHealthInsurance: &intelPromoterPortAdapter{
			prom: promoter.NewGuideContentPromoter(&intelGuidePromoterStoreAdapter{Store: intelStores}, defaultTenantID, intelDefaultAuthorID, "health_insurance", "health_insurance", promoter.WithGuideContentAIModel(intelExtractionModel)),
		},
		schemas.EntityTypeCultureLifestyle: &intelPromoterPortAdapter{
			prom: promoter.NewGuideContentPromoter(&intelGuidePromoterStoreAdapter{Store: intelStores}, defaultTenantID, intelDefaultAuthorID, "culture_lifestyle", "culture_lifestyle", promoter.WithGuideContentAIModel(intelExtractionModel)),
		},
		schemas.EntityTypeWikiPage: &intelPromoterPortAdapter{
			prom: promoter.NewWikiPromoter(&intelWikiPromoterStoreAdapter{Store: intelStores}, defaultTenantID, intelDefaultAuthorID, promoter.WithWikiAIModel(intelExtractionModel)),
		},
		schemas.EntityTypeLanguageTest: &intelPromoterPortAdapter{
			prom: promoter.NewLanguageTestPromoter(&intelLanguageTestPromoterStoreAdapter{Store: intelStores}, defaultTenantID, intelDefaultAuthorID, promoter.WithLanguageTestAIModel(intelExtractionModel)),
		},
		schemas.EntityTypeTaxObligation: &intelPromoterPortAdapter{
			prom: promoter.NewTaxPromoter(&intelTaxPromoterStoreAdapter{Store: intelStores}, defaultTenantID, intelDefaultAuthorID, promoter.WithTaxAIModel(intelExtractionModel)),
		},
		schemas.EntityTypeCommunityOrganisation: &intelPromoterPortAdapter{
			prom: promoter.NewCommunityOrgPromoter(&intelCommunityOrgPromoterStoreAdapter{Store: intelStores}, defaultTenantID, intelDefaultAuthorID, promoter.WithCommunityOrgAIModel(intelExtractionModel)),
		},
	}

	// Register the intelligence spine dependencies with the worker package
	// so the 11 Asynq task handlers (schedule_crawls → notify_discoveries)
	// can dispatch to the concrete services.
	intelRobotsChecker := intel_fetcher.NewRobotsChecker(vkClient.Redis())
	intelPreflightChecker := intel_fetcher.NewPreflightChecker(vkClient.Redis(), intelRobotsChecker, settingsService, defaultTenantID)
	worker.RegisterIntelDependencies(
		intelSourcesStore,
		intelStores,
		intelScheduler,
		intelExtractorAdapter,
		intelDedupService,
		intelPromoters,
		intelEventPublisher,
		intel_fetcher.NewComposite(), // composite fetcher: dispatches by crawl_method (BUG-008 fix)
		intelPreflightChecker,        // pre-flight: crawl lock, rate limit, robots.txt (INTEL-001/006 fix)
		asynqClient,
	)

	// ADR-076 Amendment 1: wire the Studio auto-promote port so
	// HandleIntelPromote can auto-create Studio jobs for high-confidence
	// extracted records when studio.auto_promote_intel_records is true.
	worker.RegisterIntelStudioAutoPromoter(&studioAutoPromoterAdapter{
		svc:      studioService,
		settings: settingsService,
		authorID: intelDefaultAuthorID,
	})

	// HTTP handlers for the /api/v1/intel/* routes. The radar reuses the
	// existing follows service (entity_type "space") and scholarships
	// service (saved-scholarship tracker) so no parallel tables are needed.
	intelHandlers := intel_handlers.NewHandlers(intelStores, intelSourcesService, followService, scholarshipsService, asynqClient, settingsService, defaultTenantID)

	// Admin Impersonation subsystem (Section 46).
	impersonationStore := impersonation.NewStore(vkClient.Redis())
	impersonationService := impersonation.NewService(impersonationStore, database.Writer)
	impersonationService.SetRBAC(rbacResolver) // SEC-009: no-admin-on-admin safety check (Section 46, ADR-047)
	impersonationHandlers := impersonation.NewHandlers(impersonationService)

	// Public API & Developer Portal subsystem (Section 47).
	publicapi.SetSiteURL(cfg.SiteURL)
	publicAPIStore := publicapi.NewStore(database.Writer)
	publicAPIService := publicapi.NewService(publicAPIStore)
	publicAPIService.SetKeyCountReader(settingsService) // api.keys.max_per_user (Rule 3)
	publicAPIHandlers := publicapi.NewHandlers(publicAPIService)
	publicAPIHandlers.SetQueries(gen.New(database.Reader))

	// User Profile & Preferences subsystem.
	usersService := users.NewService(database)
	// Wire the media resolver so PATCH /me/profile can accept
	// avatar_media_id/header_media_id and resolve them to URLs (BUG-004).
	usersService.MediaResolver = &mediaURLResolver{media: mediaService}
	// Wire the wallet service as the PointsProvider for the users service so
	// public profiles can conditionally display a user's Points balance when
	// they have opted in via show_points_publicly (Section 28). Credits are
	// never exposed publicly.
	usersService.PointsProvider = walletService
	// Wire the Asynq enqueuer so privacy settings changes (search_indexable
	// opt-out) trigger a search:index_entity task to sync the user's
	// document in the Meilisearch users index (§13/§19).
	usersService.SetEnqueuer(taskEnqueuer)
	usersHandlers := users.NewHandlers(usersService, contentService)
	// Wire the RBAC resolver so GetMyContent can fall back to a
	// tenant-scoped (non-author-scoped) fetch for super-admins and users
	// with the `content.edit` permission node.
	usersHandlers.SetPermissionResolver(rbacResolver)
	// Wire the RBAC permission lister so GetMyProfile surfaces the caller's
	// effective permission nodes on /me (§26 RBAC wiring). Frontend pages
	// gate UI on the actual node (e.g. `forum.ama.create`) instead of
	// hardcoded role names (Rule 16).
	usersHandlers.SetPermissionLister(rbacResolver)
	// ADR-064a: wire settings for configurable Nominatim URL/timeout
	// (Rule 3: no hardcoded URLs or limits).
	usersHandlers.SetSettings(settingsService)
	// Wire auth settings so ChangePassword enforces the tenant's password
	// policy (auth.password.* keys) from system_settings instead of
	// hardcoding validation rules (Rule 3).
	usersHandlers.SetAuthSettings(settingsService)

	// Wire the JWT key rotation so ChangePassword can revoke the caller's
	// current access token JTI after a successful password change.
	usersHandlers.SetKeyRotation(kr)

	// BUG-003: wire the "verified" axis checker for CanPostInSpace. The
	// "verified" posting_perm requires email_verified_at IS NOT NULL.
	spacePostChecker.VerifiedChecker = func(ctx context.Context, userID, tenantID pgtype.UUID) (bool, error) {
		verifiedAt, err := gen.New(database.Reader).GetUserEmailVerifiedStatus(ctx, gen.GetUserEmailVerifiedStatusParams{
			UserID:   userID,
			TenantID: tenantID,
		})
		if err != nil {
			return false, err
		}
		return verifiedAt.Valid, nil
	}

	// Meilisearch client for search (graceful if unreachable — NewClient
	// logs a warning and returns a usable client so the server boots even
	// when the search engine is down).
	meiliClient, err := search.NewClient(cfg.MeilisearchURL, cfg.MeilisearchKey)
	if err != nil {
		slog.Warn("meilisearch client not created", "error", err)
	}

	// Wire Meilisearch into the content service for ?q= search offload
	// (ADR-004 amendment performance tuning — docs/PERFORMANCE_AUDIT.md §4).
	// When wired, list-page ?q= queries use Meilisearch (typo tolerance,
	// ranking) instead of PostgreSQL ILIKE. Falls back to ILIKE if nil.
	if meiliClient != nil {
		contentService.SetSearcher(search.NewContentSearchAdapter(meiliClient))
	}

	// Search subsystem.
	// The indexer syncs content rows into the posts index from the reader
	// pool; it is registered with the worker so the search_index_sync task
	// can upsert documents asynchronously.
	searchIndexer := search.NewIndexer(meiliClient, gen.New(database.Reader))
	searchIndexer.SetPool(database)
	worker.RegisterSearchIndexer(searchIndexer)

	// §102: wire the Meilisearch indexer into the mentorship service and
	// register it as the MentorProcessor so mentor:generate_embedding and
	// mentor:update_meilisearch tasks are handled (ADR-110).
	mentorshipService.SetMeilisearchIndexer(searchIndexer)
	worker.RegisterMentorProcessor(mentorshipService)

	searchHandlers := search.NewHandlers(meiliClient)
	searchHandlers.SetQueries(gen.New(database.Reader))
	searchHandlers.SetSettings(settingsService)
	searchHandlers.SetCache(vkClient.Redis()) // BUG-010: jc:cache:search:* read-through cache

	// Onboarding subsystem (Section 09).
	onboardingService := onboarding.NewService(database, settingsService)
	onboardingHandlers := onboarding.NewHandlers(onboardingService)

	// SEO & Performance subsystem (Section 26).
	seoQueries := gen.New(database.Writer)
	sitemapHandler := seo.NewSitemapHandler(seoQueries, database.Reader, settingsService, vkClient, cfg.SiteURL, cfg.ImgproxyBaseURL)
	ogImageHandler := seo.NewOGImageHandler(database.Reader, vkClient, cfg.SiteName)

	// Config handlers for public settings endpoint.
	configHandlers := config.NewHandlers(settingsService)

	// Control panel handlers (super-admin).
	controlService := control.NewService(database)
	controlHandlers := control.NewHandlers(controlService, database, featureChecker, spamStore, publicAPIHandlers)
	// Wire Asynq inspector for queue monitoring (BE-040).
	workerCfg := worker.DefaultConfig()
	inspector := asynq.NewInspector(asynq.RedisClientOpt{
		Addr:     workerCfg.RedisAddr,
		Password: workerCfg.RedisPassword,
		DB:       workerCfg.RedisDB,
	})
	controlHandlers.SetInspector(inspector)

	controlClient := asynq.NewClient(asynq.RedisClientOpt{
		Addr:     workerCfg.RedisAddr,
		Password: workerCfg.RedisPassword,
		DB:       workerCfg.RedisDB,
	})
	controlHandlers.SetClient(controlClient)
	controlHandlers.SetRBAC(rbacResolver)
	controlHandlers.SetSettings(settingsService)
	controlHandlers.SetValkey(vkClient.Redis())
	controlHandlers.SetMeiliSearch(meiliClient)
	controlHandlers.SetHoncho(honcho.NewClient(cfg.HonchoURL)) // ADR-027: infra health
	controlHandlers.SetAIStore(aiStore)                        // AI budget dashboard aggregation

	// AI Provider management — DB-backed provider configs with hot-reload.
	// The ProviderStore reads/writes the ai_providers table (migration 000269);
	// the AIDispatcher is injected so create/update/delete operations can
	// re-register providers at runtime without a server restart.
	providerStore := ai.NewProviderStore(database.Reader, database.Writer)
	controlHandlers.SetProviderStore(providerStore)
	controlHandlers.SetAIDispatcher(aiDispatcher)

	// Load DB-backed provider configs and register them with the dispatcher.
	// This runs AFTER env-var providers are registered above, so DB configs
	// take priority (LoadAndRegisterProviders clears and re-registers
	// categories that have DB-backed providers). The default tenant is used
	// for the initial load; per-tenant reload happens on config changes.
	if defaultTenant, err := tenantStore.GetDefaultTenant(context.Background()); err == nil && defaultTenant != nil {
		if err := ai.LoadAndRegisterProviders(context.Background(), providerStore, aiDispatcher, defaultTenant.ID); err != nil {
			slog.Warn("ai: loading db-backed providers at startup", "error", err)
		}
	}

	// Infrastructure routes bypass tenant middleware
	r.Get("/health", h.HealthCheck)
	r.Get("/ready", h.ReadinessCheck)

	// Public bot-facing endpoints (robots.txt and llms.txt)
	// These require tenant resolution but no authentication.
	r.Group(func(r chi.Router) {
		r.Use(tenant.SingleTenantMiddleware(defaultTenant))
		r.Get("/robots.txt", bots.RobotsHandler(botStore, vkClient))
		r.Get("/llms.txt", bots.LLMsHandler(botStore, vkClient, cfg.SiteName, cfg.SiteDescription))
		r.Get("/llms-full.txt", bots.LLMsHandler(botStore, vkClient, cfg.SiteName, cfg.SiteDescription))
		r.Get("/sitemap.xml", sitemapHandler.ServeSitemap)
		r.Get("/sitemap-content.xml", sitemapHandler.ServeContentSitemap)
		r.Get("/sitemap-spaces.xml", sitemapHandler.ServeSpacesSitemap)
		r.Get("/sitemap-tags.xml", sitemapHandler.ServeTagsSitemap)
		r.Get("/sitemap-users.xml", sitemapHandler.ServeUsersSitemap)
		r.Get("/sitemap-images.xml", sitemapHandler.ServeImagesSitemap)
		r.Get("/og/{id}.png", ogImageHandler.ServeOGImage)
		// Universal RSS feeds (public — no auth required, tenant-scoped)
		rssHandlers := rsspkg.NewHandlers(gen.New(database.Reader), cfg.SiteURL, cfg.SiteName)
		rssHandlers.SetRedis(vkClient.Redis())
		r.Get("/feed.xml", rssHandlers.HandleFeed)
		r.Get("/spaces/{slug}/feed.xml", rssHandlers.HandleSpaceFeed)
		r.Get("/tags/{slug}/feed.xml", rssHandlers.HandleTagFeed)
		r.Get("/users/{username}/feed.xml", rssHandlers.HandleUserFeed)

		// Public settings endpoint (no auth required, tenant-scoped)
		r.Get("/site/settings/public", configHandlers.GetPublicSettings)

		// SeaweedFS upload fallback — dev-only presigned URL PUT handler.
		// When S3/SeaweedFS is not configured, InitiateUpload returns a fallback
		// presigned URL at /seaweedfs/upload/... which the client PUTs to. This
		// handler writes the raw bytes to the local filesystem so ServeMedia can
		// serve them back. BUG-007: gated behind auth.RequireAuth so an
		// unauthenticated attacker cannot write arbitrary files into any bucket
		// directory. The client already holds a session from InitiateUpload.
		r.With(auth.RequireAuth(kr, vkClient.Redis())).Put("/seaweedfs/upload/{bucket}/*", mediaHandlers.SeaweedFSUpload)

		// API Playground & OpenAPI Spec (Section 44 — docs subdomain)
		r.Get("/docs/api/playground", docsHandlers.APIPlaygroundHandler)
		// catalog: GET /docs/openapi.json (single canonical spec, §6.4 dedupe)
		r.Get("/docs/openapi.json", docsHandlers.OpenAPIHandler)
	})

	// API routes go through tenant resolution
	r.Group(func(r chi.Router) {
		r.Use(tenant.SingleTenantMiddleware(defaultTenant))
		r.Route("/api/v1", func(r chi.Router) {
			// Baseline rate limit for all API routes (ADR-022).
			// Tighter tiers are applied to specific subgroups below.
			r.Use(mw.RateLimit(vkClient.Redis(), "loose"))

			// Ban enforcement middleware (BE-035) — checks Valkey cache then
			// falls back to DB lookup for active bans on user_moderation_actions.
			r.Use(mw.BanEnforcement(vkClient.Redis(), func(ctx context.Context, userID string) (bool, *time.Time) {
				var banned bool

				var expiresAt *time.Time

				err := database.Reader.QueryRow(ctx,
					`SELECT EXISTS(
						SELECT 1 FROM user_moderation_actions
						WHERE user_id = $1 AND action IN ('ban', 'suspension')
						AND deleted_at IS NULL
						AND (expires_at IS NULL OR expires_at > now())
					)`, userID).Scan(&banned)
				if err != nil || !banned {
					return false, nil
				}

				_ = database.Reader.QueryRow(ctx,
					`SELECT expires_at FROM user_moderation_actions
					WHERE user_id = $1 AND action IN ('ban', 'suspension')
					AND deleted_at IS NULL
					AND (expires_at IS NULL OR expires_at > now())
					ORDER BY created_at DESC LIMIT 1`, userID).Scan(&expiresAt)

				return true, expiresAt
			}))

			// Operation-level ban enforcement — checks user_operation_bans
			// for per-operation restrictions (post, comment, discussion, blog, message).
			r.Use(mw.OperationBanEnforcement(vkClient.Redis(), func(ctx context.Context, userID string, op string) (bool, *time.Time) {
				var restricted bool
				var expiresAt *time.Time

				err := database.Reader.QueryRow(ctx,
					`SELECT EXISTS(
						SELECT 1 FROM user_operation_bans
						WHERE user_id = $1 AND operation = $2 AND deleted_at IS NULL
						AND (expires_at IS NULL OR expires_at > now())
					)`, userID, op).Scan(&restricted)
				if err != nil || !restricted {
					return false, nil
				}

				_ = database.Reader.QueryRow(ctx,
					`SELECT expires_at FROM user_operation_bans
					WHERE user_id = $1 AND operation = $2 AND deleted_at IS NULL
					AND (expires_at IS NULL OR expires_at > now())
					ORDER BY created_at DESC LIMIT 1`, userID, op).Scan(&expiresAt)

				return true, expiresAt
			}))

			// Bot detection middleware — identifies known bots from User-Agent
			// and stores bot info in request context for downstream handlers.
			// Runs after tenant middleware (needs tenant context) but before
			// route handlers.
			r.Use(bots.BotDetectionMiddleware(botStore, vkClient))

			// CSRF double-submit enforcement for all mutating /api/v1 requests
			// (Bug Hunter §5). Cookie-authenticated browser requests (the admin
			// SPA) are protected; non-cookie auth (Authorization: Bearer,
			// X-API-Key) is exempt — those clients are CSRF-immune and would
			// otherwise break. The public /auth/* subtree (login, register,
			// password reset) is also exempt: those endpoints are reached before
			// any token cookie exists and are not state-changing on behalf of an
			// already-authenticated session. Public form submissions
			// (/forms/{id}/submissions) and payment provider webhooks
			// (/payments/webhook/*) are exempt: the former accepts anonymous
			// submissions, the latter authenticates via webhook signatures, not
			// cookies. Admin form CRUD (PATCH/DELETE /forms/{id}) is NOT exempt.
			// Safe GET/OPTIONS pass through.
			//
			// WithOriginCheck adds Origin header validation as defense-in-depth:
			// a cross-site request carries a foreign Origin that is rejected
			// even if the double-submit cookie was forged via subdomain
			// injection. Same-origin SPA requests and exempt API clients
			// (no Origin header) are unaffected.
			r.Use(csrf.Middleware([]string{"/"}, func(r *http.Request) bool {
				return csrf.IsNonCookieAuthenticated(r) ||
					strings.HasPrefix(r.URL.Path, "/api/v1/auth/") ||
					(strings.HasPrefix(r.URL.Path, "/api/v1/forms/") && strings.HasSuffix(r.URL.Path, "/submissions")) ||
					strings.HasPrefix(r.URL.Path, "/api/v1/payments/webhook/")
			}, csrf.WithOriginCheck(mw.IsOriginAllowed)))

			// TOTP-for-admin enforcement (SEC-006) — when the
			// "security.totp_required_for_admin" setting is enabled, admin-level
			// users (per rbac.HasAdminCapability) without an active TOTP
			// enrollment are rejected with 403 totp_required on /api/v1/admin/*.
			r.Use(auth.RequireTOTPForAdmin(settingsService, rbacResolver, totpStore))

			// Soft auth (ADR-064) — populates the authenticated-user context for
			// every /api/v1 request when a valid Bearer token is present, without
			// rejecting guests. This lets the VisitorContext resolver (below) read
			// the user's content_country_preference on public+authenticated routes
			// without each route needing its own OptionalAuth wrapper. Per-route
			// RequireAuth guards still enforce 401 on protected endpoints.
			r.Use(auth.OptionalAuth(kr, vkClient.Redis()))

			// Impersonation middleware (SEC-002) — injects impersonated identity
			// into request context when an active impersonation session exists.
			// Must run AFTER OptionalAuth so the admin's identity is populated in
			// context before the Valkey lookup. Skips /api/v1/admin/ routes so the
			// admin can still manage impersonation sessions with their real identity
			// (Section 46: "wraps all non-admin routes").
			r.Use(impersonation.ImpersonationMiddleware(impersonationStore))

			// VisitorContext resolver (ADR-064) — resolves the effective
			// content country for geo-aware feed/search personalization. Runs
			// after GeoIP (line 105), tenant, and soft-auth so it can use all
			// three sources. Priority: ?geo= param → user preference → GeoIP →
			// "NG" default. Cached in Valkey at jc:visitor:ctx:{session_id} (30m
			// TTL, falls back to jc:visitor:ctx:user:{user_id} for legacy tokens
			// without a JTI, or jc:visitor:ctx:ip:{ip} for anonymous visitors).
			// Honors the geo_personalization.enabled killswitch.
			visitorResolver := visitor.NewResolver(gen.New(database.Reader), vkClient.Redis(), settingsService)
			r.Use(visitorResolver.Middleware)

			// CSRF token bootstrap endpoint — issues the double-submit cookie and
			// echoes the token so the SPA can attach it as X-CSRF-Token. The SPA
			// calls this once at startup and again after a 403 CSRF rejection.
			r.Get("/csrf", csrf.TokenHandler(cfg.CookieDomain, cfg.IsProduction()))

			// Public site settings (appearance/branding) — no auth required.
			// Also registered outside /api/v1 at /site/settings/public for bot/SSR use.
			r.Get("/site/settings/public", configHandlers.GetPublicSettings)

			// Auth routes (public - no auth required, tenant-scoped)
			// Tight rate limit to throttle brute-force attempts.
			r.Route("/auth", func(r chi.Router) {
				r.Use(mw.RateLimit(vkClient.Redis(), "tight"))
				// Per §41, a "registration" IP block only blocks the registration
				// endpoint, not all requests (BUG-003).
				r.With(spam.RequireNotIPBlocked(spam.BlockTypeRegistration)).Post("/register", authHandlers.Register)
				r.Post("/login", authHandlers.Login)
				r.Post("/refresh", authHandlers.Refresh)
				r.Post("/logout", authHandlers.Logout)

				// TOTP 2FA challenge verification & backup code (AUDIT-BE-001)
				// These are unauthenticated — the challenge token itself proves
				// the user completed password auth.
				r.Post("/totp/verify-challenge", authHandlers.VerifyLoginChallenge)
				r.Post("/totp/backup", authHandlers.ConsumeBackupCode)

				// Password reset flow
				r.Post("/forgot-password", authHandlers.HandleForgotPassword)
				r.Get("/reset-password", authHandlers.HandleResetPasswordPage)
				r.Post("/reset-password", authHandlers.HandleResetPassword)

				// Email verification
				r.Get("/verify-email", authHandlers.HandleVerifyEmail)
				r.Post("/resend-verification", authHandlers.HandleResendVerification)

				// OAuth social login routes.
				// POST returns auth_url as JSON for frontend-driven redirect.
				r.Post("/oauth/{provider}/initiate", oauthHandlers.HandleInitiate)
				r.Get("/oauth/{provider}/callback", oauthHandlers.HandleCallback)
			})

			// Feature flags endpoint (public - frontend needs flags to
			// render navigation, no auth required)
			r.Get("/feature-flags", featureHandlers.HandleGetFeatureFlags)

			// Feature flags admin toggle (requires auth + RBAC permission)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.feature_flags.manage")).
				Patch("/admin/feature-flags/{flag_name}", featureHandlers.HandleSetFeatureFlag)

			// Admin Reviews list — all reviews for moderation (gated by reviews.moderate).
			// Registered outside the ratings_reviews feature gate so admins can always
			// reach the moderation queue regardless of the feature flag.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "reviews.moderate")).
				Get("/admin/reviews", reviewHandlers.ListAllReviews)

			// Admin Feature Flags management — full CRUD for the admin SPA, all
			// gated by admin.feature_flags.manage. The public GET /feature-flags
			// returns a map; GET /admin/feature-flags returns the array form.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.feature_flags.manage")).
				Get("/admin/feature-flags", featureHandlers.ListFeatureFlags)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.feature_flags.manage")).
				Post("/admin/feature-flags", featureHandlers.CreateFeatureFlag)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.feature_flags.manage")).
				Put("/admin/feature-flags/{flag_name}", featureHandlers.UpdateFeatureFlag)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.feature_flags.manage")).
				Delete("/admin/feature-flags/{flag_name}", featureHandlers.DeleteFeatureFlag)

			// Custom field group routes
			// Read (require auth)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/custom-field-groups", cfHandlers.ListFieldGroups)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/custom-field-groups/{id}", cfHandlers.GetFieldGroup)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/custom-field-groups/{id}/fields", cfHandlers.ListFieldsByGroup)
			// Custom field entity types (public — for frontend entity type selection)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/custom-field-entity-types", cfHandlers.ListEntityTypes)
			// Admin CRUD for custom field entity types (Section 06)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.group.view")).
				Get("/admin/custom-field-entity-types", cfHandlers.ListEntityTypesAdmin)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.group.create")).
				Post("/admin/custom-field-entity-types", cfHandlers.CreateEntityType)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.group.edit")).
				Patch("/admin/custom-field-entity-types/{id}", cfHandlers.UpdateEntityType)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.group.delete")).
				Delete("/admin/custom-field-entity-types/{id}", cfHandlers.DeleteEntityType)
			// Guide Category admin CRUD (Section 14 — Guide Content Type System)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.group.view")).
				Get("/admin/guide-categories", cfHandlers.ListGuideCategoriesAdmin)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.group.create")).
				Post("/admin/guide-categories", cfHandlers.CreateGuideCategoryAdmin)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.group.delete")).
				Delete("/admin/guide-categories/{slug}", cfHandlers.DeleteGuideCategoryAdmin)
			// Write (require auth + permission)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.group.create")).
				Post("/custom-field-groups", cfHandlers.CreateFieldGroup)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.group.edit")).
				Patch("/custom-field-groups/{id}", cfHandlers.UpdateFieldGroup)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.group.delete")).
				Delete("/custom-field-groups/{id}", cfHandlers.DeleteFieldGroup)

			// Custom field routes (field-level operations, separate path prefix)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.field.create")).
				Post("/custom-field-groups/{id}/fields", cfHandlers.CreateField)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.field.edit")).
				Patch("/custom-fields/{id}", cfHandlers.UpdateField)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.field.delete")).
				Delete("/custom-fields/{id}", cfHandlers.DeleteField)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.field.edit")).
				Put("/custom-fields/{id}/permissions", cfHandlers.SetFieldPermission)
			// BUG-007: gate ListFieldPermissions with custom_fields.group.view so
			// non-admin users cannot read the field-level RBAC configuration.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.group.view")).
				Get("/custom-fields/{id}/permissions", cfHandlers.ListFieldPermissions)
			// BUG-009: DELETE route for field permissions — completes the CRUD
			// surface so permissions can be removed, not just upserted.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.field.edit")).
				Delete("/custom-fields/{id}/permissions", cfHandlers.DeleteFieldPermission)

			// Field value routes
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/custom-field-values/{entity_type}/{entity_id}", cfHandlers.ListFieldValues)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.values.set")).
				Put("/custom-field-values", cfHandlers.SetFieldValue)

			// Form routes
			// Public submission (optional auth — anonymous users can submit forms)
			// §06: rate-limited to 10 submissions per user (or IP for anonymous) per hour.
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "form_submission")).Post("/forms/{id}/submissions", cfHandlers.SubmitForm)
			// Read (require auth)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/forms", cfHandlers.ListForms)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/forms/{id}", cfHandlers.GetForm)
			// Public form schema by slug (§06 L240). The embed route is the
			// iframe-friendly variant; /forms/{slug}/embed serves both the
			// canonical schema and the embed use case. A separate
			// /forms/{slug} route would conflict with /forms/{id} in chi's
			// router (both are single-segment params), so the embed route
			// is the canonical public form schema route.
			r.Get("/forms/{slug}/embed", cfHandlers.GetFormEmbed)
			// Write (require auth + permission)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forms.create")).
				Post("/forms", cfHandlers.CreateForm)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forms.edit")).
				Patch("/forms/{id}", cfHandlers.UpdateForm)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forms.delete")).
				Delete("/forms/{id}", cfHandlers.DeleteForm)
			// Ensure form has a backing field group (called by admin UI before adding fields)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forms.edit")).
				Post("/forms/{id}/ensure-field-group", cfHandlers.EnsureFormGroup)
			// Space-scoped form creation (the builder UI posts here)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forms.create")).
				Post("/spaces/{spaceId}/forms", cfHandlers.CreateSpaceForm)
			// Submission listing (require auth + permission)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forms.submissions.view")).
				Get("/forms/{id}/submissions", cfHandlers.ListFormSubmissions)

			// Form submission trigger management (Section 06, BUG-003). The
			// admin form builder UI calls these to configure triggers that
			// fire after a submission is created.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forms.submissions.view")).
				Get("/forms/{id}/triggers", cfHandlers.ListFormTriggers)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forms.edit")).
				Post("/forms/{id}/triggers", cfHandlers.CreateFormTrigger)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forms.edit")).
				Patch("/forms/{id}/triggers/{triggerId}", cfHandlers.UpdateFormTrigger)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forms.delete")).
				Delete("/forms/{id}/triggers/{triggerId}", cfHandlers.DeleteFormTrigger)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forms.edit")).
				Post("/forms/{id}/triggers/reorder", cfHandlers.ReorderFormTriggers)

			// Form steps (multi-step forms, Section 06). Steps are owned by a
			// form; viewing requires forms.submissions.view (same as the form
			// detail), mutation requires forms.edit / forms.delete.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forms.submissions.view")).
				Get("/forms/{id}/steps", cfHandlers.ListFormSteps)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forms.edit")).
				Post("/forms/{id}/steps", cfHandlers.CreateFormStep)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forms.edit")).
				Patch("/forms/{id}/steps/{stepId}", cfHandlers.UpdateFormStep)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forms.delete")).
				Delete("/forms/{id}/steps/{stepId}", cfHandlers.DeleteFormStep)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forms.edit")).
				Post("/forms/{id}/steps/reorder", cfHandlers.ReorderFormSteps)

			// Admin field-group routes — the same underlying handlers as the
			// /custom-field-groups block above, mounted a second time under
			// /admin per catalog (an admin-facing management surface distinct
			// from the tenant-scoped API the SPA consumes at the bare paths).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.group.view")).
				Get("/admin/field-groups", cfHandlers.ListFieldGroups)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.group.view")).
				Get("/admin/field-groups/{id}", cfHandlers.GetFieldGroup)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.group.create")).
				Post("/admin/field-groups", cfHandlers.CreateFieldGroupAdmin)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.group.edit")).
				Patch("/admin/field-groups/{id}", cfHandlers.UpdateFieldGroup)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.group.delete")).
				Delete("/admin/field-groups/{id}", cfHandlers.DeleteFieldGroup)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.group.view")).
				Get("/admin/field-groups/{id}/fields", cfHandlers.ListFieldsByGroup)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.field.create")).
				Post("/admin/field-groups/{id}/fields", cfHandlers.CreateField)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.field.edit")).
				Patch("/admin/field-groups/{id}/fields/{fid}", cfHandlers.UpdateFieldInGroup)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.field.delete")).
				Delete("/admin/field-groups/{id}/fields/{fid}", cfHandlers.DeleteFieldInGroup)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.field.edit")).
				Post("/admin/field-groups/{id}/fields/reorder", cfHandlers.ReorderFields)

			// Admin form submission single-item routes — the list already
			// exists at /forms/{id}/submissions above; these add the
			// per-submission GET/PATCH/DELETE the catalog documents under
			// /admin/forms.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forms.submissions.view")).
				Get("/admin/forms/{id}/submissions/{sub_id}", cfHandlers.GetFormSubmission)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forms.submissions.edit")).
				Patch("/admin/forms/{id}/submissions/{sub_id}", cfHandlers.UpdateFormSubmission)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forms.submissions.delete")).
				Delete("/admin/forms/{id}/submissions/{sub_id}", cfHandlers.DeleteFormSubmission)

			// Admin form management (Section 06 L253-261, BUG-008). The bare
			// /forms routes above remain for backward compatibility with the
			// existing admin frontend; these /admin/forms routes are the
			// §06-canonical admin surface. The list-submissions route is
			// mounted here because §06 specifies it under /admin/forms.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forms.submissions.view")).
				Get("/admin/forms", cfHandlers.ListForms)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forms.create")).
				Post("/admin/forms", cfHandlers.CreateForm)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forms.submissions.view")).
				Get("/admin/forms/{id}", cfHandlers.GetForm)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forms.edit")).
				Patch("/admin/forms/{id}", cfHandlers.UpdateForm)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forms.delete")).
				Delete("/admin/forms/{id}", cfHandlers.DeleteForm)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forms.submissions.view")).
				Get("/admin/forms/{id}/submissions", cfHandlers.ListFormSubmissions)

			// Space type routes — reads are public (OptionalAuth) so the /spaces
			// directory page can render the type filter chips for guests; writes
			// move under /admin per catalog (management of the type definitions
			// themselves is admin-only).
			r.With(auth.OptionalAuth(kr, vkClient.Redis())).Get("/space-types", spaceHandlers.ListSpaceTypes)
			r.With(auth.OptionalAuth(kr, vkClient.Redis())).Get("/space-types/{id}", spaceHandlers.GetSpaceType)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spaces.types.create")).
				Get("/admin/space-types", spaceHandlers.ListSpaceTypes)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spaces.types.create")).
				Post("/admin/space-types", spaceHandlers.CreateSpaceType)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spaces.types.edit")).
				Patch("/admin/space-types/{id}", spaceHandlers.UpdateSpaceType)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spaces.types.delete")).
				Delete("/admin/space-types/{id}", spaceHandlers.DeleteSpaceType)

			// Space routes
			// Public space reads use OptionalAuth so guests can browse public
			// spaces (Section 05: "public spaces visible to all"; Guide §16 lists
			// /spaces/* as SSR-public). Mutations and /me/* stay RequireAuth.
			// OptionalPermission lets admins revoke directory.spaces.view from a
			// group to hide /spaces from that group's authed members.
			// Ref: japaconnect-engineering/sections/03-rbac.md (Directory visibility nodes)
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), rbac.OptionalPermission(rbacResolver, "directory.spaces.view")).Get("/spaces", spaceHandlers.ListSpaces)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/spaces", spaceHandlers.ListMySpaces)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/spaces/moderated", spaceHandlers.ListMyModeratedSpaces)
			// BUG-006: /me/spaces/owned — ownership is tracked via
			// space_members.role='owner' (migration 000033), not a spaces
			// column. The ListMyOwnedSpaces handler queries that join.
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/spaces/owned", spaceHandlers.ListMyOwnedSpaces)
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService)).
				Get("/spaces/{slug}", spaceHandlers.GetSpace)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spaces.create")).
				Post("/spaces", spaceHandlers.CreateSpace)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spaces.edit")).
				Patch("/spaces/{id}", spaceHandlers.UpdateSpace)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spaces.delete")).
				Delete("/spaces/{id}", spaceHandlers.DeleteSpace)
			// Space membership (join/leave toggle)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService)).
				Post("/spaces/{slug}/join", spaceHandlers.ToggleSpaceMembership)
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService)).
				Get("/spaces/{slug}/members", spaceHandlers.ListMembers)

			// Space follow/unfollow (Section 31) — M5: rate-limited
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService), mw.RateLimit(vkClient.Redis(), "moderate")).
				Post("/spaces/{slug}/follow", spaceHandlers.ToggleFollow)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService), mw.RateLimit(vkClient.Redis(), "moderate")).
				Delete("/spaces/{slug}/follow", spaceHandlers.ToggleFollow)

			// Space mute/unmute (Section 52). BE-3: enforce
			// interactions.mute.manage — the same node that guards user mutes
			// (Section 41 line 176). Mute-status is a read-only check and
			// stays behind RequireAuth only.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService), rbac.RequirePermission(rbacResolver, "interactions.mute.manage")).
				Post("/spaces/{slug}/mute", spaceHandlers.MuteSpace)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService), rbac.RequirePermission(rbacResolver, "interactions.mute.manage")).
				Delete("/spaces/{slug}/mute", spaceHandlers.UnmuteSpace)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService)).
				Get("/spaces/{slug}/mute-status", spaceHandlers.GetMuteStatus)

			// Space content & pinned posts
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService)).
				Get("/spaces/{slug}/content", spaceHandlers.ListSpaceContent)
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService)).
				Get("/spaces/{slug}/pinned", spaceHandlers.ListPinned)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService), rbac.RequireSpacePermission(rbacResolver, "spaces.moderate")).
				Post("/spaces/{slug}/pinned", spaceHandlers.AddPinned)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService), rbac.RequireSpacePermission(rbacResolver, "spaces.moderate")).
				Delete("/spaces/{slug}/pinned/{content_id}", spaceHandlers.RemovePinned)

			// Dedicated leave (catalog: DELETE /spaces/:id/leave), split from
			// the join/leave toggle above.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService)).
				Delete("/spaces/{slug}/leave", spaceHandlers.Leave)
			// catalog: POST /spaces/:id/report — takes a raw ID (like the other
			// report routes), not the slug the rest of this block resolves on.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "reports.submit")).Post("/spaces/{id}/report", reportsHandlers.ReportSpace)

			// Space rule routes (slug-resolved via SpaceResolver middleware)
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService)).
				Get("/spaces/{slug}/rules", spaceHandlers.ListSpaceRules)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService), rbac.RequireSpacePermission(rbacResolver, "spaces.moderate")).
				Post("/spaces/{slug}/rules", spaceHandlers.CreateSpaceRule)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService), rbac.RequireSpacePermission(rbacResolver, "spaces.moderate")).
				Patch("/spaces/{slug}/rules/{ruleId}", spaceHandlers.UpdateSpaceRule)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService), rbac.RequireSpacePermission(rbacResolver, "spaces.moderate")).
				Delete("/spaces/{slug}/rules/{ruleId}", spaceHandlers.DeleteSpaceRule)

			// Space moderator routes (slug-resolved via SpaceResolver middleware).
			// These use RequirePermissionWithInvalidation so the affected
			// user's permission cache is flushed immediately after the mutation.
			// AUDIT-FRESH-NAME-001: consolidated on spaces.moderator.assign
			// (spaces.moderate.assigned removed as a duplicate node).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService), rbac.RequirePermissionWithInvalidation(rbacResolver, "spaces.moderator.assign")).
				Post("/spaces/{slug}/moderators", spaceHandlers.AssignModerator)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService), rbac.RequirePermissionWithInvalidation(rbacResolver, "spaces.moderator.assign")).
				Delete("/spaces/{slug}/moderators/{userId}", spaceHandlers.RemoveModerator)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService)).
				Get("/spaces/{slug}/moderators", spaceHandlers.ListModerators)

			// Space parent spaces (many-to-many hierarchy)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService), rbac.RequirePermission(rbacResolver, "spaces.edit")).
				Post("/spaces/{slug}/parents", spaceHandlers.AddParentSpace)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService), rbac.RequirePermission(rbacResolver, "spaces.edit")).
				Delete("/spaces/{slug}/parents/{parentId}", spaceHandlers.RemoveParentSpace)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService)).
				Get("/spaces/{slug}/parents", spaceHandlers.ListParentSpaces)

			// Permission preset routes
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/space-permission-presets", spaceHandlers.ListPermissionPresets)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spaces.admin")).
				Put("/space-permission-presets/{name}", spaceHandlers.UpdatePermissionPreset)

			// Content routes (Section 14 — Content Type System)
			// Create (require auth + moderate rate limit + censor check + permission)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "moderate"), spam.CensorMiddleware(database, vkClient), spam.RequireNotIPBlocked(spam.BlockTypePosting), rbac.RequirePermission(rbacResolver, "content.create")).
				Post("/content", contentHandlers.CreateContent)
			// Read (require auth for list; single content is public for
			// published items — Section 14 public content SSR).
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/content", contentHandlers.ListContent)
			r.With(auth.OptionalAuth(kr, vkClient.Redis())).Get("/content/{slug}", contentHandlers.GetContent)
			// Content types (public — for frontend content type selection)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/content-types", contentHandlers.ListContentTypes)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/content/visibility-options", contentHandlers.GetVisibilityOptions)
			// Update/Delete (require auth + edit lock + censor check + permission)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.EditLockMiddleware(vkClient.Redis(), settingsService), spam.CensorMiddleware(database, vkClient), rbac.RequirePermission(rbacResolver, "content.edit")).
				Patch("/content/{id}", contentHandlers.UpdateContent)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.EditLockMiddleware(vkClient.Redis(), settingsService), rbac.RequirePermission(rbacResolver, "content.delete")).
				Delete("/content/{id}", contentHandlers.DeleteContent)
			// Lifecycle: publish/archive/schedule are status transitions (content.edit);
			// view/share-count just require auth.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.edit")).
				Post("/content/{id}/publish", contentHandlers.PublishContent)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.edit")).
				Post("/content/{id}/archive", contentHandlers.ArchiveContent)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.edit")).
				Post("/content/{id}/schedule", contentHandlers.ScheduleContent)
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "loose")).Post("/content/{id}/view", contentHandlers.RecordContentView)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/content/{id}/share-count", contentHandlers.GetContentShareCount)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "reports.submit")).Post("/content/{id}/report", reportsHandlers.ReportContent)
			// Revisions (require auth + permission to view)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.revisions.view")).
				Get("/content/{id}/revisions", contentHandlers.ListRevisions)
			// Pin/Unpin (require auth + content.pin permission)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.pin")).
				Post("/content/{id}/pin", contentHandlers.PinContent)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.pin")).
				Delete("/content/{id}/pin", contentHandlers.UnpinContent)
			// Reactions (require auth + permission)
			r.With(features.Gate(featureChecker, "feature.reactions")).Group(func(r chi.Router) {
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.reactions.add")).Post("/content/{id}/reactions", contentHandlers.AddReaction)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.reactions.remove")).Delete("/content/{id}/reactions/{key}", contentHandlers.RemoveReaction)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/content/{id}/reactions", contentHandlers.ListReactions)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/content/{id}/reactions/me", contentHandlers.ListUserReactions)
			})
			// Content → series lookup (auth-required). Returns the series_id
			// for a content item so the editor can restore the series selector.
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/content/{id}/series", blogHandlers.GetContentSeries)

			// Reaction type routes (admin-configurable emoji set)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/reaction-types", contentHandlers.ListReactionTypes)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.reaction_types.manage")).
				Post("/reaction-types", contentHandlers.CreateReactionType)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.reaction_types.manage")).
				Patch("/reaction-types/{key}", contentHandlers.UpdateReactionType)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.reaction_types.manage")).
				Delete("/reaction-types/{key}", contentHandlers.DeleteReactionType)

			// Share platform routes (admin-configurable social media sharing)
			// Public read for enabled platforms, admin CRUD for management.
			r.Get("/share-platforms/enabled", contentHandlers.ListEnabledSharePlatforms)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/share-platforms", contentHandlers.ListSharePlatforms)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.reaction_types.manage")).
				Post("/share-platforms", contentHandlers.CreateSharePlatform)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.reaction_types.manage")).
				Patch("/share-platforms/{key}", contentHandlers.UpdateSharePlatform)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.reaction_types.manage")).
				Delete("/share-platforms/{key}", contentHandlers.DeleteSharePlatform)

			// JSON-LD template routes
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/content/jsonld-templates", contentHandlers.ListJSONLDTemplates)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.jsonld.manage")).
				Put("/content/jsonld-templates", contentHandlers.UpsertJSONLDTemplate)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.jsonld.manage")).
				Delete("/content/jsonld-templates/{id}", contentHandlers.DeleteJSONLDTemplate)

			// Page builder block types (Section 16 — Page Builder).
			// Any authenticated user can view the block registry; no
			// special permission required.
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/page-builder/block-types", contentHandlers.ListBlockTypes)

			// Renderer routes (Section 15 — Block Editor SSR).
			// When the renderer failed to initialise (bad script path),
			// register a 503 stub so the route is reachable but reports
			// the service is unavailable instead of nil-dereferencing.
			renderHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				errtracking.WriteError(w, http.StatusServiceUnavailable, "renderer_disabled", "Content rendering is temporarily unavailable")
			})
			if rendererHandlers != nil {
				renderHandler = rendererHandlers.Render
			}
			r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "moderate")).Post("/content/render", renderHandler)

			// Link preview routes (Section 15 — Link Preview Fetch, BUG-008 RBAC gate)
			// BUG-007: tight rate limit (10 req/min per user) — the endpoint
			// triggers server-side external fetches, so it must be throttled
			// to prevent authenticated abuse (server-side fetch amplification).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "link_preview.fetch"), mw.RateLimit(vkClient.Redis(), "tight")).Get("/link-preview", linkPreviewHandlers.Fetch)

			// Content Bookmarks (Section 14 — Content System API)
			r.With(features.Gate(featureChecker, "feature.bookmarks")).Group(func(r chi.Router) {
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "bookmarks.create")).Post("/content/{id}/bookmark", contentHandlers.AddBookmark)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "bookmarks.create")).Delete("/content/{id}/bookmark", contentHandlers.RemoveBookmark)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/content/{id}/bookmark", contentHandlers.GetBookmarkStatus)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "bookmarks.create")).Patch("/content/{id}/bookmark/folder", contentHandlers.MoveBookmark)
			})

			// Content Tags (Section 14 — Content System API)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/content/{id}/tags", contentHandlers.ListContentTags)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.edit")).
				Post("/content/{id}/tags", contentHandlers.AddContentTag)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.edit")).
				Delete("/content/{id}/tags/{tagId}", contentHandlers.RemoveContentTag)

			// Content Shares (Section 14 — Content System API)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.share")).Post("/content/{id}/share", contentHandlers.ShareContent)

			// Related Content (Section 14 — Content System API, pgvector cosine similarity)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/content/{id}/related", contentHandlers.GetRelatedContent)

			// Singular reaction action routes (catalog lines 259-260). The
			// plural /content/{id}/reactions collection routes are registered
			// above; these singular aliases are required by the route catalog
			// for non-admin callers. POST reads reaction_key from the body;
			// DELETE reads key from the query string (no {key} path param).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.reactions.add")).
				Post("/content/{id}/react", contentHandlers.AddReaction)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.reactions.remove")).
				Delete("/content/{id}/react", contentHandlers.RemoveReaction)

			// Content space assignment (catalog line 261). Multi-space update
			// with primary_space_id + additional_space_ids.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.edit")).
				Patch("/content/{id}/spaces", contentHandlers.UpdateContentSpaces)

			// Comments (Section 49 — Unified Comment System)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/comments/{id}", commentsHandlers.GetComment)
			r.Get("/content/{id}/comments", commentsHandlers.ListComments)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spam.CensorMiddleware(database, vkClient), spam.RequireNotIPBlocked(spam.BlockTypePosting), rbac.RequirePermission(rbacResolver, "comments.create")).Post("/content/{id}/comments", commentsHandlers.CreateComment)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.EditLockMiddleware(vkClient.Redis(), settingsService), spam.CensorMiddleware(database, vkClient), rbac.RequirePermission(rbacResolver, "comments.edit_own")).Patch("/comments/{id}", commentsHandlers.UpdateComment)
			// BUG-003: own-vs-any resolved in the handler (RequireAuth only) so a
			// moderator with comments.delete_any is not blocked by a delete_own
			// middleware gate, and a plain member cannot delete another's comment.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.EditLockMiddleware(vkClient.Redis(), settingsService)).Delete("/comments/{id}", commentsHandlers.DeleteComment)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comments.vote")).Post("/comments/{id}/vote", commentsHandlers.VoteComment)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comments.vote")).Delete("/comments/{id}/vote", commentsHandlers.RemoveVote)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "reports.submit")).Post("/comments/{id}/report", reportsHandlers.ReportComment)
			// BL-003: pin/unpin route is RequireAuth only; the handler resolves
			// moderator (comments.pin) vs content-author at request time so a
			// content author who doesn't hold comments.pin can still pin comments
			// on their own content (Section 49).
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Post("/comments/{id}/pin", commentsHandlers.PinComment)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Delete("/comments/{id}/pin", commentsHandlers.UnpinComment)

			// Follow System routes (Section 31 — Follow System)
			// M5: rate-limit all follow mutation routes to prevent follow spam.
			r.With(features.Gate(featureChecker, "feature.follow")).Group(func(r chi.Router) {
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "follows.toggle"), mw.RateLimit(vkClient.Redis(), "moderate")).Post("/follows", followHandlers.ToggleFollow)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/following", followHandlers.ListMyFollowing)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/followers", followHandlers.ListMyFollowers)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/followed-tags", followHandlers.ListMyFollowedTags)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/users/{user_id}/followers", followHandlers.GetFollowers)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/users/{user_id}/following", followHandlers.GetFollowing)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/users/{user_id}/mutual/{other_user_id}", followHandlers.CheckMutual)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/users/{user_id}/follow-status", followHandlers.GetFollowStatus)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/spaces/{space_id}/followers", followHandlers.GetFollowers)
			})

			// User Blocks (Section 41 — user-facing, non-admin)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "interactions.block.manage")).Post("/users/{user_id}/block", interactionsHandlers.BlockUser)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "interactions.block.manage")).Delete("/users/{user_id}/block", interactionsHandlers.UnblockUser)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/blocked-users", interactionsHandlers.ListBlocked)

			// User Mutes (Section 52)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "interactions.mute.manage")).Post("/users/{user_id}/mute", interactionsHandlers.MuteUser)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "interactions.mute.manage")).Delete("/users/{user_id}/mute", interactionsHandlers.UnmuteUser)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/users/{user_id}/mute-status", interactionsHandlers.GetUserMuteStatus)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "reports.submit")).Post("/users/{id}/report", reportsHandlers.ReportUser)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/muted-users", interactionsHandlers.ListMutedUsers)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/muted-spaces", interactionsHandlers.ListMutedSpaces)

			// Notification System routes (Section 35 — Notification System)
			r.With(features.Gate(featureChecker, "feature.notifications")).Group(func(r chi.Router) {
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/notifications", notificationHandlers.ListNotifications)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/notifications/unread-count", notificationHandlers.GetUnreadCount)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/me/notifications/{id}/read", notificationHandlers.MarkAsRead)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/me/notifications/read-all", notificationHandlers.MarkAllAsRead)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/me/notifications/{id}", notificationHandlers.DeleteNotification)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/notification-preferences", notificationHandlers.GetPreferences)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Patch("/me/notification-preferences", notificationHandlers.UpdatePreferences)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Patch("/me/notification-preferences/{type}", notificationHandlers.UpdatePreference)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/notification-types", notificationHandlers.ListNotificationTypes)

				// Push Subscription routes (public VAPID key, authenticated subscribe/unsubscribe)
				// catalog: POST /me/push-tokens, DELETE /me/push-tokens/:id
				r.Get("/push/vapid-key", notificationHandlers.HandleGetVAPIDKey)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/me/push-tokens", notificationHandlers.HandlePushSubscribe)
				// BUG-005: catalog route is DELETE /me/push-tokens/:id (path-param id).
				// The {id} variant is registered below (Task 28 me-additions block) and
				// uses HandlePushUnsubscribeByID. This body-endpoint variant is kept for
				// backward compatibility with older clients that key by endpoint.
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/me/push-tokens", notificationHandlers.HandlePushUnsubscribe)
			})

			// Feed routes (Section 32 — Activity Feed / Timeline)
			r.Get("/feed/card-style", feedHandlers.GetCardStyle)                                            // public — no auth required
			r.Get("/explore", feedHandlers.GetExploreFeed)                                                  // public — no auth required (catalog: /explore)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/feed", feedHandlers.GetPrimaryFeed)        // catalog: /feed (primary timeline)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/feed/for-you", feedHandlers.GetForYouFeed) // catalog: /feed/for-you (personalized ranked)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/feed/space/{space_id}", feedHandlers.GetSpaceFeed)
			// catalog: GET /spaces/:id/feed — same handler, param name matches (space_id).
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/spaces/{space_id}/feed", feedHandlers.GetSpaceFeed)
			// Public per Section 32:12 (profile timeline audience = "Anyone"), but
			// gated so a banned/suspended/deleted user's timeline 404s like the rest
			// of their profile (governing decision: "the whole profile 404s for
			// non-active users"). The feed route's param is {user_id}, not {id}.
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), requireActiveProfileUser(usersService, "user_id")).Get("/feed/profile/{user_id}", feedHandlers.GetProfileFeed)

			// Trending & Discovery routes (Phase 28)
			// Public trending endpoints (no auth required)
			r.Get("/trending/content", trendingHandlers.GetTrendingContent)
			r.Get("/trending/tags", trendingHandlers.GetTrendingTags)
			// Catalog alias: /tags/trending
			r.Get("/tags/trending", trendingHandlers.GetTrendingTags)
			r.Get("/trending/spaces", trendingHandlers.GetTrendingSpaces)
			r.Get("/trending/users", trendingHandlers.GetTrendingUsers)

			// Auth-required suggestion endpoints
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/suggestions/users", trendingHandlers.GetSuggestedUsers)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/suggestions/spaces", trendingHandlers.GetSuggestedSpaces)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/suggestions/tags", trendingHandlers.GetSuggestedTags)

			// Recommendation engine (ADR-064a) — public, works for guests and
			// authenticated users. Uses VisitorContext for geo-aware ranking.
			r.Get("/recommendations", recommendationsHandlers.GetRecommendations)

			// Geo routes (Phase 30 — Geo IP; catalog: Sections 05, 37)
			// Public: returns geo data for the current request IP (resolved by middleware)
			// /geo/me is rate-limited to 10/min per IP per Section 37.
			r.With(mw.RateLimit(vkClient.Redis(), "tight")).Get("/geo/me", geoHandlers.HandleGeoMe)
			r.Get("/geo/detect", geoHandlers.HandleGeoDetect)
			// Admin: resolve an arbitrary IP address
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "geo.lookup")).
				Get("/geo/lookup", geoHandlers.HandleGeoLookup)
			// Geo hierarchy (public — read-views over seeded country/state/city spaces)
			r.Get("/geo/countries", geoHandlers.HandleListCountries)
			r.Get("/geo/countries/{code}/states", geoHandlers.HandleListStates)
			r.Get("/geo/states/{id}/cities", geoHandlers.HandleListCities)

			// Universities (Section 39 — read-view over University-type spaces)
			r.Get("/universities", universitiesHandlers.ListUniversities)
			r.Get("/universities/{slug}", universitiesHandlers.GetUniversity)
			// Section 39 line 152: comparison matrix link redirects to the
			// /compare/universities picker with this university pre-selected.
			r.Get("/universities/{slug}/compare", universitiesHandlers.CompareUniversities)

			// University admin CRUD (P5.6 — Task 11, catalog paths)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings")).
				Get("/admin/universities", universitiesHandlers.ListUniversitiesAdmin)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings")).
				Post("/admin/universities", universitiesHandlers.CreateUniversity)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings")).
				Patch("/admin/universities/{id}", universitiesHandlers.UpdateUniversity)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings")).
				Delete("/admin/universities/{id}", universitiesHandlers.DeleteUniversity)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings")).
				Post("/admin/universities/{id}/verify", universitiesHandlers.VerifyUniversity)
			// Audit U-1: unverify, status change, and CSV bulk-import routes
			// called by the admin frontend were missing (404). Wired here so the
			// admin panel's Unverify / Change Status / CSV Import buttons work.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings")).
				Delete("/admin/universities/{id}/verify", universitiesHandlers.UnverifyUniversity)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings")).
				Patch("/admin/universities/{id}/status", universitiesHandlers.UpdateUniversityStatus)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings")).
				Post("/admin/universities/bulk-import", universitiesHandlers.BulkImportUniversities)

			// WebSocket + DM routes (Section 10 — Real-Time WebSockets)
			wsHub := websocket.NewHub()
			controlHandlers.SetWSHub(wsHub)
			// BUG-002: wire the WS hub into the notification service so new
			// notifications are pushed to clients in real time. Ref: §35 L88.
			notificationService.SetWSHub(wsHubWSAdapter{hub: wsHub})
			// PubSub bridge for multi-instance WebSocket scaling.
			wsBridge := websocket.NewPubSubBridge(vkClient.Redis(), wsHub)
			wsHub.SetPubSub(wsBridge)

			// PGX-002: use a signal-aware context so the bridge stops on
			// SIGINT/SIGTERM instead of running forever on context.Background().
			wsCtx, wsCancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			// Register the cancel func with the shutdown closers so a graceful
			// Shutdown() (not triggered by a signal) also stops the bridge
			// goroutine and releases the signal handler. Previously the cancel
			// func was discarded, leaking the goroutine + signal handler on
			// non-signal shutdown.
			closers = append(closers, wsCancel)
			go wsBridge.Start(wsCtx)

			wsService := websocket.NewService(database, wsHub)
			wsDMStore := websocket.NewDMStore(gen.New(database.Reader), gen.New(database.Writer))
			wsDMService := websocket.NewDMService(wsDMStore, wsHub)
			wsDMService.AsynqClient = asynqClient
			// Wire DM store into mentorship service for auto-opening DM
			// threads when a mentorship connection is accepted (§102, ADR-110).
			mentorshipService.SetDMCreator(wsDMStore)
			// Wire DM service into mutual aid handlers so HandleRespond can
			// open a DM thread with the post author (§85, ADR-093). An adapter
			// converts websocket.ConversationResult to mutualaid.dmConversationResult.
			mutualAidHandlers.SetDMInitiator(&mutualAidDMAdapter{dm: wsDMService})
			wsHandlers := websocket.NewHandlers(wsService, wsDMService, wsHub, vkClient, kr)
			wsHandlers.SetSpamPipeline(spamPipeline)
			wsHandlers.SetSessionChecker(func(userID, tenantID string) bool {
				var uid pgtype.UUID
				if err := uid.Scan(userID); err != nil {
					return false
				}
				var tid pgtype.UUID
				if err := tid.Scan(tenantID); err != nil {
					return false
				}
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				q := gen.New(database.Reader)
				// Check 1: active ban or suspension via user_moderation_actions.
				if _, err := q.GetUserBanExpiration(ctx, uid); err == nil {
					return false
				}
				// Check 2: user status. Covers GDPR erasure (status='deleted')
				// and direct admin suspension (status='suspended') that may not
				// have a corresponding user_moderation_actions row. Only
				// status='active' keeps the connection live.
				user, err := q.GetUserByID(ctx, gen.GetUserByIDParams{
					TenantID: tid,
					UserID:   uid,
				})
				if err != nil || user.Status != "active" {
					return false
				}
				return true
			})

			// WebSocket + DM routes — gated by feature.dm (Section 10)
			r.With(features.Gate(featureChecker, "feature.dm")).Group(func(r chi.Router) {
				// WebSocket — tight rate limit on upgrade to prevent abuse
				// (10 connections per minute per IP, per ADR-022).
				r.With(mw.RateLimit(vkClient.Redis(), "tight")).Get("/ws", wsHandlers.HandleWebSocket)
				// /auth/ws-ticket: any authenticated user needs a WebSocket
				// ticket for real-time notifications — not just DM users. The
				// previous dm.view gate caused 403 for users without DM
				// permission, breaking notification delivery on the Learn app.
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/auth/ws-ticket", wsHandlers.GetWSTicket)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "dm.view")).Get("/ws/online", wsHandlers.GetOnlineUsers)

				// Direct Messaging — dm.view enforced on all read routes (BUG-016).
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "dm.view")).Get("/dm/conversations", wsHandlers.ListConversations)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "dm.request")).Post("/dm/conversations", wsHandlers.InitiateConversation)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "dm.view")).Get("/dm/conversations/{id}/messages", wsHandlers.ListMessages)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "dm.send")).Post("/dm/conversations/{id}/messages", wsHandlers.SendMessage)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "dm.send")).Patch("/dm/conversations/{id}/messages/{mid}", wsHandlers.EditMessage)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "dm.delete")).Delete("/dm/conversations/{id}/messages/{mid}", wsHandlers.DeleteMessage)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "dm.view")).Post("/dm/conversations/{id}/read", wsHandlers.MarkAsRead)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "reports.submit")).Post("/dm/conversations/{id}/messages/{mid}/report", reportsHandlers.ReportDMMessage)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "dm.view")).Get("/dm/requests", wsHandlers.ListMessageRequests)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "dm.view")).Post("/dm/requests/{id}/accept", wsHandlers.AcceptMessageRequest)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "dm.view")).Post("/dm/requests/{id}/decline", wsHandlers.DeclineMessageRequest)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "dm.view")).Post("/dm/requests/{id}/block", wsHandlers.BlockMessageRequest)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "dm.view")).Get("/dm/privacy", wsHandlers.GetDMPrivacy)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "dm.manage_privacy")).Put("/dm/privacy", wsHandlers.UpdateDMPrivacy)
			})

			// Media routes (Section 34 — Media System)
			mediaHandlers.AsynqClient = asynqClient

			// Media serving endpoint (no auth required for serving files)
			r.Get("/media/serve/{bucket}/*", mediaHandlers.ServeMedia)

			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.upload")).Post("/media/upload", mediaHandlers.InitiateUpload)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/media/confirm/{id}", mediaHandlers.ConfirmUpload)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.library.view_own")).Get("/media/{id}", mediaHandlers.GetMedia)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.library.view_own")).Patch("/media/{id}", mediaHandlers.UpdateMetadata)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.delete_own")).Delete("/media/{id}", mediaHandlers.DeleteMedia)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.view_all")).Get("/media", mediaHandlers.ListMedia)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.library.view_own")).Get("/me/media", mediaHandlers.ListMyMedia)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.delete_own")).Post("/media/bulk-delete", mediaHandlers.BulkDelete)

			// Media quarantine admin routes — review queue for virus scan false positives
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.quarantine.manage")).
				Get("/admin/media/quarantined", mediaHandlers.ListQuarantined)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.quarantine.manage")).
				Post("/admin/media/{id}/approve", mediaHandlers.ApproveQuarantined)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.quarantine.manage")).
				Post("/admin/media/{id}/reject", mediaHandlers.RejectQuarantined)

			// Admin media listing and deletion (P5.6 — Task 9)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.view_all")).
				Get("/admin/media", mediaHandlers.ListMedia)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.delete_any")).
				Delete("/admin/media/{id}", mediaHandlers.AdminDeleteMedia)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.edit_any")).
				Patch("/admin/media/{id}", mediaHandlers.AdminUpdateMetadata)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.quarantine.manage")).
				Post("/admin/media/{id}/status", mediaHandlers.AdminUpdateMediaStatus)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.upload")).
				Post("/admin/media/upload", mediaHandlers.AdminUpload)

			// Stock photo routes (Section 34 — Stock Photo Integration).
			// Openverse is available with media.stock.search; the other three
			// providers additionally require media.stock.providers (enforced in-handler).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.stock.search")).
				Get("/stock/providers", stockHandlers.ListProviders)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.stock.search")).
				Get("/stock/search", stockHandlers.Search)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.stock.search")).
				Get("/stock/photo/{provider}/{id}", stockHandlers.GetPhoto)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.stock.import")).
				Post("/stock/import", stockHandlers.ImportPhoto)

			// Stock photo admin routes — provider config management.
			// Gated by the dedicated media.stock.admin node (not media.edit_any)
			// so that managing provider API keys is decoupled from media edit rights.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.stock.admin")).
				Get("/admin/stock/providers", stockHandlers.AdminListProviders)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.stock.admin")).
				Patch("/admin/stock/providers/{name}", stockHandlers.AdminPatchProvider)

			// Forum Q&A / Solved routes — gated by feature.forum (Section 08)
			forumService := forum.NewService(database)
			forumService.SetSettings(settingsService)
			forumService.SetRBAC(rbacResolver)
			forumService.SetPermCacheInvalidator(&forumPermCacheInvalidator{vk: vkClient.Redis()})
			forumService.SetWebhookEmitter(&forumWebhookEmitter{svc: webhookService})
			// Wire webhook event fan-out into the remaining spec-mandated services
			// (Section 12). Each emitter is nil-safe (no-op when unset).
			contentService.SetWebhookEmitter(webhookService)
			reviewService.SetWebhookEmitter(webhookService)
			paymentService.SetWebhookEmitter(webhookService)
			orgService.SetWebhookEmitter(webhookService)
			forumService.SetSpaceBroadcaster(&forumSpaceBroadcaster{hub: wsHub})
			// BUG-009/010: wire Valkey for distributed JSON-LD cache and
			// view-count counters with periodic flush. Ref: valkey-keys-catalog.md.
			forumService.SetValkey(vkClient.Redis())
			// BUG-011/012: wire Asynq client for activity feed fan-out
			// (content:fan_out_to_feeds) and Meilisearch indexing
			// (search:index_entity). Ref: Section 11, 12, 32.
			forumService.SetAsynqClient(asynqClient)
			// BUG-013: wire notification dispatcher for reply-to-author
			// notifications. Ref: Section 35.
			forumService.SetNotificationDispatcher(commentNotificationAdapter{svc: notificationService})
			forumHandlers := forum.NewHandlers(forumService, contentService)
			// Wire the persona action dispatcher (ADR-077 §5, BUG-010) now that
			// the forum service is created. The dispatcher routes approved
			// persona actions to the existing action endpoints (comments,
			// follows, content reactions/bookmarks, forum, customfields).
			participationService.SetDispatcher(&personaActionDispatcher{
				comments: commentsService,
				follows:  followService,
				content:  contentService,
				forum:    forumService,
				cf:       cfService,
			})
			// Wire mention notification dispatch for forum threads and replies
			// (Section 15 / ADR-029). Fire-and-forget per Rule 8.
			forumHandlers.SetMentionNotifier(commentNotificationAdapter{svc: notificationService})
			// Wire erased-mention redaction for GDPR/ADR-029 compliance.
			forumHandlers.SetMentionCheckers(mentionCheckers)
			// Wire RAG indexer so forum threads are embedded into
			// content_embeddings (§69/ADR-069 BUG-001 fix).
			forumHandlers.SetRAGIndexer(ragService)
			// BUG-002 (rag-audit): inject the settings reader so the forum
			// handler can resolve i18n.default_language for RAG indexing
			// instead of hardcoding "en".
			forumHandlers.SetSettings(settingsService)
			// §12 stale-index fix: wire Asynq so thread/tag CRUD enqueues
			// search:index_entity tasks.
			forumHandlers.SetAsynqClient(asynqClient)

			r.With(features.Gate(featureChecker, "feature.forum")).Group(func(r chi.Router) {
				// Thread routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.thread.create")).Post("/forum", forumHandlers.CreateThread)
				r.Get("/forum", forumHandlers.ListThreads)
				// Section 88 — AMA routes registered BEFORE /forum/{slug} to
				// avoid being shadowed by the wildcard route (chi matches in
				// registration order). Feature-gated via forum.ama.enabled (ADR-096).
				r.With(features.Gate(featureChecker, "forum.ama.enabled")).Group(func(r chi.Router) {
					r.Get("/forum/ama", amaHandlers.HandleList)
					r.Get("/forum/ama/{id}", amaHandlers.HandleGet)
					r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.ama.create")).
						Post("/forum/ama", amaHandlers.HandleCreate)
					// Close: host (author) OR forum.ama.moderate. The service
					// resolves authorship so the host can close without holding
					// the moderator grant (§88 "host or moderator").
					r.With(auth.RequireAuth(kr, vkClient.Redis())).
						Patch("/forum/ama/{id}", amaHandlers.HandleClose)
				})
				r.Get("/forum/{slug}", forumHandlers.GetThread)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.thread.edit_own")).
					Patch("/forum/{id}", forumHandlers.UpdateThread)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.thread.delete_own")).
					Delete("/forum/{id}", forumHandlers.DeleteThread)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.thread.solve")).
					Post("/forum/{id}/solve", forumHandlers.SolveThread)
				// BUG-016: the unsolve route uses RequireAuth only (not
				// RequirePermission) so the OP — a regular member without the
				// forum.thread.solve grant — can unsolve their own thread. The
				// service enforces author/space-moderator/permission-holder
				// scope. Ref: Section 08 "OP or moderator".
				r.With(auth.RequireAuth(kr, vkClient.Redis())).
					Delete("/forum/{id}/solve", forumHandlers.UnsolveThread)

				// Reply routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.reply.create")).Post("/forum/{id}/replies", forumHandlers.CreateReply)
				r.Get("/forum/{id}/replies", forumHandlers.ListReplies)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.reply.edit_own")).
					Patch("/forum/replies/{id}", forumHandlers.UpdateReply)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.reply.delete_own")).
					Delete("/forum/replies/{id}", forumHandlers.DeleteReply)

				// Vote routes — body-based (legacy) + URL-param (catalog).
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.vote")).Post("/forum/vote", forumHandlers.Vote)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.vote")).Post("/forum/{id}/vote", forumHandlers.VoteOnThread)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.vote")).Delete("/forum/{id}/vote", forumHandlers.RemoveThreadVote)

				// Poll routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.poll.create")).
					Post("/forum/polls", forumHandlers.CreatePoll)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.poll.create")).
					Post("/forum/{id}/poll", forumHandlers.CreatePollForThread)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.poll.vote")).Post("/forum/polls/{id}/vote", forumHandlers.VotePoll)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.poll.vote")).Post("/poll-options/{id}/vote", forumHandlers.VotePollOption)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.poll.vote")).Delete("/poll-options/{id}/vote", forumHandlers.RetractPollVoteHandler)
				r.Get("/forum/polls/{id}", forumHandlers.GetPoll)

				// Tag routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.tag.create")).
					Post("/forum/tags", forumHandlers.CreateTag)
				r.Get("/forum/tags", forumHandlers.ListTags)
				r.Get("/forum/tags/{slug}", forumHandlers.GetTagBySlug)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.tag.assign")).Post("/forum/{id}/tags", forumHandlers.AddThreadTagsBulk)
				r.Get("/forum/{id}/tags", forumHandlers.ListThreadTags)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.tag.assign")).Post("/forum/{id}/tags/{tagId}", forumHandlers.AddThreadTag)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.tag.assign")).Delete("/forum/{id}/tags/{tagId}", forumHandlers.RemoveThreadTag)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.tag.edit")).Patch("/forum/tags/{id}", forumHandlers.UpdateTag)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.tag.edit")).Delete("/forum/tags/{id}", forumHandlers.DeleteTag)

				// Bookmark routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.bookmark.manage")).Post("/forum/bookmarks", forumHandlers.AddBookmark)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.bookmark.manage")).Delete("/forum/bookmarks/{id}", forumHandlers.RemoveBookmark)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/forum/bookmarks", forumHandlers.ListBookmarks)
			})

			// Blog Extension routes — gated by feature.blog (Section 24)
			r.With(features.Gate(featureChecker, "feature.blog")).Group(func(r chi.Router) {
				// Base post CRUD — blog posts are content rows (content_type=
				// blog_post). BUG-001/BUG-012: use Section 24 granular blog.*
				// permission nodes (blog.create, blog.edit_own) instead of the
				// generic content.* nodes. The edit-window check in the content
				// service enforces own-vs-any scoping for blog.edit_own; admin
				// "edit any" is covered by PATCH /admin/blog/{id} (blog.review.manage).
				r.Get("/blog", blogHandlers.ListPosts)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "moderate"), spam.CensorMiddleware(database, vkClient), rbac.RequirePermission(rbacResolver, "blog.create")).
					Post("/blog", blogHandlers.CreatePost)
				r.Get("/blog/{slug}", blogHandlers.GetPost)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.EditLockMiddleware(vkClient.Redis(), settingsService), spam.CensorMiddleware(database, vkClient), rbac.RequirePermission(rbacResolver, "blog.edit_own")).
					Patch("/blog/{id}", contentHandlers.UpdateContent)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.EditLockMiddleware(vkClient.Redis(), settingsService), rbac.RequirePermission(rbacResolver, "blog.edit_own")).
					Delete("/blog/{id}", contentHandlers.DeleteContent)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.create")).
					Post("/blog/{id}/submit-for-review", blogHandlers.SubmitForReview)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.feature")).
					Post("/blog/{id}/feature", blogHandlers.FeaturePostByID)

				// Series routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.series.create")).
					Post("/blog/series", blogHandlers.CreateSeries)
				r.Get("/blog/series", blogHandlers.ListSeries)
				r.Get("/blog/series/{slug}", blogHandlers.GetSeries)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.series.edit")).
					Patch("/blog/series/{id}", blogHandlers.UpdateSeries)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.series.delete")).
					Delete("/blog/series/{id}", blogHandlers.DeleteSeries)

				// Series post routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.series.edit")).
					Post("/blog/series/{id}/posts", blogHandlers.AddPostToSeries)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.series.edit")).
					Delete("/blog/series/{id}/posts/{postId}", blogHandlers.RemovePostFromSeries)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.series.edit")).
					Patch("/blog/series/{id}/posts/reorder", blogHandlers.ReorderSeriesPosts)
				// List posts in a series (public — server-side fetched by frontend)
				r.Get("/blog/series/{id}/posts", blogHandlers.ListSeriesPosts)

				// Review routes (admin workflow — keep auth-required)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.review.create")).
					Post("/blog/reviews", blogHandlers.CreateReview)
					// BUG-006: GET /blog/reviews returns ListPendingReviewSummaries
				// (post title/slug/author/excerpt for in-review posts). Without
				// blog.review.manage any authenticated user could read draft
				// metadata of other users' submissions.
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.review.manage")).
					Get("/blog/reviews", blogHandlers.ListReviews)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.review.manage")).
					Patch("/blog/reviews/{id}", blogHandlers.UpdateReview)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.review.manage")).
					Delete("/blog/reviews/{id}", blogHandlers.DeleteReview)

				// Featured routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.feature")).
					Post("/blog/featured", blogHandlers.FeaturePost)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.feature")).
					Delete("/blog/featured/{contentId}", blogHandlers.UnfeaturePost)
				r.Get("/blog/featured", blogHandlers.ListFeatured)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.feature")).
					Patch("/blog/featured/reorder", blogHandlers.ReorderFeatured)

				// RSS feed (public — no auth required)
				r.Get("/blog/feed.xml", blogHandlers.GetRSSFeed)
			})

			// Universal Ratings & Reviews routes — gated by feature.ratings_reviews (Section 29)
			r.With(features.Gate(featureChecker, "feature.ratings_reviews")).Group(func(r chi.Router) {
				// catalog: /reviews/:entity_type/:entity_id[/aggregate]; bare /reviews
				// (reviewer-scoped list) kept as a documented extra. The bare
				// /reviews route is NOT marked [public] in the catalog (it
				// allows reviewer-scoped enumeration), so it requires auth.
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "reviews.create")).Post("/reviews/{entity_type}/{entity_id}", reviewHandlers.CreateReview)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/reviews", reviewHandlers.ListReviews)
				r.Get("/reviews/{entity_type}/{entity_id}", reviewHandlers.ListReviews)
				r.Get("/reviews/{entity_type}/{entity_id}/aggregate", reviewHandlers.GetAggregate)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/reviews/{id}", reviewHandlers.GetReview)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "reviews.edit_own")).Patch("/reviews/{id}", reviewHandlers.UpdateReview)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "reviews.delete_own")).Delete("/reviews/{id}", reviewHandlers.DeleteReview)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "reviews.flag")).Post("/reviews/{id}/flag", reviewHandlers.FlagReview)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "reviews.moderate")).Patch("/admin/reviews/{id}", reviewHandlers.ModerateReview)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "reviews.moderate")).Delete("/admin/reviews/{id}", reviewHandlers.HardDeleteReview)
			})

			// Generic Comparison System routes — gated by feature.comparison (Section 30)
			r.With(features.Gate(featureChecker, "feature.comparison")).Group(func(r chi.Router) {
				// Type definition routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comparison.type.create")).
					Post("/comparisons/types", comparisonHandlers.CreateComparisonType)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/comparisons/types", comparisonHandlers.ListComparisonTypes)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/comparisons/types/{slug}", comparisonHandlers.GetComparisonType)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comparison.type.edit")).
					Patch("/comparisons/types/{id}", comparisonHandlers.UpdateComparisonType)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comparison.type.delete")).
					Delete("/comparisons/types/{id}", comparisonHandlers.DeleteComparisonType)

				// Admin comparison-types aliases (P5.6 — Task 2, catalog paths)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comparison.type.create")).
					Post("/admin/comparison-types", comparisonHandlers.CreateComparisonType)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/admin/comparison-types", comparisonHandlers.ListComparisonTypes)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comparison.type.edit")).
					Patch("/admin/comparison-types/{id}", comparisonHandlers.UpdateComparisonType)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comparison.type.delete")).
					Delete("/admin/comparison-types/{id}", comparisonHandlers.DeleteComparisonType)

				// Type access routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comparison.type.access.manage")).
					Put("/comparisons/types/{id}/access", comparisonHandlers.SetComparisonTypeAccess)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/comparisons/types/{id}/access", comparisonHandlers.ListComparisonTypeAccess)

				// Comparison CRUD routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comparison.create")).
					Post("/comparisons", comparisonHandlers.CreateComparison)
				// Auto-generate dimensions from item names (Phase 16.05 — HITL, heuristic-based)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comparison.create")).Post("/comparisons/auto-dimensions", comparisonHandlers.AutoGenerateDimensions)
				// Dimension types config (Section 30) — returns valid dimension types
				// for the comparison builder selector (Rule 16 — no hardcoded enums).
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/comparisons/dimension-types", comparisonHandlers.GetDimensionTypes)
				// Public comparison reads (Section 30): published+public comparisons
				// are visible to all visitors; unpublished/non-public require author auth.
				r.With(auth.OptionalAuth(kr, vkClient.Redis())).Get("/comparisons/{id}", comparisonHandlers.GetComparison)
				r.With(auth.OptionalAuth(kr, vkClient.Redis())).Get("/comparisons/{id}/matrix", comparisonHandlers.GetComparisonMatrix)

				// Public comparison list (Section 30): SQL filters to published+public.
				r.Get("/comparisons", comparisonHandlers.ListComparisons)
				// Comparison update/delete at top level (P5.6 — Task 2, catalog paths)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequireAnyPermission(rbacResolver, "comparison.edit_own", "comparison.edit_any")).
					Patch("/comparisons/{id}", comparisonHandlers.UpdateComparison)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequireAnyPermission(rbacResolver, "comparison.delete_own", "comparison.delete_any")).
					Delete("/comparisons/{id}", comparisonHandlers.DeleteComparison)

				// Item routes — catalog uses /comparisons/:id for items
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comparison.item.manage")).
					Post("/comparisons/{id}/items", comparisonHandlers.CreateComparisonItem)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comparison.item.manage")).
					Post("/comparisons/{id}/items/reorder", comparisonHandlers.ReorderComparisonItems)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comparison.item.manage")).
					Patch("/comparisons/items/{id}", comparisonHandlers.UpdateComparisonItem)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comparison.item.manage")).
					Delete("/comparisons/items/{id}", comparisonHandlers.DeleteComparisonItem)
				// Catalog aliases for items at /comparisons/:id
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comparison.item.manage")).
					Patch("/comparisons/{id}/items/{itemId}", comparisonHandlers.UpdateComparisonItem)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comparison.item.manage")).
					Delete("/comparisons/{id}/items/{itemId}", comparisonHandlers.DeleteComparisonItem)

				// Dimension routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comparison.dimension.manage")).
					Post("/comparisons/{id}/dimensions", comparisonHandlers.CreateComparisonDimension)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comparison.dimension.manage")).
					Patch("/comparisons/dimensions/{id}", comparisonHandlers.UpdateComparisonDimension)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comparison.dimension.manage")).
					Delete("/comparisons/dimensions/{id}", comparisonHandlers.DeleteComparisonDimension)

				// Value routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comparison.value.manage")).
					Put("/comparisons/dimensions/{id}/values", comparisonHandlers.UpsertComparisonValues)
			})

			// Professional Profiles routes — gated by feature.professional (Section 33)
			r.With(features.Gate(featureChecker, "feature.professional")).Group(func(r chi.Router) {
				// Profile type routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.type.create")).
					Post("/profiles/types", professionalHandlers.CreateProfessionalProfileType)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.type.view")).Get("/profiles/types", professionalHandlers.ListProfessionalProfileTypes)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.type.view")).Get("/profiles/types/{slug}", professionalHandlers.GetProfessionalProfileType)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.type.create")).
					Patch("/profiles/types/{id}", professionalHandlers.UpdateProfessionalProfileType)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.type.create")).
					Delete("/profiles/types/{id}", professionalHandlers.DeleteProfessionalProfileType)

				// Profile CRUD routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.create")).
					Post("/profiles", professionalHandlers.CreateProfessionalProfile)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/profiles", professionalHandlers.ListProfessionalProfiles)
				// GET /profiles/{slug} uses OptionalAuth so guests can view
				// public professional profiles (Phase 12.08 public SSR). The
				// handler fetches by slug without needing a user context.
				// Ref: sections/33-professional-profiles.md
				r.With(auth.OptionalAuth(kr, vkClient.Redis())).Get("/profiles/{slug}", professionalHandlers.GetProfessionalProfile)
				// GET /profiles/{id}/content — public content list for a profile;
				// OptionalAuth so guests can browse a public profile's content
				// (consistent with GetProfessionalProfile). Ref: sections/33-professional-profiles.md
				r.With(auth.OptionalAuth(kr, vkClient.Redis())).Get("/profiles/{id}/content", professionalHandlers.ListProfileContent)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.edit_own")).
					Patch("/profiles/{id}", professionalHandlers.UpdateProfessionalProfile)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.delete_own")).
					Delete("/profiles/{id}", professionalHandlers.DeleteProfessionalProfile)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/profiles/{id}/follow", professionalHandlers.ToggleFollow)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/profiles/{id}/follow", professionalHandlers.ToggleFollow)

				// Member management routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.member.manage")).
					Post("/profiles/{id}/members", professionalHandlers.AddMember)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.member.manage")).
					Delete("/profiles/{id}/members/{userId}", professionalHandlers.RemoveMember)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/profiles/{id}/members", professionalHandlers.ListMembers)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.member.manage")).
					Patch("/profiles/{id}/members/{userId}", professionalHandlers.UpdateMemberRole)

				// Invitation routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.invite")).
					Post("/profiles/{id}/invitations", professionalHandlers.CreateInvitation)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/profiles/{id}/invitations", professionalHandlers.ListInvitations)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/profiles/invitations/{token}/accept", professionalHandlers.AcceptInvitation)

				// Author context route
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.author.switch")).
					Get("/profiles/{id}/author-context", professionalHandlers.GetAuthorContext)

				// Relocation routes (Section 33 — Professional Profiles extras)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/profiles/{id}/relocations", professionalHandlers.ListRelocations)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.edit_own")).
					Post("/profiles/{id}/relocations", professionalHandlers.CreateRelocation)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.edit_own")).
					Patch("/profiles/relocations/{relocationId}", professionalHandlers.UpdateRelocation)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.delete_own")).
					Delete("/profiles/relocations/{relocationId}", professionalHandlers.DeleteRelocation)

				// Document routes
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/profiles/{id}/documents", professionalHandlers.ListDocuments)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.edit_own")).
					Post("/profiles/{id}/documents", professionalHandlers.CreateDocument)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.edit_own")).
					Patch("/profiles/documents/{documentId}", professionalHandlers.UpdateDocument)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.delete_own")).
					Delete("/profiles/documents/{documentId}", professionalHandlers.DeleteDocument)

				// Verifier routes
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/profiles/{id}/verifiers", professionalHandlers.ListVerifiers)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.edit_own")).
					Post("/profiles/{id}/verifiers", professionalHandlers.CreateVerifier)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.delete_own")).
					Delete("/profiles/verifiers/{verifierId}", professionalHandlers.DeleteVerifier)
			})

			// Organization routes (Section 07 — Organization Profiles & Claim Ownership)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "organizations.create")).Post("/organizations", orgHandlers.CreateOrganization)
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), rbac.OptionalPermission(rbacResolver, "directory.organizations.view")).Get("/organizations", orgHandlers.ListPublicOrganizations)
			r.With(auth.OptionalAuth(kr, vkClient.Redis())).Get("/organizations/meta", orgHandlers.GetOrganizationMeta)
			r.With(auth.OptionalAuth(kr, vkClient.Redis())).Get("/organizations/{slug}", orgHandlers.GetPublicOrganization)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "organizations.edit_own")).Patch("/organizations/{id}", orgHandlers.UpdateOrganization)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "organizations.delete_own")).Delete("/organizations/{id}", orgHandlers.DeleteOrganization)
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), requireActiveOrgByID(orgService)).Get("/organizations/{id}/members", orgHandlers.ListPublicMembers)
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), requireActiveOrgByID(orgService)).Get("/organizations/{id}/content", contentHandlers.ListByOrganization)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), requireActiveOrgByID(orgService)).Get("/organizations/{id}/analytics", orgHandlers.GetOrganizationAnalytics)
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), requireActiveOrgByID(orgService)).Get("/organizations/{id}/media", mediaHandlers.ListByOrganization)
			// Org follow/unfollow (Section 31 — universal polymorphic follow) — M5: rate-limited
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "follows.toggle"), mw.RateLimit(vkClient.Redis(), "moderate")).Post("/organizations/{id}/follow", orgHandlers.ToggleFollow)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "follows.toggle"), mw.RateLimit(vkClient.Redis(), "moderate")).Delete("/organizations/{id}/follow", orgHandlers.ToggleFollow)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "organizations.members.manage")).Post("/organizations/{id}/members", orgHandlers.AddMember)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "organizations.members.manage")).Delete("/organizations/{id}/members/{userId}", orgHandlers.RemoveMember)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "organizations.members.manage")).Patch("/organizations/{id}/members/{userId}", orgHandlers.UpdateMemberRole)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "organizations.invite")).Post("/organizations/{id}/invitations", orgHandlers.InviteMember)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/organizations/{id}/invitations", orgHandlers.ListInvitations)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "organizations.invite")).Delete("/organizations/{id}/invitations/{invitationId}", orgHandlers.RevokeInvitation)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "organizations.invite")).Post("/organizations/invitations/{token}/accept", orgHandlers.AcceptInvitation)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "organizations.claim")).Post("/claim-ownership", orgHandlers.ClaimOwnership)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).Get("/claim-ownership", orgHandlers.ListClaims)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "organizations.claim")).Get("/claim-ownership/{id}", orgHandlers.GetClaim)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "organizations.claim")).Delete("/claim-ownership/{id}", orgHandlers.WithdrawClaim)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).Patch("/claim-ownership/{id}", orgHandlers.ReviewClaim)

			// Payment Core routes (Section 17 — Payment Core)
			// Payment initiation — idempotency middleware prevents duplicate charges
			// when clients retry a failed POST. See AUDIT-BE-004 / Rule 9.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "payments.create"),
				mw.IdempotencyMiddleware(vkClient.Redis(), settingsService)).
				Post("/payments/intent", paymentHandlers.InitiatePayment)
			// Payment webhook routes — tighter rate limit for provider callbacks.
			r.With(mw.RateLimit(vkClient.Redis(), "webhook")).Post("/payments/webhook/paystack", paymentHandlers.HandlePaystackWebhook)
			r.With(mw.RateLimit(vkClient.Redis(), "webhook")).Post("/payments/webhook/stripe", paymentHandlers.HandleStripeWebhook)
			r.With(mw.RateLimit(vkClient.Redis(), "webhook")).Post("/payments/webhook/nowpayments", paymentHandlers.HandleNOWPaymentsWebhook)
			// Payment listing and details (catalog: /me/payments)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/payments", paymentHandlers.ListPayments)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/payments/{id}", paymentHandlers.GetPayment)
			// Admin payment routes (Section 17 — Admin Payment Routes).
			// Gated by payments.refund (admin-only permission).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "payments.refund")).
				Get("/admin/payments", paymentHandlers.ListAdminPayments)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "payments.refund")).
				Get("/admin/payments/{id}", paymentHandlers.GetAdminPayment)
			// Refunds — idempotency middleware prevents duplicate refund processing. See AUDIT-BE-004 / Rule 9.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "payments.refund"),
				mw.IdempotencyMiddleware(vkClient.Redis(), settingsService)).
				Post("/payments/{id}/refund", paymentHandlers.RefundPayment)
			// Subscriptions — idempotency middleware prevents duplicate subscription creation/cancellation.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "payments.subscriptions.manage"),
				mw.IdempotencyMiddleware(vkClient.Redis(), settingsService)).
				Post("/subscriptions", paymentHandlers.CreateSubscription)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/subscriptions", paymentHandlers.ListSubscriptions)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/subscription", paymentHandlers.GetActiveSubscription)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.IdempotencyMiddleware(vkClient.Redis(), settingsService)).
				Post("/me/subscription/cancel", paymentHandlers.CancelMySubscription)
			// Discount codes — admin-only per catalog (/admin/discount-codes).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "payments.discount_codes.manage")).
				Post("/admin/discount-codes", paymentHandlers.CreateDiscountCode)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "payments.discount_codes.manage")).
				Get("/admin/discount-codes", paymentHandlers.ListDiscountCodes)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/payments/discount-codes/validate", paymentHandlers.ValidateDiscountCode)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "payments.discount_codes.manage")).
				Delete("/admin/discount-codes/{id}", paymentHandlers.DeleteDiscountCode)
			// NOTE (P5): PATCH /admin/discount-codes/:id and GET .../usage
			// deferred — no UpdateDiscountCode or usage-tracking service method exists.

			// Pricing and payment providers (public — catalog: /pricing/plans, /payment-providers)
			r.Get("/pricing/plans", paymentHandlers.ListPricingPlans)
			r.Get("/payment-providers", paymentHandlers.ListPaymentProviders)

			// Admin: Subscription Plans CRUD (catalog: /admin/plans).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "payments.plans.manage")).
				Get("/admin/plans", paymentHandlers.ListAdminPlans)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "payments.plans.manage")).
				Post("/admin/plans", paymentHandlers.CreateAdminPlan)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "payments.plans.manage")).
				Patch("/admin/plans/{id}", paymentHandlers.UpdateAdminPlan)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "payments.plans.manage")).
				Delete("/admin/plans/{id}", paymentHandlers.DeleteAdminPlan)

			// Admin: Subscription Plan ↔ Track management (migration 000273).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "payments.plans.manage")).
				Get("/admin/plans/{id}/tracks", paymentHandlers.AdminListPlanTracks)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "payments.plans.manage")).
				Put("/admin/plans/{id}/tracks", paymentHandlers.AdminSetPlanTracks)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "payments.plans.manage")).
				Post("/admin/plans/{id}/tracks/{trackId}", paymentHandlers.AdminAddTrackToPlan)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "payments.plans.manage")).
				Delete("/admin/plans/{id}/tracks/{trackId}", paymentHandlers.AdminRemoveTrackFromPlan)

			// Public: list tracks included in a plan (catalog: /pricing/plans/{slug}/tracks).
			r.Get("/pricing/plans/{slug}/tracks", paymentHandlers.PublicListPlanTracks)

			// Wallet & Credits routes — gated by feature.wallet (Section 28)
			r.With(features.Gate(featureChecker, "feature.wallet")).Group(func(r chi.Router) {
				// Balance and ledger (catalog: /me/wallet)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wallet.points.balance")).
					Get("/me/wallet", walletHandlers.GetBalance)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wallet.points.balance")).
					Get("/me/wallet/points", walletHandlers.GetPointsBalance)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wallet.points.balance")).
					Get("/me/wallet/credits", walletHandlers.GetCreditsBalance)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wallet.points.balance")).
					Get("/me/wallet/ledger", walletHandlers.GetLedger)
					// Member-facing, read-only conversion rate + min convert (catalog: /me/wallet/settings).
				// Reuses wallet.points.balance — anyone who can view their balance can view the rate.
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wallet.points.balance")).
					Get("/me/wallet/settings", walletHandlers.GetMemberSettings)
					// Conversion — idempotency middleware prevents duplicate point-to-credit conversions. See AUDIT-BE-004 / Rule 9.
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wallet.points.convert"),
					mw.IdempotencyMiddleware(vkClient.Redis(), settingsService)).
					Post("/me/wallet/convert", walletHandlers.ConvertPoints)
				// Settings (admin)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wallet.admin.settings.manage")).
					Get("/admin/wallet/settings", walletHandlers.GetSettings)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wallet.admin.settings.manage")).
					Put("/admin/wallet/settings", walletHandlers.UpdateSettings)
				// Activity rules (admin)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wallet.admin.rules.manage")).
					Get("/admin/wallet/rules", walletHandlers.ListRules)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wallet.admin.rules.manage")).
					Post("/admin/wallet/rules", walletHandlers.CreateOrUpdateRule)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wallet.admin.rules.manage")).
					Patch("/admin/wallet/rules/{id}/toggle", walletHandlers.ToggleRule)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wallet.admin.rules.manage")).
					Delete("/admin/wallet/rules/{id}", walletHandlers.DeleteRule)
				// Admin adjustments
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wallet.admin.adjust")).
					Post("/admin/wallet/adjust", walletHandlers.AdminAdjust)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wallet.admin.adjust")).
					Get("/admin/wallet/user/{userId}/balance", walletHandlers.AdminGetUserBalance)

				// Credit Currency System (Section 58, ADR-065)
				// Public: currencies + credit packs + purchase
				r.With(auth.RequireAuth(kr, vkClient.Redis())).
					Get("/wallet/currencies", walletHandlers.ListCurrencies)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).
					Get("/wallet/credit-packs", walletHandlers.ListCreditPacks)
				// Purchase credits — idempotency middleware prevents duplicate
				// payment rows / double-charges when clients retry a failed POST.
				// See AUDIT-BE-004 / Rule 9 (SET NX EX on financial endpoints).
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wallet.credits.purchase"),
					mw.IdempotencyMiddleware(vkClient.Redis(), settingsService)).
					Post("/wallet/purchase-credits", walletHandlers.PurchaseCredits)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).
					Patch("/me/preferences/currency", walletHandlers.SetPreferredCurrency)

				// Admin Panel: credit pack CRUD + purchase history

				// Admin Panel: credit pack CRUD + purchase history
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wallet.credit_pack.manage")).
					Get("/admin/wallet/credit-packs", walletHandlers.AdminListCreditPacks)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wallet.credit_pack.manage")).
					Post("/admin/wallet/credit-packs", walletHandlers.AdminCreateCreditPack)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wallet.credit_pack.manage")).
					Patch("/admin/wallet/credit-packs/{id}", walletHandlers.AdminUpdateCreditPack)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wallet.credit_pack.manage")).
					Delete("/admin/wallet/credit-packs/{id}", walletHandlers.AdminDeleteCreditPack)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wallet.purchases.view_any")).
					Get("/admin/wallet/purchases", walletHandlers.AdminListCreditPurchases)
			})

			// Advertising routes (Section 25 — Ad Placement)
			// Admin campaign management
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.campaign.manage")).
				Post("/admin/ads/campaigns", adHandlers.CreateCampaign)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.campaign.manage")).
				Get("/admin/ads/campaigns", adHandlers.ListCampaigns)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.campaign.manage")).
				Get("/admin/ads/campaigns/{id}", adHandlers.GetCampaign)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.campaign.manage")).
				Post("/admin/ads/campaigns/{id}/activate", adHandlers.ActivateCampaign)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.campaign.manage")).
				Post("/admin/ads/campaigns/{id}/pause", adHandlers.PauseCampaign)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.campaign.manage")).
				Post("/admin/ads/campaigns/{id}/end", adHandlers.EndCampaign)
			// §61: Reject a pending-review self-serve campaign (admin moderation).
			// Enqueues ads:refund_campaign_credits + ads:notify_campaign_status.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.campaign.manage")).
				Post("/admin/ads/campaigns/{id}/reject", adHandlers.RejectCampaign)
			// Public ad serving and event tracking (catalog: /ads/serve, /ads/events —
			// §8: reachable by guests, OptionalAuth so a signed-in user's frequency
			// cap still applies; usergroup targeting itself is deferred to P5).
			// Gated by feature.ads flag (default: false).
			r.With(features.Gate(featureChecker, "feature.ads"), auth.OptionalAuth(kr, vkClient.Redis())).
				Get("/ads/serve", adHandlers.RenderSlot)
			r.With(features.Gate(featureChecker, "feature.ads"), auth.OptionalAuth(kr, vkClient.Redis())).
				Post("/ads/events", adHandlers.IngestEvents)
			// Slot types (admin)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.slot.manage")).
				Get("/admin/ads/slot-types", adHandlers.ListSlotTypes)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.slot.manage")).
				Post("/admin/ads/slot-types", adHandlers.CreateSlotType)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.slot.manage")).
				Put("/admin/ads/slot-types/{id}", adHandlers.UpdateSlotType)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.slot.manage")).
				Delete("/admin/ads/slot-types/{id}", adHandlers.DeleteSlotType)

			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.campaign.manage")).
				Delete("/admin/ads/campaigns/{id}", adHandlers.DeleteCampaign)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.stats.view")).
				Get("/admin/ads/campaigns/{id}/stats", adHandlers.GetCampaignStats)
			// Organization boosts
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.campaign.manage")).
				Post("/admin/ads/boosts", adHandlers.CreateBoost)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.campaign.manage")).
				Get("/admin/ads/boosts", adHandlers.ListBoosts)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.campaign.manage")).
				Get("/admin/ads/boosts/active", adHandlers.ListActiveBoosts)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.campaign.manage")).
				Post("/admin/ads/boosts/{id}/end", adHandlers.EndBoost)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.campaign.manage")).
				Patch("/admin/ads/boosts/{id}", adHandlers.UpdateBoost)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.campaign.manage")).
				Delete("/admin/ads/boosts/{id}", adHandlers.DeleteBoost)
			// Ad Network Config (public — no auth required)
			r.Get("/ads/networks/config", adHandlers.GetAdNetworkConfig)
			// Ad Networks (Section 25 — external ad network configuration)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.network.manage")).
				Get("/admin/ads/networks", adHandlers.ListAdNetworks)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.network.manage")).
				Post("/admin/ads/networks", adHandlers.CreateAdNetwork)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.network.manage")).
				Get("/admin/ads/networks/{id}", adHandlers.GetAdNetwork)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.network.manage")).
				Put("/admin/ads/networks/{id}", adHandlers.UpdateAdNetwork)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.network.manage")).
				Delete("/admin/ads/networks/{id}", adHandlers.DeleteAdNetwork)

			// Ticket System routes (Section 38 — Ticket System)
			// Feature-flagged via features.Gate (returns 404 when disabled, ADR-042).
			// User portal (catalog: /tickets, /me/tickets)
			r.With(features.Gate(featureChecker, "feature.tickets")).Group(func(r chi.Router) {
				r.Get("/ticket-types", ticketHandlers.ListTicketTypes)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.submit")).
					Post("/tickets", ticketHandlers.CreateTicket)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.view_own")).
					Get("/me/tickets", ticketHandlers.ListMyTickets)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.view_own")).
					Get("/me/tickets/{id}", ticketHandlers.GetMyTicket)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.submit")).
					Post("/me/tickets/{id}/comments", ticketHandlers.AddReply)

				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.view_own")).
					Post("/me/tickets/{id}/attachments", ticketHandlers.AddAttachment)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.view_own")).
					Get("/me/tickets/{id}/attachments", ticketHandlers.ListAttachments)

				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.view_all")).
					Get("/admin/tickets", ticketHandlers.ListTickets)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.view_all")).
					Get("/admin/tickets/{id}", ticketHandlers.GetTicket)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.manage")).
					Patch("/admin/tickets/{id}", ticketHandlers.UpdateTicket)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.comment.internal")).
					Post("/admin/tickets/{id}/notes", ticketHandlers.AddInternalNote)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.view_all")).
					Get("/admin/tickets/{id}/events", ticketHandlers.GetTicketEvents)
				// Catalog: /admin/tickets/:id/comments accepts { body, is_internal }
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.comment.internal")).
					Post("/admin/tickets/{id}/comments", ticketHandlers.AddInternalNote)
				// Delete ticket (admin, spam/test only)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.admin")).
					Delete("/admin/tickets/{id}", ticketHandlers.DeleteTicket)
				// Resolve ticket
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.manage")).
					Post("/admin/tickets/{id}/resolve", ticketHandlers.ResolveTicket)

				// Ticket type management
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.type.manage")).
					Get("/admin/ticket-types", ticketHandlers.ListTicketTypes)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.type.manage")).
					Post("/admin/ticket-types", ticketHandlers.CreateTicketType)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.type.manage")).
					Put("/admin/ticket-types/{id}", ticketHandlers.UpdateTicketType)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.type.manage")).
					Delete("/admin/ticket-types/{id}", ticketHandlers.DeleteTicketType)
			})

			// i18n Translation management (admin) — catalog: /admin/i18n/*.
			// Manual CRUD + AI-driven translate routes. The AI translate
			// routes (POST .../translate, POST .../ai-translate-batch) are
			// registered further below alongside the other Task-25 routes.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "i18n.manage")).
				Get("/admin/i18n/strings", i18nHandlers.ListByLocale)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "i18n.manage")).
				Get("/admin/i18n/languages", i18nHandlers.ListLocales)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "i18n.manage")).
				Post("/admin/i18n/strings", i18nHandlers.Upsert)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "i18n.manage")).
				Post("/admin/i18n/strings/batch", i18nHandlers.UpsertBatch)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "i18n.manage")).
				Delete("/admin/i18n/strings/{id}", i18nHandlers.Delete)
			// Delete an entire locale and all its translations (BE-i18n-004).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "i18n.manage")).
				Delete("/admin/i18n/languages/{locale}", i18nHandlers.DeleteByLocale)

			// i18n public endpoints (no auth) — catalog: /i18n/packs, /i18n/packs/:lang_code
			r.Get("/i18n/packs", i18nHandlers.HandleListPublicLocales)
			r.Get("/i18n/packs/{lang_code}", i18nHandlers.GetLocaleTranslations)

			// GDPR / NDPR Compliance routes (Section 19)
			// User-facing endpoints
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Post("/consent", gdprHandlers.RecordConsent)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/consent", gdprHandlers.GetUserConsents)
			// BUG-015/BUG-016: consent types + policy version config endpoint
			// so the frontend does not hardcode them.
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/consent/config", gdprHandlers.GetConsentConfig)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Delete("/consent/{type}", gdprHandlers.RevokeConsent)
			// NOTE (§6.1/§6.2 dedupe): data-export and account-deletion are served
			// by the real GDPR handlers under /me/data-export and DELETE /me (below).
			// The former POST /account/export and POST /account/delete are removed to
			// avoid duplicate routes and the users-package stubs that did nothing.

			// Admin endpoints
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gdpr.audit_logs.view")).
				Get("/admin/audit-logs", gdprHandlers.ListAuditLogs)

			// Moderation queue — Content Reports (Section 51)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "reports.review")).
				Get("/admin/reports", reportsHandlers.ListReports)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "reports.review")).
				Get("/admin/reports/{id}", reportsHandlers.GetReport)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "reports.review")).
				Patch("/admin/reports/{id}", reportsHandlers.UpdateReport)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "reports.review")).
				Delete("/admin/reports/{id}", reportsHandlers.DeleteReport)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gdpr.consents.view")).
				Get("/admin/consents", gdprHandlers.ListConsentsByTenant)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gdpr.consents.delete")).
				Delete("/admin/consents/{id}", gdprHandlers.DeleteConsent)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gdpr.data_exports.view")).
				Get("/admin/gdpr/exports", gdprHandlers.ListPendingDataExports)
			// Platform data exports (Section 67) — admin-initiated exports of
			// platform data in CSV/JSON/XLSX. Separate from GDPR per-user
			// exports at /admin/gdpr/exports.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gdpr.data_exports.view")).
				Get("/admin/data-exports", platformDataExportHandlers.HandleListExports)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gdpr.data_exports.view")).
				Post("/admin/data-exports", platformDataExportHandlers.HandleCreateExport)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gdpr.data_exports.view")).
				Get("/admin/data-exports/{id}", platformDataExportHandlers.HandleGetExport)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gdpr.data_exports.view")).
				Delete("/admin/data-exports/{id}", platformDataExportHandlers.HandleDeleteExport)
			// Download endpoint for completed export files (BUG-006). Serves
			// the generated file from the storage backend, gated by the same
			// permission as the other export routes.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gdpr.data_exports.view")).
				Get("/admin/data-exports/{id}/download", platformDataExportHandlers.HandleDownloadExport)
			// Platform data imports (Section 67) — admin-initiated imports of
			// platform data from CSV/JSON files.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gdpr.data_exports.view")).
				Get("/admin/data-imports", platformDataImportHandlers.HandleListImports)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gdpr.data_exports.view")).
				Post("/admin/data-imports", platformDataImportHandlers.HandleCreateImport)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gdpr.data_exports.view")).
				Get("/admin/data-imports/{id}", platformDataImportHandlers.HandleGetImport)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gdpr.data_exports.view")).
				Delete("/admin/data-imports/{id}", platformDataImportHandlers.HandleDeleteImport)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Get("/admin/deletions", adminHandlers.ListDeletedContents)

			// Bot & Crawler Management routes (Section 43)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "bots.admin")).
				Get("/admin/bots", botHandlers.ListKnownBots)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "bots.admin")).
				Get("/admin/bots/{id}", botHandlers.GetKnownBot)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "bots.admin")).
				Post("/admin/bots", botHandlers.CreateKnownBot)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "bots.admin")).
				Put("/admin/bots/{id}", botHandlers.UpdateKnownBot)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "bots.admin")).
				Delete("/admin/bots/{id}", botHandlers.DeleteKnownBot)

			// Spam Prevention & Content Safety admin routes (Section 41)
			// IP block management
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spam.ip_block.manage")).
				Get("/admin/spam/ip-blocks", spamHandlers.ListIPBlocks)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spam.ip_block.manage")).
				Post("/admin/spam/ip-blocks", spamHandlers.CreateIPBlock)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spam.ip_block.manage")).
				Patch("/admin/spam/ip-blocks/{id}", spamHandlers.UpdateIPBlock)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spam.ip_block.manage")).
				Delete("/admin/spam/ip-blocks/{id}", spamHandlers.DeleteIPBlock)
			// Censored word management
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spam.censored_words.manage")).
				Get("/admin/spam/censored-words", spamHandlers.ListCensoredWords)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spam.censored_words.manage")).
				Post("/admin/spam/censored-words", spamHandlers.CreateCensoredWord)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spam.censored_words.manage")).
				Patch("/admin/spam/censored-words/{id}", spamHandlers.UpdateCensoredWord)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spam.censored_words.manage")).
				Delete("/admin/spam/censored-words/{id}", spamHandlers.DeleteCensoredWord)
			// User block management
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spam.user_block.manage")).
				Get("/admin/spam/user-blocks", spamHandlers.ListUserBlocks)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spam.user_block.manage")).
				Delete("/admin/spam/user-blocks/{blockerId}/{blockedId}", spamHandlers.DeleteUserBlock)

			// Platform Analytics & Site Reports routes (Section 40)
			// User dashboard
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Post("/analytics/events", analyticsHandlers.TrackEvent)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/analytics/dashboard", analyticsHandlers.GetUserDashboard)

			// Admin endpoints
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "analytics.admin")).
				Get("/admin/analytics/overview", analyticsHandlers.GetAdminOverview)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "analytics.admin")).
				Get("/admin/analytics/events", analyticsHandlers.ListEvents)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "analytics.admin")).
				Get("/admin/analytics/security", analyticsHandlers.ListSecurityScanResults)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "analytics.admin")).
				Post("/admin/analytics/security/{id}/resolve", analyticsHandlers.ResolveSecurityScanResult)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "analytics.admin")).
				Post("/admin/analytics/refresh", analyticsHandlers.RefreshViews)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "analytics.admin")).
				Post("/admin/analytics/security/scan", analyticsHandlers.RunSecurityScan)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "analytics.cache.flush")).
				Post("/admin/analytics/cache/flush", analyticsHandlers.HandleCacheFlush)

			// Admin Impersonation routes (Section 46)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.impersonate")).
				Post("/admin/impersonate", impersonationHandlers.StartImpersonation)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.impersonate")).
				Delete("/admin/impersonate", impersonationHandlers.EndImpersonation)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.impersonate")).
				Get("/admin/impersonate/status", impersonationHandlers.GetImpersonationStatus)

			// Developer Portal — Swagger UI (public, no auth). The spec it fetches
			// is the canonical /docs/openapi.json (§6.4 dedupe — no second route).
			r.Get("/developer/swagger", publicapi.HandleSwaggerUI(cfg.SiteName))
			// Embedded Swagger UI assets (go:embed, no CDN — PUB-016, §47).
			r.Handle("/developer/swagger/assets/*",
				http.StripPrefix("/developer/swagger/assets", publicapi.HandleSwaggerUIAssets()))

			// Developer Portal — API Key management (Section 47)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "api.keys.self_manage")).
				Get("/me/api-keys", publicAPIHandlers.ListKeys)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "api.keys.self_manage")).
				Post("/me/api-keys", publicAPIHandlers.CreateKey)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "api.keys.self_manage")).
				Delete("/me/api-keys/{id}", publicAPIHandlers.RevokeKey)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "api.keys.self_manage")).
				Post("/me/api-keys/{id}/rotate", publicAPIHandlers.RotateKey)

			// IP Allowlist for API keys (Section 47)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "api.keys.self_manage")).
				Get("/me/api-keys/{id}/ip-allowlist", publicAPIHandlers.ListAllowedIPs)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "api.keys.self_manage")).
				Post("/me/api-keys/{id}/ip-allowlist", publicAPIHandlers.AddAllowedIP)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "api.keys.self_manage")).
				Delete("/me/api-keys/{id}/ip-allowlist/{ipId}", publicAPIHandlers.RemoveAllowedIP)

			// Public API resource endpoints (Section 47 — api.japaconnect.com).
			// Gated by API key auth via X-API-Key header. These are read-only
			// endpoints for external integrations.
			//
			// Mounted under /public/ to avoid chi radix-tree shadowing: the main
			// API registers /content/{slug}, /spaces, /spaces/{slug}, /tags,
			// /users/{id}, /search, and /me on the same router. Chi v5 treats
			// {slug} and {id} as the same wildcard pattern, so registering both
			// /content/{slug} and /content/{id} on the same tree causes the
			// second registration to silently overwrite the first. The /public/
			// prefix keeps the public API routes at a distinct path that does
			// not collide with the main API routes.
			r.Group(func(r chi.Router) {
				r.Use(publicapi.APIKeyMiddleware(publicAPIService))
				r.Use(publicapi.SlidingWindowRateLimitWithSettings(vkClient.Redis(), settingsService))

				// Scope enforcement (Section 47 lines 169-182, BUG-003 fix:
				// RequireScope was defined but never wired — all public
				// endpoints accepted any valid key regardless of scopes).
				// Each endpoint requires its specific api.read.* scope. Keys
				// with the default api.read.* scope pass all checks via
				// wildcard matching in hasScope().
				r.With(publicapi.RequireScope("api.read.content")).
					Get("/public/content", publicAPIHandlers.ListContent)
				r.With(publicapi.RequireScope("api.read.content")).
					Get("/public/content/{id}", publicAPIHandlers.GetContentByID)
				r.With(publicapi.RequireScope("api.read.content")).
					Get("/public/content/{id}/comments", publicAPIHandlers.ListContentComments)
				r.With(publicapi.RequireScope("api.read.spaces")).
					Get("/public/spaces", publicAPIHandlers.ListSpaces)
				r.With(publicapi.RequireScope("api.read.spaces")).
					Get("/public/spaces/{slug}", publicAPIHandlers.GetSpaceBySlug)
				r.With(publicapi.RequireScope("api.read.tags")).
					Get("/public/tags", publicAPIHandlers.ListTags)
				r.With(publicapi.RequireScope("api.read.tags")).
					Get("/public/tags/{slug}", publicAPIHandlers.GetTagBySlug)
				r.With(publicapi.RequireScope("api.read.users.public")).
					Get("/public/users", publicAPIHandlers.ListUsers)
				r.With(publicapi.RequireScope("api.read.users.public")).
					Get("/public/users/{id}", publicAPIHandlers.GetUserByID)
				r.With(publicapi.RequireScope("api.read.content")).
					Get("/public/search", publicAPIHandlers.SearchContent)
				r.With(publicapi.RequireScope("api.read.me"), csrf.EnsureCookie(cfg.CookieDomain, cfg.IsProduction())).
					Get("/public/me", publicAPIHandlers.GetMyProfile)

				// Trending endpoints (Section 47 — public read-only snapshots)
				// Mounted under /public/trending/ to avoid chi radix-tree
				// collision with the existing /trending/* routes (line ~2246).
				r.With(publicapi.RequireScope("api.read.content")).
					Get("/public/trending/content", publicAPIHandlers.GetTrendingContentHandler)
				r.With(publicapi.RequireScope("api.read.tags")).
					Get("/public/trending/tags", publicAPIHandlers.GetTrendingTagsHandler)
				r.With(publicapi.RequireScope("api.read.spaces")).
					Get("/public/trending/spaces", publicAPIHandlers.GetTrendingSpacesHandler)
				r.With(publicapi.RequireScope("api.read.users.public")).
					Get("/public/trending/users", publicAPIHandlers.GetTrendingUsersHandler)
			})

			// User Profile & Preferences routes (catalog: /me, /me/profile)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), csrf.EnsureCookie(cfg.CookieDomain, cfg.IsProduction())).Get("/me", usersHandlers.GetMyProfile)
			// Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md (user.profile.edit_own)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "user.profile.edit_own")).Patch("/me/profile", usersHandlers.UpdateMyProfile)
			// NOTE (§6.3 dedupe): /me/preferences overlaps /me/notification-preferences;
			// kept as a documented extra pending confirmation of distinct semantics.
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/preferences", usersHandlers.GetMyPreferences)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Put("/me/preferences", usersHandlers.UpdateMyPreferences)

			// User /me/ account management routes (Section 50)
			// Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md (user.account.manage_own)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "user.account.manage_own")).Post("/me/change-password", usersHandlers.ChangePassword)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/privacy", usersHandlers.GetPrivacy)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Patch("/me/privacy", usersHandlers.UpdatePrivacy)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/sessions", usersHandlers.ListSessions)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/me/sessions/{id}", usersHandlers.RevokeSession)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/me/sessions", usersHandlers.RevokeAllSessions)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/connected-accounts", usersHandlers.ListConnectedAccounts)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/me/connected-accounts/{provider}", usersHandlers.DisconnectAccount)

			// TOTP 2FA (catalog: /me/totp/* — moved out of the /auth group)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/totp/status", totpHandlers.HandleStatus)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/me/totp/enable", totpHandlers.HandleSetup)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/me/totp/confirm", totpHandlers.HandleVerify)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/me/totp", totpHandlers.HandleDisable)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/content", usersHandlers.ListMyContent)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/content/{id}", usersHandlers.GetMyContent)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/drafts", usersHandlers.ListMyDrafts)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/drafts/{id}", usersHandlers.GetMyDraft)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/scheduled", usersHandlers.ListMyScheduled)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/bookmarks", usersHandlers.ListMyBookmarks)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/me/bookmarks/{id}", usersHandlers.DeleteMyBookmark)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/organizations", orgHandlers.ListMyOrganizations)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/bookmark-folders", bookmarkFoldersHandlers.ListFolders)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "bookmarks.manage")).Post("/me/bookmark-folders", bookmarkFoldersHandlers.CreateFolder)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "bookmarks.manage")).Patch("/me/bookmark-folders/{id}", bookmarkFoldersHandlers.UpdateFolder)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "bookmarks.manage")).Delete("/me/bookmark-folders/{id}", bookmarkFoldersHandlers.DeleteFolder)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/analytics", usersHandlers.GetMyAnalytics)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/analytics/content", usersHandlers.GetMyContentAnalytics)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/analytics/followers", usersHandlers.GetMyFollowerGrowth)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/me/deactivate", usersHandlers.DeactivateAccount)
			// Geo-aware location endpoints (ADR-064a): POST sets the user's
			// current_geo_space_id from browser geolocation or Space picker;
			// GET returns the resolved location. Both require auth.
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/me/location", usersHandlers.UpdateMyLocation)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/location", usersHandlers.GetMyLocation)
			// GDPR deletion + data export — served by the REAL gdpr handlers
			// (queue-backed export via data:export_user_data + Honcho hard erasure),
			// not the users-package stubs. Consolidated here from /account/* (§6).
			// Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md (user.account.delete_own, user.data.export_own)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "user.account.delete_own")).Delete("/me", gdprHandlers.RequestAccountDeletion)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "user.data.export_own"), mw.RateLimit(vkClient.Redis(), "moderate")).Post("/me/data-export", gdprHandlers.RequestDataExport)
			// Cancel a pending/processing data export request.
			// Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/me/data-export", gdprHandlers.CancelMyDataExport)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/data-export/status", gdprHandlers.GetMyDataExportStatus)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/data-export/download", gdprHandlers.GetMyDataExportDownload)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "reports.view_own")).Get("/me/reports", reportsHandlers.ListMyReports)
			// Public user directory — PII-safe (no email/status/groups). OptionalAuth
			// so authed users get their session without a 401. OptionalPermission
			// lets admins revoke directory.users.view from a group to hide /users
			// from that group's authed members (guests always pass through).
			// Admin listing stays at GET /admin/users with admin.users.list.
			// Ref: japaconnect-engineering/sections/02-authentication-sessions.md
			//      japaconnect-engineering/sections/03-rbac.md (Directory visibility nodes)
			//      japaconnect-engineering/sections/19-privacy-gdpr.md (search_indexable opt-out)
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), rbac.OptionalPermission(rbacResolver, "directory.users.view")).Get("/users", usersHandlers.ListPublicUsers)
			// Public per Section 32:12 (profile timeline audience = "Anyone").
			r.With(auth.OptionalAuth(kr, vkClient.Redis())).Get("/users/{username}", usersHandlers.GetUserByUsername)
			// Public profile Media tab (Task A3) — same audience rule as above.
			// Gated by requireActiveProfileUser: 404s for banned/suspended/deleted users.
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), requireActiveProfileUser(usersService, "id")).Get("/users/{id}/media", mediaHandlers.ListByUser)
			// Public profile Spaces tab — public+active spaces only; gated to 404 non-active users.
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), requireActiveProfileUser(usersService, "id")).Get("/users/{id}/spaces", spaceHandlers.ListByMemberUser)
			// Public profile Certificates tab — earned-certificate badges; gated to 404 non-active users.
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), requireActiveProfileUser(usersService, "id")).Get("/users/{id}/certificates", learningHandlers.ListCertificatesByUser)
			// Public profile Likes tab — published+public content the user reacted to; gated to 404 non-active users.
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), requireActiveProfileUser(usersService, "id")).Get("/users/{id}/likes", contentHandlers.ListLikedByUser)
			// Public profile Replies tab — user's comments on published+public content; gated to 404 non-active users.
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), requireActiveProfileUser(usersService, "id")).Get("/users/{id}/replies", commentsHandlers.ListByAuthor)
			// Admin user routes (catalog: /admin/users)
			// BUG-017 through BUG-021: config endpoint surfacing valid user
			// statuses, profile types, violation types, restriction operations,
			// and restriction durations so the admin frontend does not hardcode them.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.users.list")).Get("/admin/users/config", usersHandlers.GetUserModerationConfig)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.users.list")).Get("/admin/users", usersHandlers.ListUsers)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.users.manage")).Post("/admin/users", adminHandlers.CreateUser)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.users.list")).Get("/admin/users/{id}", usersHandlers.GetAdminUser)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.users.manage")).Get("/admin/users/{id}/sessions", usersHandlers.ListUserSessions)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.users.manage")).Get("/admin/users/{id}/activity", usersHandlers.ListUserActivity)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "user.delete")).Delete("/admin/users/{id}", usersHandlers.DeleteUser)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "user.delete")).Delete("/admin/users/{id}/hard", adminHandlers.HardDeleteUser)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.users.manage")).Patch("/admin/users/{id}/profile-type", usersHandlers.UpdateProfileType)

			// Onboarding routes (Section 09)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/onboarding/status", onboardingHandlers.GetStatus)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Put("/onboarding/step", onboardingHandlers.UpdateStep)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/onboarding/complete", onboardingHandlers.Complete)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/onboarding/recommendations", onboardingHandlers.GetRecommendations)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/onboarding/options", onboardingHandlers.GetOptions)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/onboarding/tour/dismiss", onboardingHandlers.DismissTour)

			// Admin onboarding settings
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings.view")).Get("/admin/onboarding/settings", onboardingHandlers.GetAdminSettings)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings")).Put("/admin/onboarding/settings", onboardingHandlers.UpdateAdminSettings)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings.view")).Get("/admin/onboarding/templates", onboardingHandlers.ListTemplates)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings.view")).Get("/admin/onboarding/templates/{id}", onboardingHandlers.GetTemplate)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings")).Post("/admin/onboarding/templates", onboardingHandlers.CreateTemplate)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings")).Patch("/admin/onboarding/templates/{id}", onboardingHandlers.UpdateTemplate)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings")).Delete("/admin/onboarding/templates/{id}", onboardingHandlers.DeleteTemplate)

			// Search routes — /search uses OptionalAuth so guests can search
			// published content (Phase 10.01 specifies public SSR search). The
			// handler degrades gracefully when no user is present (skips
			// user-scoped analytics logging). Ref: sections/13-search.md
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "moderate")).Get("/search", searchHandlers.Search)
			// Rate-limit autocomplete to prevent enumeration/abuse (SEC-010).
			r.With(mw.RateLimit(vkClient.Redis(), "moderate")).Get("/search/autocomplete", searchHandlers.Autocomplete)
			// catalog: GET /spaces/search — space-discovery search (distinct from
			// /spaces/{id}/search below, which searches one space's content).
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "moderate")).Get("/spaces/search", searchHandlers.SearchSpaces)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/spaces/{id}/search", searchHandlers.SearchSpace)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/tags/{slug}/search", searchHandlers.SearchTag)

			// General Tags (Sections 08, 49) — distinct from forum tags.
			// OptionalPermission lets admins revoke directory.tags.view from a
			// group to hide /tags from that group's authed members.
			// Ref: japaconnect-engineering/sections/03-rbac.md (Directory visibility nodes)
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), rbac.OptionalPermission(rbacResolver, "directory.tags.view")).Get("/tags", tagsHandlers.ListTags)
			r.Get("/tags/{slug}", tagsHandlers.GetTag)
			r.Get("/tags/{slug}/content", tagsHandlers.GetTagContent)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "moderate")).Post("/tags/{slug}/follow", tagsHandlers.ToggleTagFollow)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "moderate")).Delete("/tags/{slug}/follow", tagsHandlers.ToggleTagFollow)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/tags/{slug}/followers", tagsHandlers.GetTagFollowers)
			// Admin tag CRUD (previously missing entirely — content_tags/
			// AddContentTag was unreachable since there was no way to create a tag).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tags.manage")).
				Post("/admin/tags", tagsHandlers.CreateTag)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tags.manage")).
				Patch("/admin/tags/{id}", tagsHandlers.UpdateTag)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tags.manage")).
				Delete("/admin/tags/{id}", tagsHandlers.DeleteTag)
			// Admin tag listing (reuses the tenant-scoped ListTags handler).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tags.manage")).
				Get("/admin/tags", tagsHandlers.ListTags)
			// Tag moderators (Section 49).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tags.manage")).
				Get("/admin/tags/{id}/moderators", tagsHandlers.ListTagModerators)
			// AUDIT-RULE6: AddTagModerator/RemoveTagModerator mutate
			// tag_moderators → handler must call InvalidatePermCache (Rule 6).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermissionWithInvalidation(rbacResolver, "tags.manage")).
				Post("/admin/tags/{id}/moderators", tagsHandlers.AddTagModerator)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermissionWithInvalidation(rbacResolver, "tags.manage")).
				Delete("/admin/tags/{id}/moderators/{userId}", tagsHandlers.RemoveTagModerator)

			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.analytics.search")).
				Get("/admin/search/analytics", searchHandlers.SearchAdminAnalytics)

			// AI Core routes (Section 20 — AI Core)
			// AI content generation gated by feature.ai_content_gen flag (default: false).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.ai_content_gen"), rbac.RequirePermission(rbacResolver, "ai.generate")).
				Post("/ai/generate", aiHandlers.HandleGenerate)

			// JapaBot chat — gated by feature.japabot flag + ai.context.use RBAC
			// (Section 20: "Auth required + ai.context.use permission"). Per §19,
			// ai.context.* nodes default OFF including for super_admin.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.japabot"), rbac.RequirePermission(rbacResolver, "ai.context.use"), mw.RateLimit(vkClient.Redis(), "moderate")).
				Post("/ai/chat", aiHandlers.HandleChat)

			// Content AI Assist endpoints (catalog: /ai/assist/*), gated by
			// ai.editor.assist (Section 20 V1 Tool Catalog: content.editor_assist
			// behind ai.editor.assist), moderate rate limit.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.editor.assist"), mw.RateLimit(vkClient.Redis(), "moderate")).
				Post("/ai/assist/improve", aiHandlers.HandleImprove)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.editor.assist"), mw.RateLimit(vkClient.Redis(), "moderate")).
				Post("/ai/assist/summarize", aiHandlers.HandleSummarize)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.editor.assist"), mw.RateLimit(vkClient.Redis(), "moderate")).
				Post("/ai/assist/translate", aiHandlers.HandleTranslate)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.editor.assist"), mw.RateLimit(vkClient.Redis(), "moderate")).
				Post("/ai/assist/generate-tags", aiHandlers.HandleGenerateTags)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.editor.assist"), mw.RateLimit(vkClient.Redis(), "moderate")).
				Post("/ai/assist/generate-og", aiHandlers.HandleGenerateOG)

			// HITL review routes
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.hitl.review")).
				Post("/ai/hitl", aiHandlers.HandleCreateHITL)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.hitl.review")).
				Get("/ai/hitl/pending", aiHandlers.HandleListPendingHITL)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.hitl.review")).
				Post("/ai/hitl/{id}/approve", aiHandlers.HandleApproveHITL)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.hitl.review")).
				Post("/ai/hitl/{id}/reject", aiHandlers.HandleRejectHITL)

			// AI Tool Registry admin routes (Section 22 — AI Tools)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.tools.manage")).
				Get("/admin/ai/tools", aiToolHandlers.ListTools)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.tools.manage")).
				Get("/admin/ai/tools/{id}", aiToolHandlers.GetTool)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.tools.manage")).
				Put("/admin/ai/tools/{id}", aiToolHandlers.UpdateTool)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.tools.manage")).
				Delete("/admin/ai/tools/{id}", aiToolHandlers.DeleteTool)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.tools.manage")).
				Get("/admin/ai/tools/usage", aiToolHandlers.ListToolUsage)

			// AI Context admin routes — view/delete user AI context via Honcho.
			// Catalog lists two path forms (§20): /admin/ai/context/{user_id}
			// (canonical) and /admin/users/{id}/ai-context (legacy alias).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.context.admin_view")).
				Get("/admin/ai/context/{user_id}", aiHandlers.HandleGetUserContext)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.context.admin_delete_entry")).
				Delete("/admin/ai/context/{user_id}/{entry_id}", aiHandlers.HandleDeleteUserContextEntry)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.context.admin_view")).
				Get("/admin/users/{id}/ai-context", aiHandlers.HandleGetUserContext)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.context.admin_delete_entry")).
				Delete("/admin/users/{id}/ai-context/{entryId}", aiHandlers.HandleDeleteUserContextEntry)

			// AI Usage admin route (stub until ai_usage_log table exists)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.context.admin_view")).
				Get("/admin/ai/usage", aiHandlers.HandleGetAIUsage)
			// AI models admin route — lists available AI models
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.tools.manage")).
				Get("/admin/ai/models", aiHandlers.HandleListModels)

			// AI categories (public — for frontend AI category selection)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/ai/categories", aiHandlers.HandleListCategories)

			// RAG Knowledge Layer admin routes (Section 69, ADR-069).
			// All gated by ai.rag.* permission nodes seeded in migration 000245.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.rag.reindex")).
				Post("/admin/rag/reindex", ragHandlers.HandleReindex)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.rag.manage")).
				Get("/admin/rag/status", ragHandlers.HandleStatus)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.rag.manage")).
				Delete("/admin/rag/entity/{type}/{id}", ragHandlers.HandlePurgeEntity)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.rag.manage")).
				Get("/admin/settings/rag", ragHandlers.HandleGetSettings)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.rag.manage")).
				Put("/admin/settings/rag", ragHandlers.HandleUpdateSettings)

			// AI Agent Executor routes (Section 103, ADR-111) — Phase 6.
			// The agent SSE endpoint, capabilities, session history, and
			// session management. All gated by the agent.use RBAC permission
			// (seeded by migration 000372) + RequireAuth.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "agent.use")).
				Post("/ai/agent", agentHTTPHandlers.HandleAgent)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "agent.use")).
				Get("/ai/agent/capabilities", agentHTTPHandlers.HandleGetCapabilities)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "agent.use")).
				Get("/ai/agent/sessions", agentHTTPHandlers.HandleListSessions)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "agent.use")).
				Get("/ai/agent/sessions/{id}", agentHTTPHandlers.HandleGetSession)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "agent.use")).
				Post("/ai/agent/sessions/{id}/cancel", agentHTTPHandlers.HandleCancelSession)

			// Direct comparison endpoint (Section 103 — Comparison Engine,
			// Phase 7). Agent-accessible + directly callable. Orchestrates
			// the ComparisonEngine: classifies the tier, gathers knowledge
			// via pre-search, and optionally creates a comparison content
			// row for Tier 3.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "agent.use")).
				Post("/ai/compare-entities", agentHTTPHandlers.HandleCompareEntities)

			// Authorship reassignment (Section 103 — Authorship Assignment).
			// Gated by the content.assign_author permission node.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.assign_author")).
				Post("/content/{id}/reassign-author", agentHTTPHandlers.HandleReassignAuthor)

			// AI Privacy exemption management (Section 103 — Privacy Exemption
			// Columns). User-facing; NOT exposed as an agent tool. Users can
			// only set the flag on their own data (enforced in the handler).
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/me/ai-privacy", agentHTTPHandlers.HandleGetAIPrivacy)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Put("/me/ai-privacy", agentHTTPHandlers.HandleSetAIPrivacy)

			// AI Agent admin routes (Section 103 — Admin Observability).
			// Session audit trail, tool usage statistics, and cost reporting.
			// Gated by the agent.admin permission node (seeded by migration
			// 000406).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "agent.admin")).
				Get("/admin/agent/sessions", agentHTTPHandlers.HandleAdminListSessions)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "agent.admin")).
				Get("/admin/agent/sessions/{id}", agentHTTPHandlers.HandleAdminGetSession)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "agent.admin")).
				Get("/admin/agent/tool-stats", agentHTTPHandlers.HandleAdminToolStats)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "agent.admin")).
				Get("/admin/agent/cost-report", agentHTTPHandlers.HandleAdminCostReport)

			// Webhook admin CRUD routes (Section 23 — Integrations)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "integrations.webhooks.manage")).
				Get("/admin/webhooks", webhookHandlers.ListWebhooks)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "integrations.webhooks.manage")).
				Get("/admin/webhooks/{id}", webhookHandlers.GetWebhook)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "integrations.webhooks.manage")).
				Post("/admin/webhooks", webhookHandlers.CreateWebhook)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "integrations.webhooks.manage")).
				Put("/admin/webhooks/{id}", webhookHandlers.UpdateWebhook)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "integrations.webhooks.manage")).
				Delete("/admin/webhooks/{id}", webhookHandlers.DeleteWebhook)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "integrations.webhooks.manage")).
				Post("/admin/webhooks/{id}/test", webhookHandlers.TestWebhook)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "integrations.webhooks.manage")).
				Get("/admin/webhooks/{id}/deliveries", webhookHandlers.ListDeliveries)
			// Catalog: PATCH /admin/webhooks/:id (P5.6 — Task 8)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "integrations.webhooks.manage")).
				Patch("/admin/webhooks/{id}", webhookHandlers.UpdateWebhook)
			// Catalog: POST /admin/webhooks/:id/verify (§12 challenge/response — BUG-001)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "integrations.webhooks.manage")).
				Post("/admin/webhooks/{id}/verify", webhookHandlers.VerifyWebhook)
			// Catalog: POST /admin/webhooks/:id/deliveries/:delivery_id/retry (P5.6 — Task 8)
			// BUG-024: removed duplicate non-catalog route /admin/webhooks/:id/retry/:delivery_id
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "integrations.webhooks.manage")).
				Post("/admin/webhooks/{id}/deliveries/{delivery_id}/retry", webhookHandlers.RetryDelivery)

			// Webhook event types (public — for frontend webhook configuration)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/webhook-events", webhookHandlers.ListWebhookEvents)

			// Newsletter admin routes (Section 33 — Newsletter)
			// Public subscribe endpoint — gated by feature.newsletter, rate-limited
			// to prevent form-spam. Ref: conventions/api-routes-catalog.md
			r.With(features.Gate(featureChecker, "feature.newsletter"), mw.RateLimit(vkClient.Redis(), "form_submission")).
				Post("/newsletter/subscribe", newsletterHandlers.Subscribe)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "newsletter.manage")).
				Get("/admin/newsletter/subscribers", newsletterHandlers.ListSubscribers)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "newsletter.manage")).
				Get("/admin/newsletter/subscribers/{id}", newsletterHandlers.GetSubscriber)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "newsletter.manage")).
				Post("/admin/newsletter/subscribers", newsletterHandlers.CreateSubscriber)
			// Subscriber import — gated by newsletter.import (§62 RBAC, ADR-080 §6).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "newsletter.import")).
				Post("/admin/newsletter/subscribers/import", newsletterHandlers.ImportSubscribers)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "newsletter.manage")).
				Delete("/admin/newsletter/subscribers/{id}", newsletterHandlers.DeleteSubscriber)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "newsletter.manage")).
				Patch("/admin/newsletter/subscribers/{id}", newsletterHandlers.UpdateSubscriber)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "newsletter.manage")).
				Get("/admin/newsletter/campaigns", newsletterHandlers.ListCampaigns)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "newsletter.manage")).
				Get("/admin/newsletter/campaigns/{id}", newsletterHandlers.GetCampaign)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "newsletter.manage")).
				Post("/admin/newsletter/campaigns", newsletterHandlers.CreateCampaign)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "newsletter.manage")).
				Patch("/admin/newsletter/campaigns/{id}", newsletterHandlers.UpdateCampaign)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "newsletter.manage")).
				Delete("/admin/newsletter/campaigns/{id}", newsletterHandlers.DeleteCampaign)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "newsletter.manage")).
				Post("/admin/newsletter/campaigns/{id}/send", newsletterHandlers.HandleSendCampaign)

			// ── Menus — Navigation (ADR-054) ───────────────────────────

			// Admin: menu CRUD
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "menus.manage")).
				Get("/admin/menus", menusHandlers.ListMenus)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "menus.manage")).
				Post("/admin/menus", menusHandlers.CreateMenu)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "menus.manage")).
				Get("/admin/menus/{id}", menusHandlers.GetMenu)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "menus.manage")).
				Patch("/admin/menus/{id}", menusHandlers.UpdateMenu)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "menus.manage")).
				Delete("/admin/menus/{id}", menusHandlers.DeleteMenu)
			// Admin: menu item CRUD
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "menus.manage")).
				Post("/admin/menus/{id}/items", menusHandlers.CreateMenuItem)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "menus.manage")).
				Patch("/admin/menus/{id}/items/{itemId}", menusHandlers.UpdateMenuItem)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "menus.manage")).
				Delete("/admin/menus/{id}/items/{itemId}", menusHandlers.DeleteMenuItem)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "menus.manage")).
				Post("/admin/menus/{id}/items/reorder", menusHandlers.ReorderMenuItems)

			// Public: resolve menu by slug. OptionalAuth enriches the request with
			// the caller's identity when a Bearer token is present (it never rejects),
			// so the resolver returns authenticated-visibility items for logged-in
			// users while guests still receive the public menu.
			r.With(auth.OptionalAuth(kr, vkClient.Redis())).
				Get("/menus/{slug}", menusHandlers.GetMenuBySlug)

			// ── Email Templates (ADR-055) ───────────────────────────────

			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "email.templates.manage")).
				Get("/admin/email-templates", emailTemplatesHandlers.ListTemplates)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "email.templates.manage")).
				Get("/admin/email-templates/{id}", emailTemplatesHandlers.GetTemplate)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "email.templates.manage")).
				Patch("/admin/email-templates/{id}", emailTemplatesHandlers.UpdateTemplate)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "email.templates.manage")).
				Post("/admin/email-templates/{id}/reset", emailTemplatesHandlers.ResetTemplate)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "email.templates.manage")).
				Post("/admin/email-templates/{id}/test", emailTemplatesHandlers.TestTemplate)

			// ── Redirects (ADR-056) ─────────────────────────────────────

			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "redirects.manage")).
				Get("/admin/redirects", redirectsHandlers.ListRedirects)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "redirects.manage")).
				Post("/admin/redirects", redirectsHandlers.CreateRedirect)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "redirects.manage")).
				Get("/admin/redirects/{id}", redirectsHandlers.GetRedirect)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "redirects.manage")).
				Patch("/admin/redirects/{id}", redirectsHandlers.UpdateRedirect)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "redirects.manage")).
				Delete("/admin/redirects/{id}", redirectsHandlers.DeleteRedirect)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "redirects.manage")).
				Post("/admin/redirects/bulk", redirectsHandlers.BulkAction)

			// ── Coupons & External Affiliate Providers (Section 55) ────

			// Public coupon routes
			r.Get("/coupons", couponHandlers.ListPublicCoupons)
			r.Get("/coupons/{id}", couponHandlers.GetCoupon)
			r.With(mw.RateLimit(vkClient.Redis(), "moderate")).Post("/coupons/{id}/click", couponHandlers.RecordCouponClick)

			// Render helpers (internal — called by Go renderer, AUDIT-BE-022).
			// Gated by InternalGuard: only service-to-service calls or
			// loopback requests are allowed.
			r.With(mw.InternalGuard(cfg.InternalToken)).Get("/render/coupon/{id}", couponHandlers.RenderCoupon)
			r.With(mw.InternalGuard(cfg.InternalToken)).Get("/render/affiliate-link", couponHandlers.RenderAffiliateLink)

			// Admin: affiliate provider CRUD
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.providers.manage")).
				Get("/admin/affiliate-providers", couponHandlers.ListProviders)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.providers.manage")).
				Post("/admin/affiliate-providers", couponHandlers.CreateProvider)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.providers.manage")).
				Get("/admin/affiliate-providers/{id}", couponHandlers.GetProvider)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.providers.manage")).
				Patch("/admin/affiliate-providers/{id}", couponHandlers.UpdateProvider)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.providers.manage")).
				Delete("/admin/affiliate-providers/{id}", couponHandlers.DeleteProvider)

			// Admin: coupon CRUD
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "coupons.manage")).
				Get("/admin/coupons", couponHandlers.ListCouponsAdmin)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "coupons.manage")).
				Post("/admin/coupons", couponHandlers.CreateCouponAdmin)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "coupons.manage")).
				Get("/admin/coupons/{id}", couponHandlers.GetCouponAdmin)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "coupons.manage")).
				Patch("/admin/coupons/{id}", couponHandlers.UpdateCouponAdmin)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "coupons.manage")).
				Delete("/admin/coupons/{id}", couponHandlers.DeleteCouponAdmin)

			// ── User Affiliate Program (Section 56) ────────────────────

			// Public program info
			r.With(features.Gate(featureChecker, "feature.affiliate_program")).Group(func(r chi.Router) {
				r.Get("/affiliate-program", affiliateHandlers.GetProgramInfo)

				// Enrollment (auth + affiliate.program.enroll — Section 56 RBAC)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.program.enroll")).
					Post("/affiliate-program/enroll", affiliateHandlers.Enroll)

				// User's own affiliate links (auth + affiliate.links.manage)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.links.manage")).
					Get("/me/affiliate-links", affiliateHandlers.ListMyLinks)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.links.manage")).
					Post("/me/affiliate-links", affiliateHandlers.CreateMyLink)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.links.manage")).
					Get("/me/affiliate-links/{id}", affiliateHandlers.GetMyLink)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.links.manage")).
					Patch("/me/affiliate-links/{id}", affiliateHandlers.UpdateMyLink)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.links.manage")).
					Delete("/me/affiliate-links/{id}", affiliateHandlers.DeleteMyLink)

				// Earnings summary (auth + affiliate.links.manage)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.links.manage")).
					Get("/me/affiliate-earnings", affiliateHandlers.GetMyEarnings)

				// Affiliate redirect (public — no auth)
				r.Get("/r/{code}", affiliateHandlers.Redirect)

				// Admin: affiliate program management
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.admin.view")).
					Get("/admin/affiliate-program/members", affiliateHandlers.ListMembersAdmin)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.admin.manage")).
					Patch("/admin/affiliate-program/members/{user_id}", affiliateHandlers.UpdateMemberStatusAdmin)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.admin.view")).
					Get("/admin/affiliate-links", affiliateHandlers.ListAllLinksAdmin)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.admin.manage")).
					Delete("/admin/affiliate-links/{id}", affiliateHandlers.DeleteLinkAdmin)
			})

			// Referral Program admin routes (Section 34 — Referral)
			r.With(features.Gate(featureChecker, "feature.referral")).Group(func(r chi.Router) {
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "referral.manage")).
					Get("/admin/referral/program", referralHandlers.GetProgram)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "referral.manage")).
					Put("/admin/referral/program", referralHandlers.UpdateProgram)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "referral.manage")).
					Get("/admin/referral/referrals", referralHandlers.ListReferrals)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "referral.manage")).
					Get("/admin/referral/referrals/{id}", referralHandlers.GetReferral)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "referral.manage")).
					Delete("/admin/referral/referrals/{id}", referralHandlers.DeleteReferral)
			})

			// Mentorship admin routes (Section 35 — Mentorship)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "mentorship.admin")).
				Get("/admin/mentorship/programs", mentorshipHandlers.ListPrograms)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "mentorship.admin")).
				Get("/admin/mentorship/programs/{id}", mentorshipHandlers.GetProgram)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "mentorship.admin")).
				Post("/admin/mentorship/programs", mentorshipHandlers.CreateProgram)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "mentorship.admin")).
				Patch("/admin/mentorship/programs/{id}", mentorshipHandlers.UpdateProgram)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "mentorship.admin")).
				Delete("/admin/mentorship/programs/{id}", mentorshipHandlers.DeleteProgram)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "mentorship.admin")).
				Get("/admin/mentorship/matches", mentorshipHandlers.ListMatches)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "mentorship.admin")).
				Patch("/admin/mentorship/matches/{id}", mentorshipHandlers.UpdateMatch)

			// §102 Mentorship Matching routes (ADR-110).
			// Feature-flagged via features.Gate (returns 404 when disabled, ADR-042).
			r.With(features.Gate(featureChecker, "mentorship.enabled")).Group(func(r chi.Router) {
				// Public discovery: list mentors, view mentor profile, compare.
				r.Get("/mentors", mentorshipHandlers.ListMentors)
				r.Get("/mentors/{username}", mentorshipHandlers.GetMentorByUsername)
				r.With(features.Gate(featureChecker, "mentorship.comparison")).
					Get("/compare/mentors", mentorshipHandlers.CompareMentors)

				// Authenticated: manage own mentor profile.
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "mentorship.become_mentor")).
					Get("/me/mentor-profile", mentorshipHandlers.GetMyMentorProfile)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "mentorship.become_mentor")).
					Patch("/me/mentor-profile", mentorshipHandlers.UpdateMyMentorProfile)

				// Authenticated: request mentorship (member+).
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "mentorship.request")).
					Post("/mentorship/request", mentorshipHandlers.RequestMentorship)

				// Authenticated: manage outgoing requests (as mentee).
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "mentorship.request")).
					Get("/me/mentorship/outgoing", mentorshipHandlers.ListOutgoingConnections)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "mentorship.request")).
					Delete("/me/mentorship/outgoing/{id}", mentorshipHandlers.CancelOutgoingConnection)

				// Authenticated: manage incoming requests (as mentor, elevated role).
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "mentorship.respond")).
					Get("/me/mentorship/incoming", mentorshipHandlers.ListIncomingConnections)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "mentorship.respond")).
					Patch("/me/mentorship/incoming/{id}/accept", mentorshipHandlers.AcceptConnection)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "mentorship.respond")).
					Patch("/me/mentorship/incoming/{id}/decline", mentorshipHandlers.DeclineConnection)

				// Authenticated: complete or cancel an active connection (either party).
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "mentorship.request")).
					Patch("/mentorship/{id}/complete", mentorshipHandlers.CompleteConnection)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "mentorship.request")).
					Patch("/mentorship/{id}/cancel", mentorshipHandlers.CancelConnection)

				// Admin: list all connections and override status.
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "mentorship.manage")).
					Get("/admin/mentorship/connections", mentorshipHandlers.ListAllConnections)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "mentorship.manage")).
					Patch("/admin/mentorship/connections/{id}", mentorshipHandlers.AdminUpdateConnection)
			})

			// Documentation System routes (Section 44)
			// Public: read published pages for an audience
			r.Get("/docs/{audience}", docsHandlers.ListDocPages)
			r.Get("/docs/{audience}/{slug}", docsHandlers.GetDocPage)
			// Public docs nav + search (P5.6 — Task 3)
			r.Get("/docs/nav", docsHandlers.ListDocNav)
			r.Get("/docs/search", docsHandlers.SearchDocPages)

			// Admin docs page list/detail (P5.6 — Task 3, catalog paths)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "docs.admin")).
				Get("/admin/docs/pages", docsHandlers.ListAllDocPages)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "docs.admin")).
				Get("/admin/docs/pages/{id}", docsHandlers.GetAdminDocPage)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "docs.admin")).
				Post("/admin/docs/pages", docsHandlers.CreateDocPage)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "docs.admin")).
				Patch("/admin/docs/pages/{id}", docsHandlers.UpdateDocPage)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "docs.admin")).
				Delete("/admin/docs/pages/{id}", docsHandlers.DeleteDocPage)
			// Doc page approve/publish (§44 — dedicated handlers with reviewer tracking)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "docs.admin")).
				Post("/admin/docs/pages/{id}/approve", docsHandlers.ApproveDocPage)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "docs.admin")).
				Post("/admin/docs/pages/{id}/publish", docsHandlers.PublishDocPage)
			// AI doc generation (P5.6 — Task 3 — stub, delegates to ai/generate)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "docs.admin")).
				Post("/admin/docs/ai-generate", aiHandlers.HandleGenerate)

			// Learning Platform routes (Section 36)
			// Gated by feature.learning flag (default: false).
			r.With(features.Gate(featureChecker, "feature.learning")).Group(func(r chi.Router) {
				// Public: view published tracks (catalog: /learn/*)
				r.Get("/learn/tracks", learningHandlers.ListPublishedTracks)
				r.Get("/learn/tracks/{slug}", learningHandlers.GetTrackDetail)
				r.Get("/learn/tracks/{trackSlug}/modules/{moduleSlug}/lessons/{lessonSlug}", learningHandlers.GetLesson)

				// Authenticated: enrollment, progress and certificates
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/learn/tracks/{id}/enroll", learningHandlers.EnrollInTrack)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/learn/lessons/{id}", learningHandlers.GetLessonByID)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/learn/lessons/{id}/progress", learningHandlers.UpdateProgress)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/learn/lessons/{id}/complete", learningHandlers.CompleteLesson)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/learn/progress/{trackId}", learningHandlers.GetTrackProgress)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/certificates", learningHandlers.GetMyCertificates)
				// /me/enrollments: any authenticated user can list their own
				// enrollments — the handler filters by user.UserID, so no
				// university-specific RBAC node is needed. The previous
				// university.enrollment.view_own gate caused 403 for regular
				// Learn users, breaking the track-detail enrollment state fetch.
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/enrollments", learningHandlers.ListMyEnrollments)

				// Quizzes
				r.Get("/learn/quizzes/{id}", learningHandlers.GetQuiz)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/learn/quizzes/{id}/submit", learningHandlers.SubmitQuiz)

				// Certificates (public detail + download — catalog: /certificates/:id)
				r.Get("/certificates/{id}", learningHandlers.GetCertificateDetail)
				r.Get("/certificates/{id}/download", learningHandlers.DownloadCertificate)

				// Track reviews (catalog: POST|GET /learn/tracks/{id}/reviews)
				// AUDIT-FRESH-DEAD-NODES: wired university.review.create RBAC gate.
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.review.create")).Post("/learn/tracks/{id}/reviews", learningHandlers.CreateTrackReview)
				r.Get("/learn/tracks/{id}/reviews", learningHandlers.ListTrackReviews)
			})

			// Japa University creator-economy routes (ADR-063 — upgrade to Learn).
			// Instructor lifecycle
			// AUDIT-FRESH-DEAD-NODES: wired university.instructor.apply RBAC gate.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.instructor.apply")).Post("/learn/university/instructor/apply", universityHandlers.Apply)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/learn/university/instructor/me", universityHandlers.GetMyInstructorProfile)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Patch("/learn/university/instructor/me", universityHandlers.UpdateMyInstructorProfile)
			r.Get("/learn/university/instructors/{id}", universityHandlers.GetPublicInstructor)

			// Creator track management
			// AUDIT-FRESH-DEAD-NODES: wired university.course.create/submit RBAC gates (BUG-011).
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/learn/university/my/tracks", universityHandlers.ListMyTracks)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.course.create")).Post("/learn/university/my/tracks", universityHandlers.CreateMyTrack)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Patch("/learn/university/my/tracks/{id}", universityHandlers.UpdateMyTrack)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.course.submit")).Post("/learn/university/my/tracks/{id}/submit", universityHandlers.SubmitMyTrack)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/learn/university/my/tracks/{id}/stats", universityHandlers.GetMyTrackStats)

			// Marketplace catalog
			r.Get("/learn/university/catalog", universityHandlers.ListCatalog)
			r.Get("/learn/university/catalog/{slug}", universityHandlers.GetCatalogTrack)

			// Consolidated learner routes (ADR-063 consolidation: /learn/tracks/*
			// moved under /learn/university/*). These point to the same handlers as
			// the legacy /learn/tracks/* routes, which remain for backward compat.
			r.Get("/learn/university/tracks/{slug}", learningHandlers.GetTrackDetail)
			r.Get("/learn/university/tracks/{trackSlug}/modules/{moduleSlug}/lessons/{lessonSlug}", learningHandlers.GetLesson)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/learn/university/tracks/{id}/enroll", learningHandlers.EnrollInTrack)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/learn/university/lessons/{id}", learningHandlers.GetLessonByID)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/learn/university/lessons/{id}/progress", learningHandlers.UpdateProgress)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/learn/university/lessons/{id}/complete", learningHandlers.CompleteLesson)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/learn/university/progress/{trackId}", learningHandlers.GetTrackProgress)
			r.Get("/learn/university/quizzes/{id}", learningHandlers.GetQuiz)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/learn/university/quizzes/{id}/submit", learningHandlers.SubmitQuiz)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.review.create")).Post("/learn/university/tracks/{id}/reviews", learningHandlers.CreateTrackReview)
			r.Get("/learn/university/tracks/{id}/reviews", learningHandlers.ListTrackReviews)

			// Earnings & payouts
			// AUDIT-FRESH-DEAD-NODES: wired university.payout.view_own RBAC gate on both endpoints (BUG-013).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.payout.view_own")).Get("/learn/university/my/earnings", universityHandlers.GetMyEarnings)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.payout.view_own")).Get("/learn/university/my/payouts", universityHandlers.GetMyPayouts)

			// Content-country preference (ADR-064 geo personalization)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Patch("/me/preferences/content-country", usersHandlers.UpdateMyContentCountry)

			// Admin: University management (catalog: /admin/university/*)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.instructor.approve")).
				Get("/admin/university/instructors", universityHandlers.AdminListInstructors)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.instructor.approve")).
				Post("/admin/university/instructors/{id}/approve", universityHandlers.AdminApproveInstructor)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.instructor.approve")).
				Post("/admin/university/instructors/{id}/reject", universityHandlers.AdminRejectInstructor)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.instructor.manage")).
				Post("/admin/university/instructors/{id}/suspend", universityHandlers.AdminSuspendInstructor)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.instructor.manage")).
				Post("/admin/university/instructors/{id}/reinstate", universityHandlers.AdminReinstateInstructor)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.instructor.manage")).
				Delete("/admin/university/instructors/{id}", universityHandlers.AdminDeleteInstructor)
			// BUG-002: PATCH instructor tier/commission override.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.instructor.manage")).
				Patch("/admin/university/instructors/{id}", universityHandlers.AdminUpdateInstructor)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.course.approve")).
				Get("/admin/university/tracks/pending", universityHandlers.AdminListPendingTracks)
			// BUG-002: GET all tracks (not just pending).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.course.approve")).
				Get("/admin/university/tracks", universityHandlers.AdminListAllTracks)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.course.approve")).
				Post("/admin/university/tracks/{id}/approve", universityHandlers.AdminApproveTrack)
			// BUG-002: POST reject track.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.course.approve")).
				Post("/admin/university/tracks/{id}/reject", universityHandlers.AdminRejectTrack)
			// BUG-012: POST feature/unfeature track (wires university.course.feature node).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.course.feature")).
				Post("/admin/university/tracks/{id}/feature", universityHandlers.AdminFeatureTrack)
			// BUG-014: read-only payout endpoints gated by view_all, not the write process node.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.payout.view_all")).
				Get("/admin/university/payouts", universityHandlers.AdminListPayouts)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.payout.view_all")).
				Get("/admin/university/payouts/{id}", universityHandlers.AdminGetPayout)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.payout.process"),
				mw.IdempotencyMiddleware(vkClient.Redis(), settingsService)).
				Post("/admin/university/payouts/{id}/process", universityHandlers.AdminProcessPayout)
			// BUG-002: POST fail payout.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.payout.process")).
				Post("/admin/university/payouts/{id}/fail", universityHandlers.AdminFailPayout)

			// Admin: revenue-split audit viewer (ADR-063 §9, read-only)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.revenue.list")).
				Get("/admin/university/revenue-splits", universityHandlers.AdminListRevenueSplits)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.revenue.view")).
				Get("/admin/university/revenue-splits/{id}", universityHandlers.AdminGetRevenueSplit)

			// Admin: track categories CRUD (ADR-063 §9.1)
			// Ref: japaconnect-engineering/sections/36-learning-platform.md (Track categories)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.category.manage")).
				Get("/admin/university/categories", universityHandlers.AdminListCategories)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.category.manage")).
				Post("/admin/university/categories", universityHandlers.AdminCreateCategory)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.category.manage")).
				Get("/admin/university/categories/{id}", universityHandlers.AdminGetCategory)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.category.manage")).
				Patch("/admin/university/categories/{id}", universityHandlers.AdminUpdateCategory)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.category.manage")).
				Delete("/admin/university/categories/{id}", universityHandlers.AdminDeleteCategory)

			// Public: active track categories for catalog dropdown (no auth required)
			r.Get("/learn/university/categories", universityHandlers.PublicListActiveCategories)

			// Admin: CRUD for tracks, modules, lessons
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "learning.admin.manage")).
				Get("/admin/learn/tracks", learningHandlers.ListAllTracks)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "learning.admin.manage")).
				Post("/admin/learn/tracks", learningHandlers.CreateTrack)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "learning.admin.manage")).
				Patch("/admin/learn/tracks/{id}", learningHandlers.UpdateTrack)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "learning.admin.manage")).
				Delete("/admin/learn/tracks/{id}", learningHandlers.DeleteTrack)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "learning.admin.manage")).
				Post("/admin/learn/tracks/{trackId}/modules", learningHandlers.CreateModule)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "learning.admin.manage")).
				Patch("/admin/learn/modules/{id}", learningHandlers.UpdateModule)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "learning.admin.manage")).
				Delete("/admin/learn/modules/{id}", learningHandlers.DeleteModule)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "learning.admin.manage")).
				Post("/admin/learn/modules/{moduleId}/lessons", learningHandlers.CreateLesson)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "learning.admin.manage")).
				Patch("/admin/learn/lessons/{id}", learningHandlers.UpdateLesson)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "learning.admin.manage")).
				Delete("/admin/learn/lessons/{id}", learningHandlers.DeleteLesson)

			// Admin: flat list endpoints for DataTables
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "learning.admin.manage")).
				Get("/admin/learn/modules", learningHandlers.ListAllModules)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "learning.admin.manage")).
				Get("/admin/learn/lessons", learningHandlers.ListAllLessons)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "learning.admin.manage")).
				Get("/admin/learn/tracks/{trackId}/modules", learningHandlers.ListModulesByTrack)

			// Page Builder admin CRUD routes (Section 16 — Page Builder)
			// Blocks
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pagebuilder.blocks.manage")).
				Get("/admin/pagebuilder/blocks", pagebuilderHandlers.ListBlocks)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pagebuilder.blocks.manage")).
				Get("/admin/pagebuilder/blocks/{id}", pagebuilderHandlers.GetBlock)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pagebuilder.blocks.manage")).
				Post("/admin/pagebuilder/blocks", pagebuilderHandlers.CreateBlock)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pagebuilder.blocks.manage")).
				Patch("/admin/pagebuilder/blocks/{id}", pagebuilderHandlers.UpdateBlock)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pagebuilder.blocks.manage")).
				Delete("/admin/pagebuilder/blocks/{id}", pagebuilderHandlers.DeleteBlock)

			// Templates
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pagebuilder.templates.manage")).
				Get("/admin/page-templates", pagebuilderHandlers.ListTemplates)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pagebuilder.templates.manage")).
				Get("/admin/page-templates/{id}", pagebuilderHandlers.GetTemplate)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pagebuilder.templates.manage")).
				Post("/admin/page-templates", pagebuilderHandlers.CreateTemplate)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pagebuilder.templates.manage")).
				Patch("/admin/page-templates/{id}", pagebuilderHandlers.UpdateTemplate)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pagebuilder.templates.manage")).
				Delete("/admin/page-templates/{id}", pagebuilderHandlers.DeleteTemplate)

			// Themes
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pagebuilder.themes.manage")).
				Get("/admin/pagebuilder/themes", pagebuilderHandlers.ListThemes)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pagebuilder.themes.manage")).
				Get("/admin/pagebuilder/themes/{id}", pagebuilderHandlers.GetTheme)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pagebuilder.themes.manage")).
				Post("/admin/pagebuilder/themes", pagebuilderHandlers.CreateTheme)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pagebuilder.themes.manage")).
				Patch("/admin/pagebuilder/themes/{id}", pagebuilderHandlers.UpdateTheme)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pagebuilder.themes.manage")).
				Delete("/admin/pagebuilder/themes/{id}", pagebuilderHandlers.DeleteTheme)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pagebuilder.themes.manage")).
				Post("/admin/pagebuilder/themes/{id}/default", pagebuilderHandlers.SetDefaultTheme)

			// Pages
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pages.manage")).
				Get("/admin/pages", pagebuilderHandlers.ListPages)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pages.manage")).
				Get("/admin/pages/{id}", pagebuilderHandlers.GetPage)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pages.manage")).
				Post("/admin/pages", pagebuilderHandlers.CreatePage)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pages.manage")).
				Patch("/admin/pages/{id}", pagebuilderHandlers.UpdatePage)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pages.manage")).
				Delete("/admin/pages/{id}", pagebuilderHandlers.DeletePage)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pages.publish")).
				Post("/admin/pages/{id}/publish", pagebuilderHandlers.PublishPage)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pages.publish")).
				Post("/admin/pages/{id}/archive", pagebuilderHandlers.ArchivePage)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pages.manage")).
				Patch("/admin/pages/{id}/blocks/reorder", pagebuilderHandlers.ReorderBlocks)

			// Public page endpoint (no auth required)
			r.Get("/pages/{slug}", pagebuilderHandlers.GetPublishedPage)

			// ── Sidebars — Sidebar Management System (Section 59, ADR-066) ──
			// Replaces the deprecated pagebuilder sidebar routes.

			// Admin: sidebar CRUD (requires sidebars.manage permission)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "sidebars.manage")).
				Get("/admin/sidebars", sidebarHandlers.ListSidebars)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "sidebars.manage")).
				Post("/admin/sidebars", sidebarHandlers.CreateSidebar)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "sidebars.manage")).
				Get("/admin/sidebars/{id}", sidebarHandlers.GetSidebar)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "sidebars.manage")).
				Patch("/admin/sidebars/{id}", sidebarHandlers.UpdateSidebar)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "sidebars.manage")).
				Delete("/admin/sidebars/{id}", sidebarHandlers.DeleteSidebar)
			// Admin: sidebar widget CRUD
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "sidebars.manage")).
				Post("/admin/sidebars/{id}/widgets", sidebarHandlers.CreateSidebarWidget)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "sidebars.manage")).
				Patch("/admin/sidebars/{id}/widgets/{widgetId}", sidebarHandlers.UpdateSidebarWidget)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "sidebars.manage")).
				Delete("/admin/sidebars/{id}/widgets/{widgetId}", sidebarHandlers.DeleteSidebarWidget)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "sidebars.manage")).
				Post("/admin/sidebars/{id}/widgets/reorder", sidebarHandlers.ReorderSidebarWidgets)

			// Public: sidebar widget type registry (no auth required).
			r.Get("/sidebars/widget-types", sidebarHandlers.ListWidgetTypes)

			// Public: resolve sidebar for current page. OptionalAuth enriches the
			// request with the caller's identity when a Bearer token is present
			// (it never rejects), so authenticated users see authenticated-visibility
			// sidebars/widgets while guests still receive public ones.
			r.With(auth.OptionalAuth(kr, vkClient.Redis())).
				Get("/sidebars/resolve", sidebarHandlers.Resolve)

			// Public page-templates endpoints (read-only, no auth required)
			r.Get("/page-templates", pagebuilderHandlers.ListTemplates)
			r.Get("/page-templates/{id}", pagebuilderHandlers.GetTemplate)

			// Admin widget configurations CRUD (Section 16 — widget_configurations).
			// Gates on widgets.manage RBAC node (migration 000412).
			r.Group(func(r chi.Router) {
				r.Use(auth.RequireAuth(kr, vkClient.Redis()))
				r.Use(rbac.RequirePermission(rbacResolver, "widgets.manage"))
				r.Get("/admin/widgets", widgetHandlers.ListWidgets)
				r.Post("/admin/widgets", widgetHandlers.CreateWidget)
				r.Patch("/admin/widgets/{id}", widgetHandlers.UpdateWidget)
				r.Delete("/admin/widgets/{id}", widgetHandlers.DeleteWidget)
			})

			// Platform Settings (admin)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings")).
				Get("/admin/settings", adminHandlers.ListSettings)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings")).
				Put("/admin/settings/{category}", adminHandlers.UpdateSettings)

			// RBAC Management routes (admin)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.rbac.manage")).
				Get("/admin/user-groups", rbacHandlers.ListGroups)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.rbac.manage")).
				Post("/admin/user-groups", rbacHandlers.CreateGroup)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.rbac.manage")).
				Delete("/admin/user-groups/{id}", rbacHandlers.DeleteGroup)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.rbac.manage")).
				Get("/admin/user-groups/{id}/members", rbacHandlers.ListGroupMembers)
			// AUDIT-RULE6: AddGroupMember/RemoveGroupMember mutate user_group
			// membership → RequirePermissionWithInvalidation signals that the
			// handler must call InvalidatePermCache for the target user (Rule 6).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermissionWithInvalidation(rbacResolver, "admin.rbac.manage")).
				Post("/admin/user-groups/{id}/members", rbacHandlers.AddGroupMember)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermissionWithInvalidation(rbacResolver, "admin.rbac.manage")).
				Delete("/admin/user-groups/{id}/members/{userId}", rbacHandlers.RemoveGroupMember)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.rbac.manage")).
				Get("/admin/permissions", rbacHandlers.ListPermissions)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.rbac.manage")).
				Get("/admin/roles", rbacHandlers.ListRoles)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.rbac.manage")).
				Post("/admin/roles", rbacHandlers.CreateRole)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.rbac.manage")).
				Get("/admin/roles/{id}/permissions", rbacHandlers.ListRolePermissions)
			// AUDIT-RULE6: AssignPermissionToRole mutates role_permissions →
			// RequirePermissionWithInvalidation signals handler must invalidate
			// affected users' caches (Rule 6).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermissionWithInvalidation(rbacResolver, "admin.rbac.manage")).
				Post("/admin/roles/{id}/permissions", rbacHandlers.AssignPermissionToRole)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.rbac.manage")).
				Get("/admin/user-groups/{id}/roles", rbacHandlers.ListGroupRoles)
			// AUDIT-RULE6: AssignRoleToGroup mutates user_role_assignments →
			// RequirePermissionWithInvalidation signals handler must invalidate
			// affected group members' caches (Rule 6).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermissionWithInvalidation(rbacResolver, "admin.rbac.manage")).
				Post("/admin/user-groups/{id}/roles", rbacHandlers.AssignRoleToGroup)

			// GetUserPermissions resolves the CALLER's own permission set (no admin
			// gate in the handler itself) — was never registered as a route, so the
			// admin panel's fetchPermissions() has been silently 404ing and falling
			// back to an empty permission set. Wired here at /me/permissions.
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/me/permissions", rbacHandlers.GetUserPermissions)

			// API scopes (public — for developer portal scope selection)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/developer/scopes", rbacHandlers.ListScopes)

			// Admin Moderation routes
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Get("/admin/moderation-queue", adminHandlers.ListModeration)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Post("/admin/moderation-queue/bulk", adminHandlers.BulkModeration)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Post("/admin/moderation-queue/{id}/appeal", adminHandlers.AppealModeration)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Patch("/admin/moderation-queue/{id}", adminHandlers.ModerateItem)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "moderation.alerts.manage")).
				Get("/admin/moderation/alerts", adminHandlers.ListModerationAlerts)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "moderation.alerts.manage")).
				Post("/admin/moderation/alerts/{id}/acknowledge", adminHandlers.AcknowledgeModerationAlert)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "moderation.alerts.manage")).
				Post("/admin/moderation/alerts/{id}/resolve", adminHandlers.ResolveModerationAlert)
			// BUG-010: moderator removal-by-status for comments (Section 49).
			// Sets status='removed' + removed_by + removed_reason; the row stays
			// visible for audit, unlike a soft-delete.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comments.delete_any")).
				Post("/admin/comments/{id}/remove", commentsHandlers.RemoveComment)

			// Admin Broadcast route
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.broadcast")).
				Post("/admin/broadcasts", adminHandlers.Broadcast)

			// ═══════════════════════════════════════════════════════════
			// P5.6 — Batch route wiring for CATALOG_ONLY routes
			// ═══════════════════════════════════════════════════════════

			// Task 4: Notifications admin — notification type CRUD
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "notifications.manage")).
				Get("/admin/notification-types", notificationHandlers.AdminListNotificationTypes)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "notifications.manage")).
				Post("/admin/notification-types", notificationHandlers.CreateNotificationType)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "notifications.manage")).
				Patch("/admin/notification-types/{key}", notificationHandlers.UpdateNotificationType)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "notifications.manage")).
				Delete("/admin/notification-types/{key}", notificationHandlers.DeleteNotificationType)
			// Broadcasts (P5.6 — Task 4)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.broadcast")).
				Get("/admin/broadcasts", adminHandlers.ListBroadcasts)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.broadcast")).
				Delete("/admin/broadcasts/{id}", adminHandlers.DeleteBroadcast)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.broadcast")).
				Post("/admin/broadcasts/{id}/send", adminHandlers.Broadcast)

			// Task 5: Tickets admin — PATCH ticket-types
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.type.manage")).
				Patch("/admin/ticket-types/{id}", ticketHandlers.UpdateTicketType)

			// Task 6: RBAC admin — DELETE/PATCH roles, PATCH user-groups
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.rbac.manage")).
				Delete("/admin/roles/{id}", rbacHandlers.DeleteRole)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.rbac.manage")).
				Patch("/admin/roles/{id}", rbacHandlers.UpdateRole)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.rbac.manage")).
				Patch("/admin/roles/{id}/permissions", rbacHandlers.SetRolePermissions)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.rbac.manage")).
				Patch("/admin/user-groups/{id}", rbacHandlers.UpdateGroup)

			// Task 8: Webhooks — already wired above

			// Task 9: Media admin — already wired above

			// Task 10: Organizations admin — verify
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Post("/admin/organizations/{id}/verify", adminHandlers.VerifyOrganization)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Delete("/admin/organizations/{id}/verify", adminHandlers.UnverifyOrganization)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Post("/admin/organizations/{id}/claim", adminHandlers.ClaimOrganization)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Get("/admin/organizations/pending-claims", adminHandlers.ListPendingOwnershipClaims)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Get("/admin/organizations/claims/{id}/submission", adminHandlers.GetClaimFormSubmission)

			// Organizations admin CRUD
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Get("/admin/organizations", orgHandlers.AdminListOrganizations)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Get("/admin/organizations/{id}", orgHandlers.AdminGetOrganization)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Post("/admin/organizations", orgHandlers.AdminCreateOrganization)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Patch("/admin/organizations/{id}", orgHandlers.AdminUpdateOrganization)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Delete("/admin/organizations/{id}", orgHandlers.AdminDeleteOrganization)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Get("/admin/organizations/{id}/members", orgHandlers.AdminListMembers)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Get("/admin/organizations/{id}/invitations", orgHandlers.AdminListInvitations)

			// Task 12: Spaces admin extras
			// Admin spaces CRUD (reuses the tenant-scoped space handlers; the
			// admin's resolved tenant scopes the listing).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spaces.admin")).
				Get("/admin/spaces", spaceHandlers.AdminListSpaces)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spaces.admin")).
				Get("/admin/spaces/{id}", spaceHandlers.AdminGetSpace)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spaces.admin")).
				Get("/admin/spaces/{id}/moderators", spaceHandlers.AdminListModerators)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spaces.admin")).
				Post("/admin/spaces", spaceHandlers.CreateSpace)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spaces.admin")).
				Patch("/admin/spaces/{id}", spaceHandlers.UpdateSpace)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spaces.admin")).
				Delete("/admin/spaces/{id}", spaceHandlers.DeleteSpace)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spaces.moderate")).
				Get("/admin/spaces/{id}/join-requests", spaceHandlers.ListJoinRequests)
			// Admin-wide join requests (across all spaces in the admin tenant).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spaces.moderate")).
				Get("/admin/join-requests", spaceHandlers.AdminListJoinRequests)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spaces.moderate")).
				Post("/admin/join-requests/{id}/approve", spaceHandlers.AdminApproveJoinRequest)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spaces.moderate")).
				Post("/admin/join-requests/{id}/reject", spaceHandlers.AdminRejectJoinRequest)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spaces.moderate")).
				Patch("/admin/spaces/{id}/join-requests/{requestId}", spaceHandlers.UpdateJoinRequest)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spaces.moderate")).
				Patch("/admin/spaces/{id}/status", spaceHandlers.UpdateSpaceStatus)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermissionWithInvalidation(rbacResolver, "spaces.moderator.assign")).
				Post("/admin/spaces/{id}/moderators", spaceHandlers.AdminAssignModerator)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermissionWithInvalidation(rbacResolver, "spaces.moderator.assign")).
				Delete("/admin/spaces/{id}/moderators/{moderator_id}", spaceHandlers.AdminRemoveModerator)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/spaces/{id}/analytics", spaceHandlers.GetSpaceAnalytics)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spaces.moderate")).
				Patch("/spaces/{id}/pinned/reorder", spaceHandlers.ReorderPinned)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/spaces/suggest-parents", spaceHandlers.SuggestParentSpaces)

			// Task 13: Professional profiles admin + public extras
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Post("/admin/profiles/{id}/verify", professionalHandlers.VerifyProfile)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Delete("/admin/profiles/{id}/verify", professionalHandlers.UnverifyProfile)

			// Task 14: Learning admin extras
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "learning.admin.manage")).
				Get("/admin/learn/tracks/{id}/enrollments", learningHandlers.ListTrackEnrollments)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "learning.admin.manage")).
				Post("/admin/learn/tracks/{id}/publish", learningHandlers.PublishTrack)

			// Task 15: Payments/coupons admin + subscription
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "payments.discount_codes.manage")).
				Patch("/admin/discount-codes/{id}", paymentHandlers.UpdateDiscountCode)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "payments.discount_codes.manage")).
				Get("/admin/discount-codes/{id}/usage", paymentHandlers.GetDiscountCodeUsage)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Delete("/subscriptions/{id}", paymentHandlers.DeleteSubscription)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.IdempotencyMiddleware(vkClient.Redis(), settingsService)).
				Post("/me/subscription/reactivate", paymentHandlers.ReactivateSubscription)

			// Task 16: Advertising admin
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.campaign.manage")).
				Patch("/admin/ads/campaigns/{id}", adHandlers.UpdateCampaign)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.slot.manage")).
				Get("/admin/ads/slot-types/{id}", adHandlers.GetSlotType)

			// Task 17: Wallet admin — PATCH /admin/wallet/rules/{id} updates by
			// ID (BUG-005/BUG-009 — previously called CreateOrUpdateRule which
			// ignored {id} and upserted by activity_key). The duplicate
			// /admin/wallet/award route is removed (BUG-009) — use
			// /admin/wallet/adjust instead, which is the catalog entry.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wallet.admin.rules.manage")).
				Patch("/admin/wallet/rules/{id}", walletHandlers.UpdateRule)

			// Task 18: Blog review queue
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.review.manage")).
				Get("/admin/blog/review-queue", blogHandlers.ListReviews)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.review.manage")).
				Post("/admin/blog/review-queue/{id}/approve", blogHandlers.ApproveReview)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.review.manage")).
				Post("/admin/blog/review-queue/{id}/reject", blogHandlers.RejectReview)
			// Admin blog post management (Section 24 blog extension).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.review.manage")).
				Get("/admin/blog", blogHandlers.AdminListPosts)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.review.manage")).
				Post("/admin/blog", blogHandlers.AdminCreatePost)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.review.manage")).
				Patch("/admin/blog/{id}", blogHandlers.AdminUpdatePost)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.review.manage")).
				Delete("/admin/blog/{id}", blogHandlers.AdminDeletePost)

			// Task 19: Analytics admin — domain-specific
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "analytics.admin")).
				Get("/admin/analytics/content", analyticsHandlers.GetAdminContentAnalytics)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "analytics.admin")).
				Get("/admin/analytics/revenue", analyticsHandlers.GetRevenueAnalytics)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "analytics.admin")).
				Get("/admin/analytics/search", analyticsHandlers.GetSearchAnalytics)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "analytics.admin")).
				Get("/admin/analytics/spaces", analyticsHandlers.GetSpaceAnalytics)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "analytics.admin")).
				Get("/admin/analytics/users", analyticsHandlers.GetUserAnalytics)

			// Task 20: GDPR admin
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gdpr.deletions.view")).
				Get("/admin/gdpr/deletion-requests", gdprHandlers.ListPendingDeletions)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gdpr.deletions.view")).
				Post("/admin/gdpr/deletion-requests/{id}/execute", gdprHandlers.ExecuteDeletion)
			// NOTE: GET /admin/gdpr/exports is registered earlier in the GDPR
			// routes block (gdpr.data_exports.view). Do not re-register here.

			// Task 21: Users admin moderation
			// AUDIT-RULE6: Ban/Unban insert/remove user_bans → handler must call
			// InvalidatePermCache for the target user (Rule 6).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermissionWithInvalidation(rbacResolver, "admin.moderation")).
				Post("/admin/users/{id}/ban", adminHandlers.BanUser)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermissionWithInvalidation(rbacResolver, "admin.moderation")).
				Delete("/admin/users/{id}/ban", adminHandlers.UnbanUser)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Get("/admin/users/{id}/bans", adminHandlers.ListUserBans)
			// AUDIT-RULE6: UpdateUserGroup changes users.user_group_id → handler
			// must call InvalidatePermCache for the target user (Rule 6).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermissionWithInvalidation(rbacResolver, "admin.rbac.manage")).
				Patch("/admin/users/{id}/group", adminHandlers.UpdateUserGroup)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Patch("/admin/users/{id}/status", adminHandlers.UpdateUserStatus)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Post("/admin/users/{id}/warn", adminHandlers.WarnUser)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Get("/admin/users/{id}/warnings", adminHandlers.ListUserWarnings)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Delete("/admin/users/{id}/warnings/{warningId}", adminHandlers.DeleteUserWarning)

			// Granular ban system
			// AUDIT-RULE6: Restrict/LiftRestriction/LiftAllRestrictions mutate
			// user_operation_bans → handler must call InvalidatePermCache (Rule 6).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermissionWithInvalidation(rbacResolver, "admin.moderation")).
				Post("/admin/users/{id}/restrict", adminHandlers.RestrictUser)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermissionWithInvalidation(rbacResolver, "admin.moderation")).
				Delete("/admin/users/{id}/restrict/{operation}", adminHandlers.LiftRestriction)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermissionWithInvalidation(rbacResolver, "admin.moderation")).
				Delete("/admin/users/{id}/restrict", adminHandlers.LiftAllRestrictions)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Get("/admin/users/{id}/restrictions", adminHandlers.ListRestrictions)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Post("/admin/users/{id}/suspend", adminHandlers.SuspendUser)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Post("/admin/users/{id}/warn-typed", adminHandlers.WarnUserTyped)

			// Task 22: Appeals admin
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "reports.review")).
				Get("/admin/appeals", adminHandlers.ListAppeals)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "reports.review")).
				Get("/admin/appeals/{id}", adminHandlers.GetAppeal)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "reports.review")).
				Patch("/admin/appeals/{id}", adminHandlers.UpdateAppeal)

			// Task 23: Content admin moderation
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Get("/admin/content", contentHandlers.ListContent)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.edit")).
				Patch("/admin/content/{id}", contentHandlers.UpdateContent)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.delete")).
				Get("/admin/content/{id}/remove", adminHandlers.RemoveContent)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.delete")).
				Post("/admin/content/{id}/remove", adminHandlers.RemoveContent)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.edit")).
				Post("/admin/content/{id}/restore", adminHandlers.RestoreContent)

			// Task 24: Affiliate admin commission rates
			r.With(features.Gate(featureChecker, "feature.affiliate_program")).Group(func(r chi.Router) {
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.admin.manage")).
					Get("/admin/affiliate-program/commission-rates", affiliateHandlers.GetCommissionRates)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.admin.manage")).
					Patch("/admin/affiliate-program/commission-rates", affiliateHandlers.UpdateCommissionRates)
			})

			// Affiliate enhancements: author revenue sharing + unregistered link detection
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.admin.manage")).
				Get("/admin/affiliate/scan-results", couponHandlers.ListAffiliateScanResults)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.admin.manage")).
				Patch("/admin/affiliate/scan-results/{id}", couponHandlers.ReviewAffiliateScanResult)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.admin.manage")).
				Get("/admin/affiliate/commissions", couponHandlers.ListAffiliateCommissions)
			// AUDIT-BE-TASK-001 / audit route-diff: RecordAffiliateCommission was
			// fully implemented (handler + service + task enqueue) but never
			// registered as a route, so admins could never record an external
			// commission and the affiliate:credit_author_commission task never fired.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.admin.manage")).
				Post("/admin/affiliate/commissions", couponHandlers.RecordAffiliateCommission)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.admin.manage")).
				Patch("/admin/affiliate/providers/{id}/author-commission", couponHandlers.UpdateProviderAuthorCommission)

			// Task 25: i18n AI translate
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "i18n.manage")).
				Post("/admin/i18n/strings/{id}/translate", aiHandlers.HandleTranslate)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "i18n.manage")).
				Post("/admin/i18n/strings/ai-translate-batch", aiHandlers.HandleTranslate)

			// Task 26: Moderation queue assign + SEO validate
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Patch("/admin/moderation-queue/{id}/assign", adminHandlers.AssignModerationItem)
			// SEO validate is an admin utility (Section 26); gated by the same
			// admin.settings node the admin sidebar uses for /seo (apps/admin
			// +layout.svelte nav map), so the UI gate and backend gate agree.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings")).
				Get("/admin/seo/validate/{id}", sitemapHandler.ValidateSEO)

			// Task 27: Pagebuilder admin extra + public pages
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pages.manage")).
				Post("/admin/pages/{id}/save-as-template", pagebuilderHandlers.SaveAsTemplate)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pages.manage")).
				Post("/admin/pages/{id}/duplicate", pagebuilderHandlers.DuplicatePage)
			r.Get("/pages", pagebuilderHandlers.ListPublishedPages)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pages.manage")).
				Post("/pages", pagebuilderHandlers.CreatePage)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pages.manage")).
				Patch("/pages/{id}", pagebuilderHandlers.UpdatePage)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pages.manage")).
				Delete("/pages/{id}", pagebuilderHandlers.DeletePage)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pages.manage")).
				Patch("/pages/{id}/blocks", pagebuilderHandlers.ReorderBlocks)
			// Public widget render endpoint (Section 16 — Valkey-cached by
			// widget.cache_ttl_secs, key jc:widget:{id}).
			r.Get("/widgets/{id}", widgetHandlers.RenderWidget)

			// Task 28: Me additions
			// Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md
			// /me/account updates username (30-day cooldown), email (re-verify),
			// display_name — distinct from /me/profile (avatar/bio/timezone).
			// Gated by user.account.manage_own (Section 50 line 263).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "user.account.manage_own")).
				Patch("/me/account", usersHandlers.UpdateMyAccount)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "api.keys.self_manage")).
				Get("/me/api-keys/{id}", publicAPIHandlers.GetAPIKey)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "api.keys.self_manage")).
				Patch("/me/api-keys/{id}", publicAPIHandlers.UpdateAPIKey)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Delete("/me/push-tokens/{id}", notificationHandlers.HandlePushUnsubscribeByID)

			// Task 29: AI conversations — gated by feature.japabot +
			// ai.context.use RBAC, matching /ai/chat (Section 20: JapaBot
			// conversational AI is gated by ai.context.* RBAC). Without
			// these gates a user whose JapaBot access was revoked could
			// still list/read their Honcho conversation history.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.japabot"), rbac.RequirePermission(rbacResolver, "ai.context.use")).
				Get("/ai/conversations", aiHandlers.HandleListConversations)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.japabot"), rbac.RequirePermission(rbacResolver, "ai.context.use")).
				Get("/ai/conversations/{id}", aiHandlers.HandleGetConversation)

			// Task 33: Content public extras
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/content/{id}/analytics", analyticsHandlers.GetContentAnalytics)
			// BUG-001: /content/{id}/discussions is the thread-link feature
			// (Section 49 - list linked discussions / start a conversation), not
			// the plain comment list/create handlers.
			r.With(auth.OptionalAuth(kr, vkClient.Redis())).
				Get("/content/{id}/discussions", commentsHandlers.ListDiscussions)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comments.create")).
				Post("/content/{id}/discussions", commentsHandlers.StartConversation)
			// NOTE: /content/{id}/react (POST+DELETE) and /content/{id}/spaces
			// (PATCH) are registered once in the main content route group above
			// (see "Singular reaction action routes"). They must not be
			// re-registered here — chi treats the second registration as a
			// duplicate and TestNoDuplicateRoutes fails.

			// Task 34: Users public extras
			// Profile Analytics dashboard — owner-only per JEOS §342 [auth, self].
			// RequireAuth blocks guests; the handler blocks non-owner users with
			// 403 analytics_forbidden. requireActiveProfileUser still 404s for
			// banned/suspended/deleted users.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), requireActiveProfileUser(usersService, "id")).
				Get("/users/{id}/analytics", usersHandlers.GetUserAnalytics)
			// Public profile Posts tab — author-scoped published+public content (was
			// wired to ListContent, which ignored {id} and returned all tenant content).
			// Public per Section 32; gated to 404 non-active users.
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), requireActiveProfileUser(usersService, "id")).
				Get("/users/{id}/content", contentHandlers.ListByAuthor)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "follows.toggle"), mw.RateLimit(vkClient.Redis(), "moderate")).
				Post("/users/{id}/follow", followHandlers.ToggleFollow)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "follows.toggle"), mw.RateLimit(vkClient.Redis(), "moderate")).
				Delete("/users/{id}/follow", followHandlers.ToggleFollow)

			// Task 35: Suggestions/content — BUG-003: now uses pgvector similarity
			// (Section 53 "You might also like") instead of returning trending content.
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/suggestions/content", trendingHandlers.GetSuggestedContent)

			// ──────────────────────────────────────────────────────────────
			// V2 feature routes (Sections 61–76, ADRs 068, 070–085).
			// Each feature's settings toggle (e.g. jobs.enabled, remittance.enabled)
			// is enforced inside the handler via settingsService, not via the
			// feature.* flag gate, matching the RAG pattern (Section 69).
			// ──────────────────────────────────────────────────────────────

			// Section 63 — Jobs Board (ADR-081)
			// Public browse + auth apply + org admin CRUD + admin moderation.
			r.Get("/jobs", jobsHandlers.HandleFindJobs)
			r.Get("/jobs/{slug}", jobsHandlers.HandleJobDetail)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "jobs.apply")).
				Post("/jobs/{id}/apply", jobsHandlers.HandleApply)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "jobs.apply")).
				Get("/me/jobs/applications", jobsHandlers.HandleMyApplications)
			// Org-scoped employer routes (Section 63).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "jobs.post")).
				Get("/org/{org_id}/jobs", jobsHandlers.HandleListOrgJobs)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "jobs.post")).
				Post("/org/{org_id}/jobs", jobsHandlers.HandleCreateJobListing)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "jobs.post")).
				Get("/org/{org_id}/jobs/{id}/applications", jobsHandlers.HandleListJobApplications)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "jobs.post")).
				Patch("/org/{org_id}/jobs/{id}/applications/{app_id}", jobsHandlers.HandleUpdateApplicationStatus)
			// Admin moderation (Section 63).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "jobs.manage")).
				Get("/admin/jobs", jobsHandlers.HandleAdminListJobs)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "jobs.manage")).
				Patch("/admin/jobs/{id}/feature", jobsHandlers.HandleAdminToggleFeature)
			// Admin CRUD for job listings — jobs.manage
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "jobs.manage")).
				Patch("/admin/jobs/{id}", jobsHandlers.HandleAdminUpdateJob)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "jobs.manage")).
				Delete("/admin/jobs/{id}", jobsHandlers.HandleAdminDeleteJob)

			// Section 64 — Advanced Gamification (ADR-082)
			r.Get("/gamification/badges", gamificationHandlers.HandleListBadges)
			r.Get("/gamification/challenges", gamificationHandlers.HandleListChallenges)
			r.Get("/gamification/leaderboard", gamificationHandlers.HandleGetLeaderboard)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/me/badges", gamificationHandlers.HandleMyBadges)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/me/challenges", gamificationHandlers.HandleMyChallenges)
			r.Get("/users/{username}/badges", gamificationHandlers.HandleUserBadges)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gamification.manage_badges")).
				Get("/admin/gamification/badges", gamificationHandlers.HandleAdminListBadges)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gamification.manage_badges")).
				Post("/admin/gamification/badges", gamificationHandlers.HandleAdminCreateBadge)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gamification.manage_badges")).
				Patch("/admin/gamification/badges/{id}", gamificationHandlers.HandleAdminUpdateBadge)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gamification.manage_badges")).
				Delete("/admin/gamification/badges/{id}", gamificationHandlers.HandleAdminDeleteBadge)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gamification.award_badge")).
				Post("/admin/gamification/badges/{id}/award", gamificationHandlers.HandleAdminAwardBadge)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gamification.manage_challenges")).
				Get("/admin/gamification/challenges", gamificationHandlers.HandleAdminListChallenges)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gamification.manage_challenges")).
				Post("/admin/gamification/challenges", gamificationHandlers.HandleAdminCreateChallenge)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gamification.manage_challenges")).
				Patch("/admin/gamification/challenges/{id}", gamificationHandlers.HandleAdminUpdateChallenge)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gamification.manage_challenges")).
				Delete("/admin/gamification/challenges/{id}", gamificationHandlers.HandleAdminDeleteChallenge)

			// Section 65 — Content Import & Migration (ADR-083)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "import.use")).
				Post("/me/imports", contentImportHandlers.HandleStartImport)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "import.use")).
				Get("/me/imports", contentImportHandlers.HandleListImports)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "import.use")).
				Get("/me/imports/sources", contentImportHandlers.HandleListImportSources)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "import.use")).
				Get("/me/imports/{id}", contentImportHandlers.HandleGetImport)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "import.manage")).
				Get("/admin/imports", contentImportHandlers.HandleAdminListImports)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "import.manage")).
				Post("/admin/imports", contentImportHandlers.HandleAdminStartImport)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "import.manage")).
				Post("/admin/imports/{id}/retry", contentImportHandlers.HandleAdminRetryImport)

			// Section 66 — Advanced Analytics Dashboard (ADR-084)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "analytics.advanced")).
				Get("/admin/analytics/top-content", advancedAnalyticsHandlers.HandleTopContent)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "analytics.advanced")).
				Get("/admin/analytics/revenue-streams", advancedAnalyticsHandlers.HandleRevenueStreams)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "analytics.advanced")).
				Get("/admin/analytics/ads", advancedAnalyticsHandlers.HandleAdPerformance)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "analytics.advanced")).
				Get("/admin/analytics/newsletter", advancedAnalyticsHandlers.HandleNewsletterEngagement)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "analytics.advanced")).
				Get("/admin/analytics/retention", advancedAnalyticsHandlers.HandleRetention)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "analytics.export")).
				Get("/admin/analytics/export", advancedAnalyticsHandlers.HandleExport)

			// Section 67 — Travel Search & Compare (ADR-085)
			// Feature-flagged via features.Gate (returns 404 when disabled, ADR-042).
			r.With(features.Gate(featureChecker, "travel.enabled")).Group(func(r chi.Router) {
				r.Get("/travel/search/flights", travelHandlers.HandleSearchFlights)
				r.Get("/travel/search/hotels", travelHandlers.HandleSearchHotels)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "travel.manage")).
					Get("/admin/travel/providers", travelHandlers.HandleAdminListProviders)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "travel.manage")).
					Post("/admin/travel/providers", travelHandlers.HandleAdminCreateProvider)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "travel.manage")).
					Patch("/admin/travel/providers/{id}", travelHandlers.HandleAdminUpdateProvider)
			})

			// Section 68 — AI Persona System (ADR-068)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "persona.view")).
				Get("/admin/personas", personasHandlers.HandleList)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "persona.manage"), mw.RateLimit(vkClient.Redis(), "mass_provision")).
				Post("/admin/personas", personasHandlers.HandleCreate)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "persona.view")).
				Get("/admin/personas/{id}", personasHandlers.HandleGet)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "persona.manage")).
				Patch("/admin/personas/{id}", personasHandlers.HandleUpdate)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "participation.view")).
				Get("/admin/personas/{id}/activity", personasHandlers.HandleActivity)

			// Section 75 — AI Content Studio (ADR-076)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "studio.view")).
				Get("/admin/studio/jobs", studioHandlers.HandleListJobs)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "studio.create")).
				Post("/admin/studio/jobs", studioHandlers.HandleCreateJob)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "studio.view")).
				Get("/admin/studio/jobs/{id}", studioHandlers.HandleGetJob)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "studio.manage")).
				Post("/admin/studio/jobs/{id}/retry", studioHandlers.HandleRetryJob)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "studio.manage")).
				Delete("/admin/studio/jobs/{id}", studioHandlers.HandleCancelJob)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "studio.view")).
				Get("/admin/studio/batches", studioHandlers.HandleListBatches)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "studio.create")).
				Post("/admin/studio/batches", studioHandlers.HandleCreateBatch)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "studio.view")).
				Get("/admin/studio/batches/{id}", studioHandlers.HandleGetBatch)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "studio.manage")).
				Post("/admin/studio/batches/{id}/retry", studioHandlers.HandleRetryBatch)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "studio.view")).
				Get("/admin/studio/personas", studioHandlers.HandlePersonas)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "studio.view")).
				Get("/admin/studio/content-types", studioHandlers.HandleContentTypes)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "studio.view")).
				Get("/admin/studio/stats", studioHandlers.HandleStats)

			// Section 76 — Participation Orchestration (ADR-077)
			// Admin queue/stats endpoints. The queue is the review surface
			// (participation.review, BUG-006); stats are read-only
			// (participation.view). Review approve/reject reuses
			// PATCH /admin/moderation-queue/:id (wired to content_reviews
			// in admin.ModerateItem, BUG-001).
			participationHandlers := participation.NewHandlers(database, participationService, settingsService, vkClient.Redis())
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "participation.review")).
				Get("/admin/participation/queue", participationHandlers.HandleQueue)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "participation.view")).
				Get("/admin/participation/stats", participationHandlers.HandleStats)

			// Section 70 — Visa Document Checklist Generator (ADR-070)
			// Feature-flagged via features.Gate (returns 404 when disabled, ADR-042).
			r.With(features.Gate(featureChecker, "checklist.enabled")).Group(func(r chi.Router) {
				r.Get("/checklist/selector", checklistHandlers.HandleSelector)
				r.Get("/checklist/{guide_id}", checklistHandlers.HandleGetItems)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "checklist.use")).
					Get("/checklist/{guide_id}/progress", checklistHandlers.HandleGetProgress)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "checklist.use")).
					Patch("/checklist/{guide_id}/progress/{item_key}", checklistHandlers.HandleUpdateProgress)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "checklist.use")).
					Delete("/checklist/{guide_id}/progress", checklistHandlers.HandleResetProgress)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "checklist.manage")).
					Get("/admin/checklist/progress", checklistHandlers.HandleAdminListProgress)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "checklist.manage")).
					Get("/admin/checklist/completion-stats", checklistHandlers.HandleAdminCompletionStats)
			})

			// Section 79 — Community Story Wall (ADR-087)
			// Feature-flagged via features.Gate (returns 404 when disabled, ADR-042).
			r.With(features.Gate(featureChecker, "story.enabled")).Group(func(r chi.Router) {
				// Public browse (anonymous): list + single story by slug.
				r.Get("/stories", storyHandlers.HandleList)
				r.Get("/stories/{slug}", storyHandlers.HandleGetBySlug)
				// Authenticated: submit, edit own/any, delete own/any.
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "story.create")).
					Post("/stories", storyHandlers.HandleSubmit)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequireAnyPermission(rbacResolver, "story.edit_own", "story.edit_any")).
					Patch("/stories/{id}", storyHandlers.HandleEdit)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequireAnyPermission(rbacResolver, "story.delete_own", "story.delete_any")).
					Delete("/stories/{id}", storyHandlers.HandleDelete)
				// Admin: verify + moderation queue + admin delete.
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "story.verify")).
					Post("/admin/stories/{id}/verify", storyHandlers.HandleVerify)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "story.verify")).
					Get("/admin/stories", storyHandlers.HandleAdminList)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "story.delete_any")).
					Delete("/admin/stories/{id}", storyHandlers.HandleAdminDelete)
			})

			// Section 80 — Destination Wiki (ADR-088)
			// Feature-flagged via features.Gate (returns 404 when disabled, ADR-042).
			r.With(features.Gate(featureChecker, "wiki.enabled")).Group(func(r chi.Router) {
				// Public browse (anonymous): wiki index + single page + history.
				r.Get("/wiki/{space_slug}", wikiHandlers.HandleGetIndex)
				r.Get("/wiki/{space_slug}/{page_slug}", wikiHandlers.HandleGetPage)
				r.Get("/wiki/{space_slug}/{page_slug}/history", wikiHandlers.HandleGetHistory)
				// Authenticated: submit revision (wiki.edit permission).
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wiki.edit")).
					Post("/wiki/{space_slug}/{page_slug}/revisions", wikiHandlers.HandleSubmitRevision)
				// Admin: review revisions (wiki.review permission).
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wiki.review")).
					Post("/admin/wiki/{page_id}/revisions/{id}/approve", wikiHandlers.HandleApproveRevision)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wiki.review")).
					Post("/admin/wiki/{page_id}/revisions/{id}/reject", wikiHandlers.HandleRejectRevision)
				// Admin: manage pages (wiki.manage permission).
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wiki.manage")).
					Patch("/admin/wiki/{page_id}", wikiHandlers.HandleUpdatePage)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wiki.manage")).
					Post("/admin/wiki/generate", wikiHandlers.HandleGenerate)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wiki.manage")).
					Get("/admin/wiki", wikiHandlers.HandleAdminList)
			})

			// Section 81 — Scam Alert Board (ADR-089)
			r.With(features.Gate(featureChecker, "scam_alert.enabled")).Group(func(r chi.Router) {
				r.Get("/scam-alerts", scamAlertHandlers.HandleList)
				r.Get("/scam-alerts/{id}", scamAlertHandlers.HandleGet)
				r.Post("/scam-alerts", scamAlertHandlers.HandleSubmit)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).
					Post("/scam-alerts/{id}/subscribe", scamAlertHandlers.HandleSubscribe)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).
					Delete("/scam-alerts/{id}/subscribe", scamAlertHandlers.HandleUnsubscribe)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "scam_alert.review")).
					Get("/admin/scam-alerts", scamAlertHandlers.HandleAdminList)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "scam_alert.review")).
					Patch("/admin/scam-alerts/{id}", scamAlertHandlers.HandleReview)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "scam_alert.pin")).
					Post("/admin/scam-alerts/{id}/pin", scamAlertHandlers.HandlePin)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "scam_alert.pin")).
					Delete("/admin/scam-alerts/{id}/pin", scamAlertHandlers.HandleUnpin)
			})

			// Section 82 — Japa Planning Groups (ADR-090)
			r.With(features.Gate(featureChecker, "planning_group.enabled")).Group(func(r chi.Router) {
				r.Get("/planning-groups", planningGroupHandlers.HandleList)
				r.Get("/planning-groups/{id}", planningGroupHandlers.HandleGet)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "planning_group.create")).
					Post("/planning-groups", planningGroupHandlers.HandleCreate)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "planning_group.create")).
					Post("/planning-groups/{id}/join", planningGroupHandlers.HandleJoin)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "planning_group.manage_own")).
					Post("/planning-groups/{id}/invite", planningGroupHandlers.HandleInvite)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).
					Delete("/planning-groups/{id}/leave", planningGroupHandlers.HandleLeave)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).
					Get("/planning-groups/{id}/members", planningGroupHandlers.HandleGetMembers)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "planning_group.manage_own")).
					Patch("/planning-groups/{id}", planningGroupHandlers.HandleUpdate)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "planning_group.manage_own")).
					Delete("/planning-groups/{id}", planningGroupHandlers.HandleDelete)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).
					Get("/planning-groups/{id}/milestones", planningGroupHandlers.HandleListMilestones)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).
					Post("/planning-groups/{id}/milestones", planningGroupHandlers.HandleCreateMilestone)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).
					Patch("/planning-groups/{id}/milestones/{mid}", planningGroupHandlers.HandleUpdateMilestone)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).
					Get("/planning-groups/{id}/checkins", planningGroupHandlers.HandleListCheckins)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).
					Post("/planning-groups/{id}/checkins", planningGroupHandlers.HandleCreateCheckin)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "planning_group.manage_any")).
					Get("/admin/planning-groups", planningGroupHandlers.HandleAdminList)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "planning_group.manage_any")).
					Post("/admin/planning-groups/{id}/extend", planningGroupHandlers.HandleAdminExtend)
			})

			// Section 83 — Visa Timeline Tracker (ADR-091)
			r.With(features.Gate(featureChecker, "visa_timeline.enabled")).Group(func(r chi.Router) {
				r.Get("/visa-timelines/stats", visaTimelineHandlers.HandleStats)
				r.Get("/visa-timelines/stats/trend", visaTimelineHandlers.HandleTrend)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "visa_timeline.create")).
					Get("/me/visa-timelines", visaTimelineHandlers.HandleListOwn)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "visa_timeline.create")).
					Post("/me/visa-timelines", visaTimelineHandlers.HandleCreate)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "visa_timeline.manage_own")).
					Patch("/me/visa-timelines/{id}", visaTimelineHandlers.HandleUpdate)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "visa_timeline.manage_own")).
					Post("/me/visa-timelines/{id}/milestones", visaTimelineHandlers.HandleAddMilestone)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "visa_timeline.manage_own")).
					Patch("/me/visa-timelines/{id}/milestones/{mid}", visaTimelineHandlers.HandleUpdateMilestone)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "visa_timeline.manage_any")).
					Get("/admin/visa-timelines", visaTimelineHandlers.HandleAdminList)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "visa_timeline.manage_any")).
					Delete("/admin/visa-timelines/{id}", visaTimelineHandlers.HandleAdminDelete)
			})

			// Section 84 — Community Digest Widget (ADR-092)
			r.With(features.Gate(featureChecker, "digest.enabled")).Group(func(r chi.Router) {
				r.Get("/digest/latest", digestHandlers.HandleGetLatest)
				r.Get("/digest/{id}", digestHandlers.HandleGet)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "digest.manage")).
					Get("/admin/digest", digestHandlers.HandleAdminList)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "digest.manage")).
					Get("/admin/digest/{id}", digestHandlers.HandleAdminGet)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "digest.manage")).
					Patch("/admin/digest/{id}", digestHandlers.HandleAdminUpdate)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "digest.manage")).
					Post("/admin/digest/{id}/publish", digestHandlers.HandleAdminPublish)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "digest.manage")).
					Post("/admin/digest/generate", digestHandlers.HandleAdminGenerate)
			})

			// Section 85 — Mutual Aid Board (ADR-093)
			r.With(features.Gate(featureChecker, "mutual_aid.enabled")).Group(func(r chi.Router) {
				r.Get("/mutual-aid", mutualAidHandlers.HandleList)
				r.Get("/mutual-aid/{id}", mutualAidHandlers.HandleGet)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "mutual_aid.create_offer")).
					Post("/mutual-aid", mutualAidHandlers.HandleCreate)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "mutual_aid.create_offer")).
					Patch("/mutual-aid/{id}", mutualAidHandlers.HandleUpdate)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).
					Post("/mutual-aid/{id}/respond", mutualAidHandlers.HandleRespond)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).
					Post("/mutual-aid/{id}/fulfill", mutualAidHandlers.HandleFulfill)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).
					Post("/mutual-aid/{id}/renew", mutualAidHandlers.HandleRenew)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).
					Delete("/mutual-aid/{id}", mutualAidHandlers.HandleDelete)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "mutual_aid.moderate")).
					Get("/admin/mutual-aid", mutualAidHandlers.HandleAdminList)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "mutual_aid.moderate")).
					Patch("/admin/mutual-aid/{id}/status", mutualAidHandlers.HandleAdminModerate)
			})

			// Section 86 — Community Calendar (ADR-094)
			r.With(features.Gate(featureChecker, "calendar.enabled")).Group(func(r chi.Router) {
				r.Get("/calendar/categories", calendarHandlers.HandleCategories)
				r.Get("/calendar", calendarHandlers.HandleList)
				r.Get("/calendar/{id}", calendarHandlers.HandleGet)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).
					Post("/calendar/{id}/save", calendarHandlers.HandleSave)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).
					Delete("/calendar/{id}/save", calendarHandlers.HandleUnsave)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).
					Post("/calendar/subscribe", calendarHandlers.HandleSubscribe)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).
					Delete("/calendar/subscribe", calendarHandlers.HandleUnsubscribe)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "calendar.submit")).
					Post("/calendar", calendarHandlers.HandleSubmit)
				// BUG-003: /me/calendar/saved — user dashboard saved entries (§86 Integration).
				r.With(auth.RequireAuth(kr, vkClient.Redis())).
					Get("/me/calendar/saved", calendarHandlers.HandleListSaved)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "calendar.review")).
					Get("/admin/calendar", calendarHandlers.HandleAdminList)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "calendar.review")).
					Patch("/admin/calendar/{id}", calendarHandlers.HandleAdminUpdate)
					// BUG-001/BUG-008: soft-delete admin endpoint gated by calendar.manage (admin+).
					// Per Rule 11, user content is soft-deleted, never hard-deleted.
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "calendar.manage")).
					Delete("/admin/calendar/{id}", calendarHandlers.HandleAdminDelete)
			})

			// Section 87 — Japa Stage Progress Profiles (ADR-095)
			r.With(features.Gate(featureChecker, "japa_stage.enabled")).Group(func(r chi.Router) {
				r.Get("/users/{username}/japa-stage", japaStageHandlers.HandleGetByUsername)
				r.Get("/members/by-stage", japaStageHandlers.HandleListByStage)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "profile.japa_stage.edit")).
					Get("/me/japa-stage", japaStageHandlers.HandleGetOwn)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "profile.japa_stage.edit")).
					Patch("/me/japa-stage", japaStageHandlers.HandleUpdateOwn)
			})

			// Section 88 — Ask an Expat AMA (ADR-096)
			// AMA routes are registered inside the forum group (before
			// /forum/{slug}) to avoid route shadowing. See line ~2037.

			// Section 89 — Community Achievement Feed (ADR-097)
			r.With(features.Gate(featureChecker, "achievement.enabled")).Group(func(r chi.Router) {
				r.Get("/achievements", achievementHandlers.HandleList)
				r.Get("/achievements/{slug}", achievementHandlers.HandleGet)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "achievement.create")).
					Post("/achievements", achievementHandlers.HandleSubmit)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "achievement.edit_own")).
					Patch("/achievements/{id}", achievementHandlers.HandleEdit)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "achievement.delete_own")).
					Delete("/achievements/{id}", achievementHandlers.HandleDelete)
				r.Get("/spaces/{slug}/achievements", achievementHandlers.HandleListBySpace)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "achievement.moderate")).
					Get("/admin/achievements", achievementHandlers.HandleAdminList)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "achievement.moderate")).
					Patch("/admin/achievements/{id}/status", achievementHandlers.HandleAdminSetStatus)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "achievement.moderate")).
					Delete("/admin/achievements/{id}", achievementHandlers.HandleAdminDelete)
			})

			// Section 98 — Service Directory (ADR-106)
			r.With(features.Gate(featureChecker, "service_directory.enabled")).Group(func(r chi.Router) {
				r.Get("/services", serviceDirHandlers.HandleList)
				r.Get("/services/{slug}", serviceDirHandlers.HandleGet)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "service.create")).
					Post("/services", serviceDirHandlers.HandleCreate)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequireAnyPermission(rbacResolver, "service.edit_own", "service.edit_any")).
					Patch("/services/{id}", serviceDirHandlers.HandleEdit)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "service.claim")).
					Post("/services/{id}/claim", serviceDirHandlers.HandleClaim)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "service.verify")).
					Get("/admin/services", serviceDirHandlers.HandleAdminList)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "service.verify")).
					Patch("/admin/services/{id}/verify", serviceDirHandlers.HandleAdminVerify)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "service.verify")).
					Patch("/admin/services/{id}/claim/{claim_id}", serviceDirHandlers.HandleAdminResolveClaim)
			})

			// Section 71 — Japa Readiness Assessment (ADR-071)
			// GET /assessment is public (active form + scoring metadata).
			// POST /assessment/submit and GET /assessment/result/* are public:
			// §71 allows anonymous users to take the assessment; the handlers
			// do optional auth + ownership checks internally. Anonymous results
			// (user_id null) are publicly viewable; user results require the
			// owner (enforced in the handler).
			// Submit is rate-limited to 10 per user (or IP for anonymous) per hour,
			// matching the §06 form_submission limiter (§23 — form-spam prevention).
			// Feature-flagged via features.Gate (returns 404 when disabled, ADR-042).
			r.With(features.Gate(featureChecker, "assessment.enabled")).Group(func(r chi.Router) {
				r.Get("/assessment", assessmentHandlers.HandleGetAssessment)
				r.With(auth.OptionalAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "form_submission")).
					Post("/assessment/submit", assessmentHandlers.HandleSubmit)
				r.Get("/assessment/result/by-submission/{submission_id}", assessmentHandlers.HandleGetResultBySubmission)
				r.Get("/assessment/result/{id}", assessmentHandlers.HandleGetResult)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "assessment.take")).
					Get("/assessment/history", assessmentHandlers.HandleHistory)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "assessment.manage")).
					Get("/admin/assessment/form", assessmentHandlers.HandleAdminGetForm)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "assessment.manage")).
					Patch("/admin/assessment/form", assessmentHandlers.HandleAdminUpdateForm)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "assessment.view_results")).
					Get("/admin/assessment/results", assessmentHandlers.HandleAdminListResults)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "assessment.view_results")).
					Get("/admin/assessment/stats", assessmentHandlers.HandleAdminStats)
			})

			// Section 72 — Cost of Living Calculator (ADR-072)
			// Public browse routes use OptionalAuth + OptionalPermission so anonymous
			// users can access the calculator (ADR-072 §7: "all users including
			// anonymous") while authed users whose roles have had col_calculator.use
			// revoked are denied (403). This makes the seeded col_calculator.use node
			// a real killswitch instead of dead — same pattern as scholarship.browse.
			// Feature-flagged via features.Gate (returns 404 when disabled, ADR-042).
			r.With(features.Gate(featureChecker, "col_calculator.enabled")).Group(func(r chi.Router) {
				r.With(auth.OptionalAuth(kr, vkClient.Redis()), rbac.OptionalPermission(rbacResolver, "col_calculator.use")).
					Get("/calculator/cost-of-living/cities", costOfLivingHandlers.HandleListCities)
				r.With(auth.OptionalAuth(kr, vkClient.Redis()), rbac.OptionalPermission(rbacResolver, "col_calculator.use")).
					Get("/calculator/cost-of-living/compare", costOfLivingHandlers.HandleCompare)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "col_calculator.manage")).
					Get("/admin/calculator/cost-of-living/stats", costOfLivingHandlers.HandleAdminStats)
			})

			// Section 73 — Scholarship Finder (ADR-073)
			// Browse routes use OptionalAuth + OptionalPermission so anonymous
			// users can browse (ADR-073 §7) while authed users whose roles have
			// had scholarship.browse revoked are denied (403). This makes the
			// seeded scholarship.browse node a real killswitch instead of dead.
			// Feature-flagged via features.Gate (returns 404 when disabled, ADR-042).
			r.With(features.Gate(featureChecker, "scholarship_finder.enabled")).Group(func(r chi.Router) {
				r.With(auth.OptionalAuth(kr, vkClient.Redis()), rbac.OptionalPermission(rbacResolver, "scholarship.browse")).
					Get("/scholarships", scholarshipsHandlers.HandleFindScholarships)
				r.With(auth.OptionalAuth(kr, vkClient.Redis()), rbac.OptionalPermission(rbacResolver, "scholarship.browse")).
					Get("/scholarships/{id}", scholarshipsHandlers.HandleScholarshipDetail)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "scholarship.save")).
					Get("/scholarships/saved", scholarshipsHandlers.HandleListSavedScholarships)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "scholarship.save")).
					Post("/scholarships/{id}/save", scholarshipsHandlers.HandleSaveScholarship)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "scholarship.save")).
					Patch("/scholarships/{id}/save", scholarshipsHandlers.HandleUpdateSavedScholarship)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "scholarship.save")).
					Delete("/scholarships/{id}/save", scholarshipsHandlers.HandleDeleteSavedScholarship)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "scholarship.manage")).
					Get("/admin/scholarships/stats", scholarshipsHandlers.HandleAdminStatusDistribution)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "scholarship.manage")).
					Get("/admin/scholarships/deadlines", scholarshipsHandlers.HandleAdminUpcomingDeadlines)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "scholarship.manage")).
					Get("/admin/scholarships/reminder-log", scholarshipsHandlers.HandleAdminReminderLog)
				// Admin CRUD for scholarship records — scholarship.manage
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "scholarship.manage")).
					Get("/admin/scholarships", scholarshipsHandlers.HandleAdminListScholarships)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "scholarship.manage")).
					Post("/admin/scholarships", scholarshipsHandlers.HandleAdminCreateScholarship)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "scholarship.manage")).
					Patch("/admin/scholarships/{id}", scholarshipsHandlers.HandleAdminUpdateScholarship)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "scholarship.manage")).
					Delete("/admin/scholarships/{id}", scholarshipsHandlers.HandleAdminDeleteScholarship)
			})

			// Section INTEL — Intelligence System (radar + scholarships + sources)
			// Radar feed + counts use OptionalAuth so anonymous users can
			// browse; follow/unfollow, saved, and save/unsave require auth.
			// The scholarship search + detail routes mirror the §73 browse
			// pattern (OptionalAuth). The source directory is public.
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), rbac.OptionalPermission(rbacResolver, "intel.radar.view")).
				Get("/intel/radar", intelHandlers.HandleRadarFeed)
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), rbac.OptionalPermission(rbacResolver, "intel.radar.view")).
				Get("/intel/radar/counts", intelHandlers.HandleRadarCounts)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "intel.radar.view")).
				Get("/intel/radar/followed", intelHandlers.HandleRadarFollowed)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "intel.radar.view")).
				Post("/intel/radar/follow", intelHandlers.HandleRadarFollow)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "intel.radar.view")).
				Delete("/intel/radar/follow", intelHandlers.HandleRadarUnfollow)
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), rbac.OptionalPermission(rbacResolver, "intel.radar.view")).
				Get("/intel/radar/feed/{item_id}", intelHandlers.HandleRadarItemDetail)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "intel.radar.view")).
				Get("/intel/radar/saved", intelHandlers.HandleRadarSaved)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "intel.radar.view")).
				Post("/intel/radar/save", intelHandlers.HandleRadarSave)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "intel.radar.view")).
				Delete("/intel/radar/save/{item_id}", intelHandlers.HandleRadarUnsave)
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), rbac.OptionalPermission(rbacResolver, "intel.radar.view")).
				Get("/intel/scholarships", intelHandlers.HandleSearchScholarships)
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), rbac.OptionalPermission(rbacResolver, "intel.radar.view")).
				Get("/intel/scholarships/{id}", intelHandlers.HandleScholarshipDetail)
			// Sources directory — RBAC-gated by intel.sources.manage (control
			// panel admin only). The source registry is admin data, not a public
			// surface; extracted intelligence data is what the main app shows.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "intel.sources.manage")).
				Get("/intel/sources", intelHandlers.HandleSourcesDirectory)
			// Sources CRUD — RBAC-gated by intel.sources.manage (control panel
			// admin only). BUG-002 fix: the control panel calls these routes but
			// they were not registered, so source create/edit/delete returned 404.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "intel.sources.manage")).
				Post("/intel/sources", intelHandlers.HandleCreateSource)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "intel.sources.manage")).
				Patch("/intel/sources/{id}", intelHandlers.HandleUpdateSource)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "intel.sources.manage")).
				Delete("/intel/sources/{id}", intelHandlers.HandleDeleteSource)
			// Crawl-now — triggers an immediate fetch of a source (INTEL-011 fix).
			// RBAC-gated by intel.sources.crawl_now (migration 000431).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "intel.sources.crawl_now")).
				Post("/intel/sources/{id}/crawl-now", intelHandlers.HandleCrawlNow)
			// Disable source from the review queue surface (INTEL-011 fix).
			// RBAC-gated by intel.review.disable_source (migration 000431).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "intel.review.disable_source")).
				Post("/intel/sources/{id}/disable", intelHandlers.HandleDisableSource)
			// Raw document fetch — RBAC-gated by intel.review.view (control panel
			// four-pane moderation raw content pane). BUG-003 fix.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "intel.review.view")).
				Get("/intel/raw-documents/{id}", intelHandlers.HandleGetRawDocument)
			// Review queue — RBAC-gated by intel.review.* (admin only).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "intel.review.view")).
				Get("/intel/review/queue", intelHandlers.HandleReviewQueue)
			// Intel records list (by status) — used by the control panel
			// review list page (non-review_required statuses) and the
			// intelligence overview page queue counts. BUG-A fix: the prior
			// running binary served this route but it was missing from the
			// source/router, so a rebuild would 404 the status filter.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "intel.review.view")).
				Get("/intel/records", intelHandlers.HandleListIntelRecords)
			// Intel record detail (single, any status) — used by the control
			// panel four-pane moderation detail page. BUG-B fix: the prior
			// approach scanned the review_required queue, so approved/
			// rejected/published records were unreachable from the detail
			// page.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "intel.review.view")).
				Get("/intel/records/{id}", intelHandlers.HandleGetIntelRecord)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "intel.review.approve")).
				Post("/intel/review/{record_id}/approve", intelHandlers.HandleReviewApprove)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "intel.review.reject")).
				Post("/intel/review/{record_id}/reject", intelHandlers.HandleReviewReject)
			// Review edit — saves admin field edits to editorial_overrides.
			// RBAC-gated by intel.review.edit. BUG-004 fix.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "intel.review.edit")).
				Patch("/intel/review/{record_id}/edit", intelHandlers.HandleReviewEdit)

			// Section 74 — Remittance Rate Tracker (ADR-075)
			// Feature-flagged via features.Gate (returns 404 when disabled, ADR-042).
			r.With(features.Gate(featureChecker, "remittance.enabled")).Group(func(r chi.Router) {
				r.Get("/remittance/rates", remittanceHandlers.HandleGetRates)
				r.Get("/remittance/calculate", remittanceHandlers.HandleCalculate)
				r.Get("/remittance/history", remittanceHandlers.HandleGetHistory)
				r.Get("/remittance/providers", remittanceHandlers.HandleGetProviders)
				r.Get("/remittance/corridors", remittanceHandlers.HandleGetCorridors)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "remittance.alert")).
					Get("/remittance/alerts", remittanceHandlers.HandleListAlerts)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "remittance.alert")).
					Post("/remittance/alerts", remittanceHandlers.HandleCreateAlert)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "remittance.alert")).
					Patch("/remittance/alerts/{id}", remittanceHandlers.HandleUpdateAlert)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "remittance.alert")).
					Delete("/remittance/alerts/{id}", remittanceHandlers.HandleDeleteAlert)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "remittance.manage")).
					Get("/admin/remittance/providers", remittanceHandlers.HandleAdminListProviders)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "remittance.manage")).
					Post("/admin/remittance/providers", remittanceHandlers.HandleAdminCreateProvider)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "remittance.manage")).
					Patch("/admin/remittance/providers/{id}", remittanceHandlers.HandleAdminUpdateProvider)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "remittance.manage")).
					Post("/admin/remittance/providers/{id}/rates", remittanceHandlers.HandleAdminInsertManualRate)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "remittance.fetch")).
					Post("/admin/remittance/providers/{id}/fetch", remittanceHandlers.HandleAdminTriggerFetch)
			})

			// Phase 49 — V2 Functional Tools (§90–§97, §99–§101)
			// Ref: japaconnect-engineering/sections/90-97,99-101
			// Feature-flagged via features.Gate (returns 404 when disabled, ADR-042).
			// The internal service.Enabled() checks remain as a secondary defense.

			// §90 — Visa Route Comparison (ADR-098)
			r.With(features.Gate(featureChecker, "visa_routes.enabled")).Group(func(r chi.Router) {
				r.Get("/visa-routes", visaRoutesHandlers.HandleList)
				r.Get("/visa-routes/compare", visaRoutesHandlers.HandleCompare)
				r.Get("/visa-routes/{id}", visaRoutesHandlers.HandleGet)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "visa_routes.manage")).
					Get("/admin/visa-routes", visaRoutesHandlers.HandleAdminList)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "visa_routes.manage")).
					Post("/admin/visa-routes", visaRoutesHandlers.HandleAdminCreate)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "visa_routes.manage")).
					Post("/admin/visa-routes/generate", visaRoutesHandlers.HandleAdminGenerate)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "visa_routes.manage")).
					Patch("/admin/visa-routes/{id}", visaRoutesHandlers.HandleAdminUpdate)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "visa_routes.manage")).
					Patch("/admin/visa-routes/{id}/approve", visaRoutesHandlers.HandleAdminApprove)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "visa_routes.manage")).
					Delete("/admin/visa-routes/{id}", visaRoutesHandlers.HandleAdminDelete)
			})

			// §91 — Embassy Appointment Alerter (ADR-099)
			r.With(features.Gate(featureChecker, "appointment_alert.enabled")).Group(func(r chi.Router) {
				r.Get("/embassies", appointmentAlertHandlers.HandleListEmbassies)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "appointment_alert.manage_own")).
					Get("/me/appointment-alerts", appointmentAlertHandlers.HandleListMyAlerts)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "appointment_alert.create")).
					Post("/me/appointment-alerts", appointmentAlertHandlers.HandleCreateMyAlert)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "appointment_alert.manage_own")).
					Patch("/me/appointment-alerts/{id}", appointmentAlertHandlers.HandleUpdateMyAlert)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "appointment_alert.manage_own")).
					Delete("/me/appointment-alerts/{id}", appointmentAlertHandlers.HandleDeleteMyAlert)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "embassy_monitor.manage")).
					Get("/admin/embassies", appointmentAlertHandlers.HandleAdminListEmbassies)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "embassy_monitor.manage")).
					Post("/admin/embassies", appointmentAlertHandlers.HandleAdminCreateEmbassy)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "embassy_monitor.manage")).
					Patch("/admin/embassies/{id}", appointmentAlertHandlers.HandleAdminUpdateEmbassy)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "embassy_monitor.manage")).
					Post("/admin/embassies/{id}/poll", appointmentAlertHandlers.HandleAdminPollEmbassy)
			})

			// §92 — Arrival Checklist (ADR-100)
			r.With(features.Gate(featureChecker, "arrival_checklist.enabled")).Group(func(r chi.Router) {
				r.Get("/arrival-checklist/selector", arrivalChecklistHandlers.HandleSelector)
				r.Get("/arrival-checklist/{guide_id}", arrivalChecklistHandlers.HandleGetGuide)
				// User-facing progress routes (§92 catalog lines 2023-2025)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).
					Get("/me/arrival-checklists", arrivalChecklistHandlers.HandleListMyChecklists)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).
					Get("/me/arrival-checklists/{id}", arrivalChecklistHandlers.HandleGetMyChecklist)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).
					Post("/me/arrival-checklists", arrivalChecklistHandlers.HandleStartChecklist)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).
					Patch("/me/arrival-checklists/{id}/items/{item_id}", arrivalChecklistHandlers.HandleTickItem)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "arrival_checklist.manage")).
					Get("/admin/arrival-checklists", arrivalChecklistHandlers.HandleAdminList)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "arrival_checklist.manage")).
					Post("/admin/arrival-checklists", arrivalChecklistHandlers.HandleAdminCreate)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "arrival_checklist.manage")).
					Post("/admin/arrival-checklists/generate", arrivalChecklistHandlers.HandleAdminGenerate)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "arrival_checklist.manage")).
					Patch("/admin/arrival-checklists/{id}/approve", arrivalChecklistHandlers.HandleAdminApprove)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "arrival_checklist.manage")).
					Patch("/admin/arrival-checklists/{id}", arrivalChecklistHandlers.HandleAdminUpdate)
			})

			// §94 — Professional Credential Checker (ADR-102)
			r.With(features.Gate(featureChecker, "credential_checker.enabled")).Group(func(r chi.Router) {
				r.Get("/credentials/professions", credentialCheckerHandlers.HandleListProfessions)
				r.Get("/credentials", credentialCheckerHandlers.HandleListPathways)
				r.Get("/credentials/{id}", credentialCheckerHandlers.HandleGetPathway)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "credential_checker.note")).
					Post("/credentials/{id}/notes", credentialCheckerHandlers.HandleCreateNote)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "credential_checker.manage")).
					Get("/admin/credentials", credentialCheckerHandlers.HandleAdminList)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "credential_checker.manage")).
					Post("/admin/credentials", credentialCheckerHandlers.HandleAdminCreate)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "credential_checker.manage")).
					Post("/admin/credentials/generate", credentialCheckerHandlers.HandleAdminGenerate)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "credential_checker.manage")).
					Patch("/admin/credentials/{id}", credentialCheckerHandlers.HandleAdminUpdate)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "credential_checker.manage")).
					Patch("/admin/credentials/{id}/approve", credentialCheckerHandlers.HandleAdminApprove)
			})

			// §95 — CV Formatter (ADR-103)
			r.With(features.Gate(featureChecker, "cv_formatter.enabled")).Group(func(r chi.Router) {
				r.Get("/cv-formatter/standards", cvFormatterHandlers.HandleListStandards)
				r.Get("/cv-formatter/standards/{dest}", cvFormatterHandlers.HandleGetStandard)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "cv_formatter.use")).
					Post("/cv-formatter/format", cvFormatterHandlers.HandleFormat)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "cv_formatter.use")).
					Get("/cv-formatter/sessions/{id}", cvFormatterHandlers.HandleGetSession)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "cv_formatter.use")).
					Get("/cv-formatter/sessions/{id}/download", cvFormatterHandlers.HandleDownload)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "cv_formatter.manage")).
					Get("/admin/cv-formatter/standards", cvFormatterHandlers.HandleAdminListStandards)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "cv_formatter.manage")).
					Post("/admin/cv-formatter/standards", cvFormatterHandlers.HandleAdminUpsertStandard)
			})

			// §96 — Housing Guide (ADR-104)
			r.With(features.Gate(featureChecker, "housing_guide.enabled")).Group(func(r chi.Router) {
				r.Get("/housing-guide", housingGuideHandlers.HandleList)
				r.Get("/housing-guide/{id}", housingGuideHandlers.HandleGet)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "housing_guide.manage")).
					Get("/admin/housing-guides", housingGuideHandlers.HandleAdminList)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "housing_guide.manage")).
					Post("/admin/housing-guides", housingGuideHandlers.HandleAdminCreate)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "housing_guide.manage")).
					Post("/admin/housing-guides/generate", housingGuideHandlers.HandleAdminGenerate)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "housing_guide.manage")).
					Patch("/admin/housing-guides/{id}", housingGuideHandlers.HandleAdminUpdate)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "housing_guide.manage")).
					Patch("/admin/housing-guides/{id}/approve", housingGuideHandlers.HandleAdminApprove)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "housing_guide.manage")).
					Patch("/admin/housing-guides/{id}/reject", housingGuideHandlers.HandleAdminReject)
			})

			// §97 — Attestation Guide (ADR-105)
			r.With(features.Gate(featureChecker, "attestation_guide.enabled")).Group(func(r chi.Router) {
				r.Get("/attestation", attestationGuideHandlers.HandleList)
				r.Get("/attestation/{id}", attestationGuideHandlers.HandleGet)
				r.Get("/attestation/{id}/export/pdf", attestationGuideHandlers.HandleExportPDF)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "attestation_guide.update")).
					Post("/attestation/{id}/updates", attestationGuideHandlers.HandleSubmitUpdate)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "attestation_guide.manage")).
					Get("/admin/attestation", attestationGuideHandlers.HandleAdminList)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "attestation_guide.manage")).
					Get("/admin/attestation/updates", attestationGuideHandlers.HandleAdminListUpdates)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "attestation_guide.manage")).
					Post("/admin/attestation", attestationGuideHandlers.HandleAdminCreate)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "attestation_guide.manage")).
					Post("/admin/attestation/generate", attestationGuideHandlers.HandleAdminGenerate)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "attestation_guide.manage")).
					Patch("/admin/attestation/{id}/approve", attestationGuideHandlers.HandleAdminApprove)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "attestation_guide.manage")).
					Patch("/admin/attestation/updates/{id}", attestationGuideHandlers.HandleAdminReviewUpdate)
			})

			// §99 — Language Test Hub (ADR-107)
			r.With(features.Gate(featureChecker, "language_test_hub.enabled")).Group(func(r chi.Router) {
				r.Get("/language-tests", languageTestHubHandlers.HandleListTests)
				r.Get("/language-tests/requirements", languageTestHubHandlers.HandleListRequirements)
				r.Get("/language-tests/{test_slug}", languageTestHubHandlers.HandleGetTest)
				r.Get("/language-tests/{test_slug}/resources", languageTestHubHandlers.HandleListResources)
				r.Get("/language-tests/{test_slug}/stories", languageTestHubHandlers.HandleListStories)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "moderate"), rbac.RequirePermission(rbacResolver, "language_test_hub.story")).
					Post("/language-tests/{test_slug}/stories", languageTestHubHandlers.HandleCreateStory)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "language_test_hub.manage")).
					Get("/admin/language-tests/requirements", languageTestHubHandlers.HandleAdminListRequirements)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "language_test_hub.manage")).
					Post("/admin/language-tests/requirements", languageTestHubHandlers.HandleAdminCreateRequirement)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "language_test_hub.manage")).
					Post("/admin/language-tests/requirements/generate", languageTestHubHandlers.HandleAdminGenerateRequirement)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "language_test_hub.manage")).
					Patch("/admin/language-tests/requirements/{id}/approve", languageTestHubHandlers.HandleAdminApproveRequirement)
			})

			// §100 — Tax Calculator (ADR-108)
			r.With(features.Gate(featureChecker, "tax_calculator.enabled")).Group(func(r chi.Router) {
				r.Get("/tax-calculator", taxCalculatorHandlers.HandleList)
				r.Get("/tax-calculator/treaty", taxCalculatorHandlers.HandleTreaty)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tax_calculator.manage")).
					Get("/admin/tax-calculator", taxCalculatorHandlers.HandleAdminList)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tax_calculator.manage")).
					Post("/admin/tax-calculator/guides", taxCalculatorHandlers.HandleCreateGuide)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tax_calculator.manage")).
					Post("/admin/tax-calculator/guides/generate", taxCalculatorHandlers.HandleGenerateGuide)
				// BUG-005: ADR-108 requires the HITL approve endpoint to
				// check tax_content.review (qualified reviewer), not
				// tax_calculator.manage. A user with manage but not
				// review must not be able to publish tax content.
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tax_content.review")).
					Patch("/admin/tax-calculator/guides/{id}/approve", taxCalculatorHandlers.HandleApproveGuide)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tax_calculator.manage")).
					Post("/admin/tax-calculator/treaties", taxCalculatorHandlers.HandleUpsertTreaty)
			})

			// §101 — Diaspora Community Locator (ADR-109)
			r.With(features.Gate(featureChecker, "community_locator.enabled")).Group(func(r chi.Router) {
				r.Get("/community-locator", communityLocatorHandlers.HandleList)
				r.Get("/community-locator/{slug}", communityLocatorHandlers.HandleGet)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "community_locator.submit")).
					Post("/community-locator", communityLocatorHandlers.HandleSubmit)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "community_locator.claim")).
					Post("/community-locator/{id}/claim", communityLocatorHandlers.HandleClaim)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "community_locator.review")).
					Get("/admin/community-locator", communityLocatorHandlers.HandleAdminList)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "community_locator.review")).
					Patch("/admin/community-locator/{id}/approve", communityLocatorHandlers.HandleAdminApprove)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "community_locator.manage")).
					Patch("/admin/community-locator/{id}/verify", communityLocatorHandlers.HandleAdminVerify)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "community_locator.manage")).
					Patch("/admin/community-locator/{id}/claim/{claim_id}", communityLocatorHandlers.HandleAdminResolveClaim)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "community_locator.manage")).
					Post("/admin/community-locator/generate", communityLocatorHandlers.HandleAdminGenerate)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "community_locator.manage")).
					Patch("/admin/community-locator/{id}/archive", communityLocatorHandlers.HandleAdminArchive)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "community_locator.manage")).
					Delete("/admin/community-locator/{id}", communityLocatorHandlers.HandleAdminDelete)
			})

			// Section 61 — Self-Serve Advertising (ADR-079)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ads.self_serve.create")).
				Get("/org/{org_id}/ads/slots", adsSelfServeHandlers.HandleListSlots)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ads.self_serve.create")).
				Post("/org/{org_id}/ads/campaigns", adsSelfServeHandlers.HandleCreateCampaign)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ads.self_serve.manage")).
				Get("/org/{org_id}/ads/campaigns", adsSelfServeHandlers.HandleListCampaigns)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ads.self_serve.manage")).
				Get("/org/{org_id}/ads/campaigns/{id}", adsSelfServeHandlers.HandleGetCampaign)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ads.self_serve.manage")).
				Patch("/org/{org_id}/ads/campaigns/{id}", adsSelfServeHandlers.HandleUpdateCampaign)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ads.self_serve.manage")).
				Post("/org/{org_id}/ads/campaigns/{id}/pause", adsSelfServeHandlers.HandlePauseCampaign)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ads.self_serve.manage")).
				Post("/org/{org_id}/ads/campaigns/{id}/resume", adsSelfServeHandlers.HandleResumeCampaign)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ads.self_serve.stats")).
				Get("/org/{org_id}/ads/campaigns/{id}/stats", adsSelfServeHandlers.HandleGetStats)

			// Section 77 — Mobile Push Devices (ADR-074)
			// Device token registration for FCM/APNs push delivery.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "push.receive")).
				Post("/me/devices", devicesHandlers.HandleRegisterDevice)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "push.receive")).
				Delete("/me/devices/{device_id}", devicesHandlers.HandleDeregisterDevice)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "push.receive")).
				Get("/me/devices", devicesHandlers.HandleListDevices)
			// Admin push test (ADR-074).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "push.manage")).
				Post("/admin/push/test", devicesHandlers.HandleAdminTestPush)

			// Newsletter public routes (Section 62 — ADR-080).
			// Unsubscribe via token (no auth) + subscriber preferences.
			r.Delete("/newsletter/unsubscribe", newsletterHandlers.HandleUnsubscribe)
			// Provider bounce/complaint webhook (§62 Compliance, ADR-080 §4).
			// Authenticated via X-Webhook-Secret shared secret, not user auth.
			r.Post("/newsletter/webhook", newsletterHandlers.HandleWebhook)
			// Subscriber preferences — gated by newsletter.subscribe (§62 RBAC,
			// ADR-080 §6: "subscribe/unsubscribe own email").
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "newsletter.subscribe")).
				Get("/newsletter/preferences", newsletterHandlers.HandleGetPreferences)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "newsletter.subscribe")).
				Patch("/newsletter/preferences", newsletterHandlers.HandleUpdatePreferences)
			// Admin newsletter stats (Section 62).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "newsletter.manage")).
				Get("/admin/newsletter/stats", newsletterHandlers.HandleAdminStats)

			// Section 60 — Referral System (ADR-078) public + me routes.
			// Admin referral/program routes are wired above (referral.manage).
			// /ref/{code} is public; click tracking enqueues referral:record_click.
			// NOTE: /r/{code} is already used by the affiliate redirect (§56),
			// so the referral system uses /ref/{code} to avoid a route collision.
			// The JEOS catalog (§60) lists /r/:code; this is a documented
			// deviation — see the comment in .jeos/sections/60-referral-system.md.
			r.Get("/ref/{code}", referralHandlers.HandleRedirect)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "referral.use")).
				Get("/me/referral", referralHandlers.HandleGetMyCode)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "referral.use")).
				Get("/me/referral/history", referralHandlers.HandleMyHistory)
			// V2 admin referral moderation routes (ADR-078).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "referral.manage")).
				Get("/admin/referrals", referralHandlers.HandleAdminListReferrals)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "referral.manage")).
				Get("/admin/referrals/stats", referralHandlers.HandleAdminReferralStats)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "referral.manage")).
				Get("/admin/referrals/flagged", referralHandlers.HandleAdminFlagged)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "referral.manage")).
				Patch("/admin/referrals/{id}/resolve", referralHandlers.HandleAdminResolveFlagged)
		})
	})

	// Payment webhook routes — inside /api/v1 with a tighter rate limit.
	// Payment providers (Paystack, Stripe, NOWPayments) can send
	// high-frequency webhook traffic.
	// NOTE: These are registered inside the existing /api/v1 group above.

	// Super-admin Control Panel routes (Section 21).
	// Tenant-scoped, requires auth + control.panel.access permission.
	r.Group(func(r chi.Router) {
		r.Use(tenant.SingleTenantMiddleware(defaultTenant))
		r.Use(auth.RequireAuth(kr, vkClient.Redis()))
		r.Use(rbac.RequirePermission(rbacResolver, "control.panel.access"))
		// Audit finding C1: enforce double-submit CSRF on all mutating /control/* requests.
		// NOTE: the /control group is mounted at the TOP LEVEL (r.Route("/control", ...)
		// outside the /api/v1 group), so real request paths are "/control/...", NOT
		// "/api/v1/control/...". The prefix MUST be "/control/" or protection is silently
		// never applied.
		// WithOriginCheck adds defense-in-depth Origin validation (see /api/v1 group).
		r.Use(csrf.Middleware([]string{"/control/"}, csrf.IsNonCookieAuthenticated, csrf.WithOriginCheck(mw.IsOriginAllowed)))
		// Audit finding M1: (re)issue the csrf_token cookie for control-panel
		// requests so the SPA always has a valid double-submit token available.
		r.Use(csrf.EnsureCookie(cfg.CookieDomain, cfg.IsProduction()))

		r.Route("/control", func(r chi.Router) {
			// System Settings
			r.Get("/system-settings", controlHandlers.ListSystemSettings)
			r.Patch("/system-settings", controlHandlers.UpdateSystemSettings)
			r.Patch("/system-settings/{key}", controlHandlers.UpdateSystemSetting)
			r.Post("/system-settings/{key}/reset", controlHandlers.ResetSystemSetting)

			// Infrastructure Health
			r.Get("/health", controlHandlers.HealthCheck)
			// Ref: japaconnect-engineering/sections/21-control-panel.md (Database Health Tooling)
			// DB health and vacuum routes require dedicated permission nodes separate
			// from control.panel.access so access can be delegated narrowly.
			r.With(rbac.RequirePermission(rbacResolver, "platform.db.health_view")).
				Get("/health/db", controlHandlers.DBHealthCheck)
			r.With(rbac.RequirePermission(rbacResolver, "platform.db.vacuum_trigger")).
				Post("/health/db/vacuum", controlHandlers.DBVacuum)

			// Queue Monitor
			r.Get("/queue/stats", controlHandlers.QueueStats)
			r.Get("/queue/{queue_name}/tasks", controlHandlers.ListQueueTasks)
			r.Post("/queue/{queue_name}/tasks/{task_id}/retry", controlHandlers.RetryQueueTask)
			r.Get("/queue/dlq", controlHandlers.ListDLQ)
			r.Post("/queue/dlq/{task_id}/retry", controlHandlers.RetryDLQTask)
			r.Delete("/queue/dlq/{task_id}", controlHandlers.DiscardDLQTask)
			r.Post("/queue/dlq/discard-all", controlHandlers.DiscardAllDLQ)

			// Cron / Scheduler jobs (audit C3)
			r.Get("/cron/jobs", controlHandlers.ListCronJobs)
			r.Post("/cron/jobs", controlHandlers.CreateCronJob)
			r.Patch("/cron/jobs/{id}", controlHandlers.UpdateCronJob)
			r.Delete("/cron/jobs/{id}", controlHandlers.DeleteCronJob)
			r.Post("/cron/jobs/{id}/trigger", controlHandlers.TriggerCronJob)
			r.Post("/cron/jobs/{id}/pause", controlHandlers.PauseCronJob)
			r.Post("/cron/jobs/{id}/unpause", controlHandlers.UnpauseCronJob)

			// Cache Management
			r.Get("/cache/stats", controlHandlers.CacheStats)
			r.Post("/cache/flush", controlHandlers.FlushCache)

			// Security
			r.Get("/security/scan", controlHandlers.ListSecurityScanResults)
			r.Post("/security/scan", controlHandlers.TriggerSecurityScan)
			r.Get("/security", controlHandlers.SecurityOverview)
			r.Get("/security/ip-blocks", controlHandlers.ListIPBlocks)
			r.Post("/security/ip-blocks", controlHandlers.CreateIPBlock)
			r.Delete("/security/ip-blocks/{id}", controlHandlers.DeleteIPBlock)
			r.Post("/security/{id}/resolve", analyticsHandlers.ResolveSecurityScanResult)

			// API Keys (moved from /admin/api-keys)
			// Granular RBAC per Section 47 lines 233–238 (PUB-007):
			//   api.keys.view.all     — list/get any key
			//   api.keys.manage       — create/suspend/revoke/delete/update tier
			//   api.keys.analytics.view — usage analytics
			r.With(rbac.RequirePermission(rbacResolver, "api.keys.view.all")).
				Get("/api-keys", publicAPIHandlers.ListAllKeys)
			r.With(rbac.RequirePermission(rbacResolver, "api.keys.manage")).
				Post("/api-keys", controlHandlers.CreateAPIKey)
			r.With(rbac.RequirePermission(rbacResolver, "api.keys.manage")).
				Post("/api-keys/batch-delete", controlHandlers.DeleteAPIKeysBatch)
			r.With(rbac.RequirePermission(rbacResolver, "api.keys.view.all")).
				Get("/api-keys/{id}", controlHandlers.GetAPIKey)
			r.With(rbac.RequirePermission(rbacResolver, "api.keys.manage")).
				Patch("/api-keys/{id}", controlHandlers.UpdateAPIKey)
			r.With(rbac.RequirePermission(rbacResolver, "api.keys.manage")).
				Post("/api-keys/{id}/revoke", publicAPIHandlers.RevokeKey)
			r.With(rbac.RequirePermission(rbacResolver, "api.keys.manage")).
				Delete("/api-keys/{id}", controlHandlers.DeleteAPIKey)
			r.With(rbac.RequirePermission(rbacResolver, "api.keys.manage")).
				Post("/api-keys/{id}/suspend", publicAPIHandlers.SuspendKey)
			r.With(rbac.RequirePermission(rbacResolver, "api.keys.manage")).
				Delete("/api-keys/{id}/suspend", publicAPIHandlers.UnsuspendKey)
			r.With(rbac.RequirePermission(rbacResolver, "api.keys.analytics.view")).
				Get("/api-keys/analytics", controlHandlers.GetAPIKeyAnalytics)

			// AI Budget — granular RBAC (BUG-007: previously only control.panel.access).
			// ai.budget.view gates the dashboard; ai.budget.manage gates the threshold update.
			r.With(rbac.RequirePermission(rbacResolver, "ai.budget.view")).
				Get("/ai-budget", controlHandlers.GetAIBudget)
			r.With(rbac.RequirePermission(rbacResolver, "ai.budget.manage")).
				Patch("/ai-budget", controlHandlers.UpdateAIBudget)

			// AI Providers — granular RBAC (BUG-007: previously only control.panel.access).
			// ai.providers.view gates list/get; ai.providers.manage gates create/update/delete/test.
			// Super-admin CRUD for runtime-managed AI provider configs (migration 000269).
			// Changes hot-reload the dispatcher.
			r.With(rbac.RequirePermission(rbacResolver, "ai.providers.view")).
				Get("/ai-providers", controlHandlers.ListAIProviders)
			r.With(rbac.RequirePermission(rbacResolver, "ai.providers.view")).
				Get("/ai-providers/types", controlHandlers.GetAIProviderTypes)
			r.With(rbac.RequirePermission(rbacResolver, "ai.providers.manage")).
				Post("/ai-providers", controlHandlers.CreateAIProvider)
			r.With(rbac.RequirePermission(rbacResolver, "ai.providers.manage")).
				Post("/ai-providers/fetch-models", controlHandlers.FetchAIModels)
			r.With(rbac.RequirePermission(rbacResolver, "ai.providers.view")).
				Get("/ai-providers/{id}", controlHandlers.GetAIProvider)
			r.With(rbac.RequirePermission(rbacResolver, "ai.providers.manage")).
				Patch("/ai-providers/{id}", controlHandlers.UpdateAIProvider)
			r.With(rbac.RequirePermission(rbacResolver, "ai.providers.manage")).
				Delete("/ai-providers/{id}", controlHandlers.DeleteAIProvider)
			r.With(rbac.RequirePermission(rbacResolver, "ai.providers.manage")).
				Post("/ai-providers/{id}/test", controlHandlers.TestAIProvider)

			// Audit Log (full cross-tenant view)
			r.Get("/audit-logs", controlHandlers.ListAuditLogs)
			r.Delete("/audit-logs/{id}", controlHandlers.DeleteAuditLog)
			r.Post("/audit-logs/clear", controlHandlers.ClearAuditLogs)

			// Task types catalog (drives cron job editor type dropdown)
			r.Get("/task-types", controlHandlers.ListTaskTypes)

			// Schedule presets + tier names (drives cron/backup-configs UI dropdowns)
			r.Get("/cron/schedule-presets", controlHandlers.ListSchedulePresets)

			// API scopes catalog (drives API key scope editor dropdown)
			r.Get("/api-scopes", rbacHandlers.ListScopes)

			// Deployments
			r.Get("/deployments", controlHandlers.ListDeployments)

			// Permission Nodes — list all registered permission nodes
			r.Get("/permission-nodes", controlHandlers.ListPermissionNodes)

			// Webhook Endpoints — super-admin view (wires existing webhook handlers).
			// The /control group is gated by control.panel.access (super-admin
			// only); integrations.webhooks.manage is implicitly satisfied for
			// super-admins and is not re-checked here (BUG-013: documented as
			// intentional — control panel access supersedes per-feature RBAC).
			r.Get("/webhooks", webhookHandlers.ListWebhooks)
			r.Get("/webhooks/{id}", webhookHandlers.GetWebhook)
			r.Post("/webhooks", webhookHandlers.CreateWebhook)
			r.Patch("/webhooks/{id}", webhookHandlers.UpdateWebhook)
			r.Delete("/webhooks/{id}", webhookHandlers.DeleteWebhook)
			r.Post("/webhooks/{id}/test", webhookHandlers.TestWebhook)
			r.Post("/webhooks/{id}/verify", webhookHandlers.VerifyWebhook)
			r.Get("/webhooks/{id}/deliveries", webhookHandlers.ListDeliveries)
			r.Post("/webhooks/{id}/deliveries/{delivery_id}/retry", webhookHandlers.RetryDelivery)
			r.Get("/webhook-events", webhookHandlers.ListWebhookEvents)

			// OAuth Clients — super-admin CRUD
			r.Get("/oauth-clients", controlHandlers.ListOAuthClients)
			r.Get("/oauth-clients/{id}", controlHandlers.GetOAuthClient)
			r.Post("/oauth-clients", controlHandlers.CreateOAuthClient)
			r.Patch("/oauth-clients/{id}", controlHandlers.UpdateOAuthClient)
			r.Delete("/oauth-clients/{id}", controlHandlers.DeleteOAuthClient)
			r.Post("/oauth-clients/{id}/rotate-secret", controlHandlers.RotateOAuthClientSecret)
			r.Get("/oauth-providers", controlHandlers.ListOAuthProviders)

			// Extensions — super-admin CRUD (feature_flags is the single source of truth)
			r.Get("/extensions", controlHandlers.ListExtensions)
			r.Patch("/extensions/{key}", controlHandlers.UpdateExtension)
			r.Post("/extensions/{key}/enable", controlHandlers.EnableExtension)
			r.Post("/extensions/{key}/disable", controlHandlers.DisableExtension)
			r.Patch("/extensions/{key}/rollout", controlHandlers.UpdateExtensionRollout)

			// Backup Configs — super-admin CRUD
			r.Get("/backup-configs", controlHandlers.ListBackupConfigs)
			r.Post("/backup-configs", controlHandlers.CreateBackupConfig)
			r.Patch("/backup-configs/{id}", controlHandlers.UpdateBackupConfig)
			r.Delete("/backup-configs/{id}", controlHandlers.DeleteBackupConfig)
			r.Post("/backup-configs/{id}/trigger", controlHandlers.TriggerBackup)
			r.Get("/backup-targets", controlHandlers.ListBackupTargets)

			// Wallet Currencies — super_admin CRUD
			r.With(rbac.RequirePermission(rbacResolver, "wallet.currency.manage")).
				Get("/wallet/currencies", walletHandlers.ControlListCurrencies)
			r.With(rbac.RequirePermission(rbacResolver, "wallet.currency.manage")).
				Post("/wallet/currencies", walletHandlers.ControlCreateCurrency)
			r.With(rbac.RequirePermission(rbacResolver, "wallet.currency.manage")).
				Patch("/wallet/currencies/{code}", walletHandlers.ControlUpdateCurrency)
			r.With(rbac.RequirePermission(rbacResolver, "wallet.currency.manage")).
				Delete("/wallet/currencies/{code}", walletHandlers.ControlDeleteCurrency)
		})
	})

	return r
}

// ---------------------------------------------------------------------------
// Adapter: wallet.Service → payments.CreditSpender
// ---------------------------------------------------------------------------

// walletCreditAdapter adapts wallet.Service to implement payments.CreditSpender.
type walletCreditAdapter struct {
	svc *wallet.Service
}

func (a *walletCreditAdapter) SpendCredits(
	ctx context.Context,
	tenantID pgtype.UUID,
	userID pgtype.UUID,
	input payments.CreditSpendInput,
) (int64, error) {
	return a.svc.SpendCredits(ctx, tenantID, userID, wallet.SpendCreditsInput{
		Amount:      input.Amount,
		Source:      input.Source,
		Reference:   input.Reference,
		PurposeType: input.PurposeType,
		PurposeID:   input.PurposeID,
	})
}

// walletPaymentAdapter adapts payments.Service to implement wallet.PaymentCreator.
// This lets the wallet create credit-purchase payment rows via the Section 17
// payment provider abstraction without importing the payments package.
type walletPaymentAdapter struct {
	svc *payments.Service
}

func (a *walletPaymentAdapter) CreatePayment(
	ctx context.Context,
	tenantID, userID pgtype.UUID,
	input wallet.PaymentCreateInput,
) (wallet.PaymentCreateResult, error) {
	createResult, err := a.svc.CreatePayment(ctx, tenantID, userID, payments.CreatePaymentInput{
		Provider:     input.Provider,
		PurposeType:  input.PurposeType,
		PurposeID:    input.PurposeID,
		Amount:       input.Amount,
		Currency:     input.Currency,
		Metadata:     input.Metadata,
		DiscountCode: input.DiscountCode,
	})
	if err != nil {
		return wallet.PaymentCreateResult{}, err
	}
	payment := createResult.Payment
	result := wallet.PaymentCreateResult{
		ID:          payment.ID.String(),
		Provider:    payment.Provider,
		Status:      payment.Status,
		Amount:      payment.Amount,
		Currency:    payment.Currency,
		CheckoutURL: createResult.CheckoutURL,
	}
	if payment.ProviderRef != nil {
		result.ProviderRef = *payment.ProviderRef
	}
	return result, nil
}

// commentNotificationAdapter adapts notifications.Service.DispatchNotification
// (returns DispatchResult) to the comments.NotificationDispatcher and
// content.MentionNotifier interfaces (both return only error). The
// DispatchResult is discarded — comment/mention notifications are
// fire-and-forget per Rule 8.
type commentNotificationAdapter struct {
	svc *notifications.Service
}

// gamificationNotifierAdapter adapts notifications.Service.CreateNotification
// to the gamification.Notifier interface (BUG-GAMMA-003, ADR-082 §2).
// Badge-award and challenge-completion notifications are fire-and-forget
// per Rule 8. The actor is the system (zero UUID) and the subject is the
// badge/challenge itself.
type gamificationNotifierAdapter struct {
	svc *notifications.Service
}

func (a gamificationNotifierAdapter) Notify(ctx context.Context, tenantID, recipientID pgtype.UUID, nType, title, body, actionURL string) error {
	var zeroUUID pgtype.UUID
	return a.svc.CreateNotification(ctx, tenantID, recipientID, nType, "system", zeroUUID, nType, zeroUUID, title, body, actionURL)
}

// adsNotificationAdapter adapts notifications.Service.DispatchNotification to
// the adsselfserve.Notifier interface. Campaign status notifications
// (approved/rejected) are fire-and-forget per Rule 8.
// Ref: japaconnect-engineering/sections/61-self-serve-ads.md (Asynq Tasks)
type adsNotificationAdapter struct {
	svc *notifications.Service
}

func (a adsNotificationAdapter) DispatchNotification(
	ctx context.Context,
	tenantID, recipientID pgtype.UUID,
	nType, subjectTitle string,
	actorID pgtype.UUID,
	subjectType string,
	subjectID pgtype.UUID,
	subjectURL, actorName, actorAvatar string,
) error {
	_, err := a.svc.DispatchNotification(ctx, tenantID, recipientID, nType, subjectTitle,
		actorID, subjectType, subjectID, subjectURL, actorName, actorAvatar)
	return err
}

// adsBalanceChecker adapts wallet.Service.GetBalance to the
// adsselfserve.BalanceChecker interface. It resolves the org owner's user_id
// and checks whether their credits balance covers the campaign cost.
// Ref: japaconnect-engineering/sections/61-self-serve-ads.md (Payment & Escrow)
type adsBalanceChecker struct {
	wallet *wallet.Service
	db     *db.DB
}

func (c adsBalanceChecker) HasSufficientCredits(ctx context.Context, tenantID, orgID pgtype.UUID, amount int) (bool, error) {
	queries := gen.New(c.db.Reader)
	org, err := queries.GetOrganizationByIDOnly(ctx, orgID)
	if err != nil {
		return false, fmt.Errorf("ads balance check: resolving org owner: %w", err)
	}
	if !org.OwnerUserID.Valid {
		return false, fmt.Errorf("ads balance check: organization %s has no owner", orgID.String())
	}
	_, credits, err := c.wallet.GetBalance(ctx, tenantID, org.OwnerUserID)
	if err != nil {
		return false, fmt.Errorf("ads balance check: getting balance: %w", err)
	}
	return credits >= int64(amount), nil
}

// tagSlugResolverAdapter adapts tags.Service.ResolveTagSlugMap to the
// content.TagSlugResolver interface, avoiding an import cycle (tags imports
// content, so content cannot import tags directly).
// Ref: japaconnect-engineering/sections/14-content-type-system.md (bulk tag_slugs)
type tagSlugResolverAdapter struct {
	svc *tags.Service
}

func (a tagSlugResolverAdapter) ResolveTagSlugs(ctx context.Context, tenantID pgtype.UUID, slugs []string) (map[string]pgtype.UUID, error) {
	return a.svc.ResolveTagSlugMap(ctx, tenantID, slugs)
}

func (a tagSlugResolverAdapter) GetOrCreateTagSlugs(ctx context.Context, tenantID pgtype.UUID, slugs []string) (map[string]pgtype.UUID, error) {
	return a.svc.GetOrCreateTagSlugs(ctx, tenantID, slugs)
}

// guideCategoryListerAdapter adapts customfields.Service.ListGuideCategories
// to the content.GuideCategoryLister interface, avoiding an import cycle
// (customfields cannot import content).
type guideCategoryListerAdapter struct {
	svc *customfields.Service
}

func (a guideCategoryListerAdapter) ListGuideCategories(ctx context.Context, tenantID pgtype.UUID) ([]content.GuideCategory, error) {
	cats, err := a.svc.ListGuideCategories(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]content.GuideCategory, 0, len(cats))
	for _, c := range cats {
		out = append(out, content.GuideCategory{Slug: c.Slug, Label: c.Label})
	}
	return out, nil
}

// followNotificationAdapter adapts notifications.Service.DispatchNotification
// to the follows.NotificationDispatcher interface. Follow notifications
// (type follow.received, Section 35) are fire-and-forget per Rule 8 — the
// adapter discards both the DispatchResult and any error.
// Ref: japaconnect-engineering/sections/31-follow-system.md
//
//	japaconnect-engineering/sections/35-notification-system.md
type followNotificationAdapter struct {
	svc *notifications.Service
}

func (a followNotificationAdapter) DispatchFollowNotification(
	ctx context.Context,
	tenantID, recipientID, actorID pgtype.UUID,
	entityType string, entityID pgtype.UUID,
) {
	_, _ = a.svc.DispatchNotification(
		ctx, tenantID, recipientID,
		notifications.TypeNewFollower, "",
		actorID,
		entityType, entityID,
		"", "", "",
	)
}

func (a commentNotificationAdapter) DispatchNotification(
	ctx context.Context,
	tenantID, recipientID pgtype.UUID,
	nType, subjectTitle string,
	actorID pgtype.UUID,
	subjectType string,
	subjectID pgtype.UUID,
	subjectURL, actorName, actorAvatar string,
) error {
	_, err := a.svc.DispatchNotification(ctx, tenantID, recipientID, nType, subjectTitle,
		actorID, subjectType, subjectID, subjectURL, actorName, actorAvatar)
	return err
}

// forumPermCacheInvalidator adapts rbac.InvalidatePermCache to the forum
// service's permCacheInvalidator interface. Ref: Rule 6.
type forumPermCacheInvalidator struct {
	vk valkeycompat.Cmdable
}

func (f *forumPermCacheInvalidator) InvalidatePermCache(ctx context.Context, userID pgtype.UUID) error {
	return rbac.InvalidatePermCache(ctx, f.vk, userID)
}

// forumWebhookEmitter adapts webhooks.Service.EmitEvent to the forum
// service's webhookEmitter interface. Ref: Section 12 + Rule 8.
type forumWebhookEmitter struct {
	svc *webhooks.Service
}

func (f *forumWebhookEmitter) EmitEvent(ctx context.Context, tenantID pgtype.UUID, eventType string, payload any) {
	f.svc.EmitEvent(ctx, tenantID, eventType, payload)
}

// forumSpaceBroadcaster adapts websocket.Hub.BroadcastToSpace to the forum
// service's spaceBroadcaster interface. The wsEvent shape is compatible via
// JSON marshaling. Ref: Section 10.
type forumSpaceBroadcaster struct {
	hub *websocket.Hub
}

func (f *forumSpaceBroadcaster) BroadcastToSpace(tenantID, spaceID string, event forum.WSEvent) {
	f.hub.BroadcastToSpace(tenantID, spaceID, websocket.WSEvent{Type: event.Type, Payload: event.Payload})
}

// mediaURLResolver adapts media.Service to the users.MediaResolver
// interface so PATCH /me/profile can resolve avatar_media_id and
// header_media_id to presigned URLs (BUG-004).
// Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md
type mediaURLResolver struct {
	media *media.Service
}

func (r *mediaURLResolver) ResolveMediaURL(ctx context.Context, tenantID, mediaID pgtype.UUID) (string, error) {
	m, err := r.media.GetMedia(ctx, tenantID, mediaID)
	if err != nil {
		return "", err
	}
	return r.media.GetMediaURL(&m, media.ImageProxyOpts{}), nil
}

// walletCacheInvalidator adapts the Valkey client to the wallet.Service's
// CacheInvalidator interface. It deletes the jc:wallet:currencies:{tenant_id}
// key after any currency CRUD operation (Section 58, Rule 4).
type walletCacheInvalidator struct {
	vk valkeycompat.Cmdable
}

func (w *walletCacheInvalidator) InvalidateCurrenciesCache(ctx context.Context, tenantID pgtype.UUID) error {
	key := valkey.MustKey("wallet:currencies:%s", tenantID.String())
	return w.vk.Del(ctx, key).Err()
}

// achievementActivityEmitter adapts *wallet.Service to the
// achievement.ActivityEmitter interface so the achievement service can fire
// achievement.created activity events for gamification badge counting
// (§64/§89) without importing wallet directly (import cycle).
type achievementActivityEmitter struct {
	svc *wallet.Service
}

func (a *achievementActivityEmitter) EmitActivityEvent(ctx context.Context, tenantID, userID pgtype.UUID, input achievement.ActivityEventInput) (achievement.ActivityEvent, error) {
	event, err := a.svc.EmitActivityEvent(ctx, tenantID, userID, wallet.EmitActivityEventInput{
		ActivityKey: input.ActivityKey,
		Reference:   input.Reference,
		Payload:     input.Payload,
	})
	if err != nil {
		return achievement.ActivityEvent{}, err
	}
	return achievement.ActivityEvent{ID: event.ID}, nil
}

func (a *achievementActivityEmitter) EnqueueProcessActivityEvent(ctx context.Context, eventID, tenantID pgtype.UUID) error {
	return a.svc.EnqueueProcessActivityEvent(ctx, eventID, tenantID)
}

// StartWorker creates an Asynq worker server from the provided config,
// registers all task handlers, and starts processing in a background goroutine.
// The returned shutdown function must be called during application shutdown to
// gracefully drain in-flight tasks and close the Valkey connection.
func StartWorker(cfg *worker.Config) (shutdown func(), err error) {
	srv := worker.NewServer(cfg)
	mux := worker.NewMux()

	go func() {
		slog.Info("asynq worker starting", "addr", worker.Addr(cfg))

		if err := srv.Run(mux); err != nil {
			slog.Error("asynq worker stopped", "error", err)
		}
	}()

	return func() {
		slog.Info("asynq worker shutting down")
		srv.Shutdown()
	}, nil
}

// lookupSystemUserID resolves the super admin user ID (seeded in migration
// 000124 by email jedafeglobal@gmail.com) for media attribution when
// auto-populating university logos. Returns the user's UUID.
func lookupSystemUserID(ctx context.Context, database *db.DB) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := database.Reader.QueryRow(ctx, `
		SELECT id FROM users
		WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
		  AND email = 'jedafeglobal@gmail.com'
		LIMIT 1
	`).Scan(&id)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("looking up super admin user: %w", err)
	}
	return id, nil
}

// ── Assessment AI/RAG adapters ───────────────────────────────────────────
// These adapters bridge the concrete ai.Dispatcher and rag.Service to the
// assessment service's minimal AIDispatcher and RAGRetriever interfaces.
// Defined here (not in internal/assessment) to avoid an import cycle and
// keep the assessment package decoupled from concrete ai/rag types.
// Ref: ADR-071 §3 — AI-assisted roadmap generation.

// assessmentAIDispatcher adapts *ai.Dispatcher to assessment.AIDispatcher.
type assessmentAIDispatcher struct {
	dispatcher *ai.Dispatcher
}

func (a *assessmentAIDispatcher) Dispatch(ctx context.Context, category string, req assessment.AIRequest) (assessment.AIResponse, error) {
	resp, err := a.dispatcher.Dispatch(ctx, category, ai.Request{
		Category: category,
		Payload:  req.Payload,
	})
	if err != nil {
		return assessment.AIResponse{}, err
	}
	return assessment.AIResponse{
		ProviderRef: resp.ProviderRef,
		Status:      resp.Status,
		Raw:         resp.Raw,
	}, nil
}

func (a *assessmentAIDispatcher) HasCategory(category string) bool {
	return a.dispatcher.HasCategory(category)
}

// assessmentRAGRetriever adapts *rag.Service to assessment.RAGRetriever.
type assessmentRAGRetriever struct {
	svc *rag.Service
}

func (r *assessmentRAGRetriever) Retrieve(ctx context.Context, tenantID pgtype.UUID, query, language string, geoSpaceID *pgtype.UUID) ([]assessment.RAGChunk, error) {
	chunks, err := r.svc.Retrieve(ctx, tenantID, query, language, geoSpaceID)
	if err != nil {
		return nil, err
	}
	result := make([]assessment.RAGChunk, 0, len(chunks))
	for _, c := range chunks {
		result = append(result, assessment.RAGChunk{
			Content:   c.ChunkText,
			Score:     c.Similarity,
			Source:    c.EntityType,
			ChunkType: c.Tier,
		})
	}
	return result, nil
}

func (r *assessmentRAGRetriever) FormatContext(chunks []assessment.RAGChunk) string {
	ragChunks := make([]rag.RetrievedChunk, 0, len(chunks))
	for _, c := range chunks {
		ragChunks = append(ragChunks, rag.RetrievedChunk{
			EntityType: c.Source,
			ChunkText:  c.Content,
			Similarity: c.Score,
			Tier:       c.ChunkType,
		})
	}
	return r.svc.FormatContext(ragChunks)
}

// couponRenderAdapter adapts coupons.Service to implement the
// renderer.CouponResolver interface for render-time coupon_embed resolution
// (Section 55 P4, BUG-004). This avoids an import cycle: the renderer package
// cannot import coupons (coupons imports worker which imports renderer
// indirectly), so the adapter is wired here in the server package.
type couponRenderAdapter struct {
	svc *coupons.Service
}

func (a *couponRenderAdapter) ResolveCouponForRender(ctx context.Context, tenantID, id pgtype.UUID) (*renderer.CouponRenderResult, error) {
	c, err := a.svc.ResolveCouponForRender(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	return &renderer.CouponRenderResult{
		ID:            c.ID,
		Title:         c.Title,
		Code:          c.Code,
		DiscountType:  c.DiscountType,
		DiscountValue: c.DiscountValue,
		ProviderName:  c.ProviderName,
		ExpiresAt:     c.ExpiresAt,
		IsExpired:     c.IsExpired,
		ReferralURL:   c.ReferralURL,
		Status:        c.Status,
	}, nil
}

// affiliateLinkRenderAdapter adapts coupons.Service to implement the
// renderer.AffiliateLinkResolver interface for render-time affiliate link
// href rewriting (Section 55 P4, BE-CRITICAL-001). Same adapter pattern as
// couponRenderAdapter — avoids an import cycle between renderer and coupons.
type affiliateLinkRenderAdapter struct {
	svc *coupons.Service
}

func (a *affiliateLinkRenderAdapter) ResolveAffiliateLinkForRender(ctx context.Context, tenantID, providerID pgtype.UUID, originalURL string) (*renderer.AffiliateLinkRenderResult, error) {
	r, err := a.svc.ResolveAffiliateLinkForRender(ctx, tenantID, providerID, originalURL)
	if err != nil {
		return nil, err
	}
	return &renderer.AffiliateLinkRenderResult{
		ResolvedURL: r.ResolvedURL,
	}, nil
}

// wsHubWSAdapter adapts *websocket.Hub to the notifications.WSHuber
// interface so the notification service can broadcast real-time `notification`
// WS events without importing the websocket package directly (avoids an
// import cycle). BUG-002: previously the notification service never sent WS
// events. Ref: japaconnect-engineering/sections/35-notification-system.md
type wsHubWSAdapter struct {
	hub *websocket.Hub
}

func (a wsHubWSAdapter) BroadcastToUser(userID string, event notifications.WSEventPayload) {
	a.hub.BroadcastToUser(userID, websocket.WSEvent{
		Type:    event.Type,
		Payload: event.Payload,
	})
}

// mutualAidDMAdapter wraps websocket.DMService to implement the
// mutualaid.dmInitiator interface. Converts ConversationResult to the
// simpler dmConversationResult struct (avoids importing websocket types
// into the mutualaid package).
type mutualAidDMAdapter struct {
	dm *websocket.DMService
}

func (a *mutualAidDMAdapter) GetOrCreateConversationForInitiator(
	ctx context.Context,
	tenantID, initiatorID, recipientID pgtype.UUID,
) (mutualaid.DMConversationResult, error) {
	result, err := a.dm.GetOrCreateConversationForInitiator(ctx, tenantID, initiatorID, recipientID)
	if err != nil {
		return mutualaid.DMConversationResult{}, err
	}
	return mutualaid.DMConversationResult{
		ConversationID: result.Conversation.ID,
		IsNew:          result.IsNew,
	}, nil
}

func (a *mutualAidDMAdapter) SendMessage(
	ctx context.Context,
	tenantID, conversationID, senderID pgtype.UUID,
	body string,
) error {
	return a.dm.SendMessage(ctx, tenantID, conversationID, senderID, body)
}

// personaActionDispatcher adapts the existing action services (comments,
// follows, content, forum, customfields) to the participation.ActionDispatcher
// interface (ADR-077 §5, BUG-010). It is defined here in the server package
// so the participation package does not need to import those five packages —
// the dependency is inverted via the interface.
type personaActionDispatcher struct {
	comments *comments.Service
	follows  *follows.Service
	content  *content.Service
	forum    *forum.Service
	cf       *customfields.Service
}

func (d *personaActionDispatcher) CreateComment(ctx context.Context, tenantID, personaUserID, targetContentID pgtype.UUID, body []byte) (pgtype.UUID, error) {
	c, err := d.comments.CreateComment(ctx, tenantID, personaUserID, comments.CreateCommentInput{
		ContentID: targetContentID,
		Body:      body,
	})
	if err != nil {
		return pgtype.UUID{}, err
	}
	return c.ID, nil
}

func (d *personaActionDispatcher) CreateForumThread(ctx context.Context, tenantID, personaUserID, contentID, spaceID pgtype.UUID, title, slug string, body []byte) (pgtype.UUID, error) {
	// forum_question: the persona starts a new thread. The content row was
	// already published by DispatchAction; pass its ID as ContentID so the
	// forum thread links to it via forum_threads.content_id (BUG-004 —
	// previously content_id was NULL, leaving an orphan content row).
	t, err := d.forum.CreateThread(ctx, tenantID, personaUserID, forum.CreateThreadInput{
		ContentID: contentID,
		SpaceID:   spaceID,
		Title:     title,
		Slug:      slug,
		Body:      body,
	})
	if err != nil {
		return pgtype.UUID{}, err
	}
	return t.ID, nil
}

func (d *personaActionDispatcher) CreateForumReply(ctx context.Context, tenantID, personaUserID, threadID pgtype.UUID, body []byte) (pgtype.UUID, error) {
	r, err := d.forum.CreateReply(ctx, tenantID, threadID, personaUserID, forum.CreateReplyInput{
		Body: body,
	})
	if err != nil {
		return pgtype.UUID{}, err
	}
	return r.ID, nil
}

func (d *personaActionDispatcher) ToggleFollow(ctx context.Context, tenantID, personaUserID pgtype.UUID, entityType string, entityID pgtype.UUID) error {
	_, err := d.follows.ToggleFollow(ctx, tenantID, personaUserID, entityType, entityID)
	return err
}

func (d *personaActionDispatcher) AddReaction(ctx context.Context, tenantID, personaUserID pgtype.UUID, entityType string, entityID pgtype.UUID, reactionKey string) error {
	_, err := d.content.AddReaction(ctx, tenantID, entityType, entityID, personaUserID, reactionKey)
	return err
}

func (d *personaActionDispatcher) CreateBookmark(ctx context.Context, tenantID, personaUserID pgtype.UUID, entityType string, entityID pgtype.UUID) error {
	var zeroFolderID pgtype.UUID
	_, err := d.content.CreateBookmark(ctx, tenantID, personaUserID, entityType, entityID, "default", zeroFolderID)
	return err
}

func (d *personaActionDispatcher) SubmitForm(ctx context.Context, tenantID, personaUserID, formID pgtype.UUID, fieldValues map[string]any) (pgtype.UUID, error) {
	valuesJSON, err := json.Marshal(fieldValues)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("marshal field values: %w", err)
	}
	sub, err := d.cf.CreateFormSubmission(ctx, gen.CreateFormSubmissionParams{
		FormID:      formID,
		TenantID:    tenantID,
		UserID:      personaUserID,
		FieldValues: valuesJSON,
		Status:      "submitted",
	})
	if err != nil {
		return pgtype.UUID{}, err
	}
	return sub.ID, nil
}
