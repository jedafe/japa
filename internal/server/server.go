package server

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/admin"
	"github.com/jedafe/japa/internal/advertising"
	"github.com/jedafe/japa/internal/affiliate"
	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/analytics"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/blog"
	"github.com/jedafe/japa/internal/bookmarkfolders"
	"github.com/jedafe/japa/internal/bots"
	"github.com/jedafe/japa/internal/comments"
	"github.com/jedafe/japa/internal/comparison"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/control"
	"github.com/jedafe/japa/internal/coupons"
	"github.com/jedafe/japa/internal/csrf"
	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/docs"
	emailtemplates "github.com/jedafe/japa/internal/email/templates"
	errtracking "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/features"
	"github.com/jedafe/japa/internal/feed"
	"github.com/jedafe/japa/internal/follows"
	"github.com/jedafe/japa/internal/forum"
	"github.com/jedafe/japa/internal/gdpr"
	"github.com/jedafe/japa/internal/geoip"
	"github.com/jedafe/japa/internal/honcho"
	"github.com/jedafe/japa/internal/i18n"
	"github.com/jedafe/japa/internal/impersonation"
	"github.com/jedafe/japa/internal/interactions"
	"github.com/jedafe/japa/internal/learning"
	"github.com/jedafe/japa/internal/linkpreview"
	"github.com/jedafe/japa/internal/media"
	"github.com/jedafe/japa/internal/mentorship"
	"github.com/jedafe/japa/internal/menus"
	mw "github.com/jedafe/japa/internal/middleware"
	"github.com/jedafe/japa/internal/newsletter"
	"github.com/jedafe/japa/internal/notifications"
	"github.com/jedafe/japa/internal/onboarding"
	"github.com/jedafe/japa/internal/organizations"
	"github.com/jedafe/japa/internal/pagebuilder"
	"github.com/jedafe/japa/internal/payments"
	"github.com/jedafe/japa/internal/professional"
	"github.com/jedafe/japa/internal/publicapi"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/jedafe/japa/internal/redirects"
	"github.com/jedafe/japa/internal/referral"
	"github.com/jedafe/japa/internal/renderer"
	"github.com/jedafe/japa/internal/reports"
	"github.com/jedafe/japa/internal/reviews"
	rsspkg "github.com/jedafe/japa/internal/rss"
	"github.com/jedafe/japa/internal/search"
	"github.com/jedafe/japa/internal/seo"
	"github.com/jedafe/japa/internal/spaces"
	"github.com/jedafe/japa/internal/spam"
	"github.com/jedafe/japa/internal/tags"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/tickets"
	"github.com/jedafe/japa/internal/trending"
	"github.com/jedafe/japa/internal/universities"
	"github.com/jedafe/japa/internal/university"
	"github.com/jedafe/japa/internal/users"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/wallet"
	"github.com/jedafe/japa/internal/webhooks"
	"github.com/jedafe/japa/internal/websocket"
	"github.com/jedafe/japa/internal/worker"
)

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
	geoipUpdater := geoip.NewUpdater(cfg.MaxmindLicenseKey, cfg.GeoIPDBPath, geoipReader)
	worker.RegisterGeoIPUpdater(geoipUpdater)

	// Register site name for worker handlers (RSS feeds, LLMS.txt, emails).
	worker.RegisterSiteName(cfg.SiteName)

	// IP block middleware — reject blocked IPs before they reach any handler.
	// Runs after RealIP (so the resolved IP is available) but before all
	// other middleware and route handlers.
	r.Use(spam.IPBlockMiddleware(database))

	h := NewHandler(database)
	tenantStore := tenant.NewDBStore(database)

	// JWT key rotation — use persistent secret from config when set,
	// otherwise generate a random key for development (sessions won't survive restart).
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

	// Valkey client for cache, rate limiting, edit locks, brute-force
	// tracking, and space unread counters. Created with graceful fallback
	// so the server boots even when Valkey is unreachable.
	vkClient := valkey.NewWithFallback(cfg.RedisAddr)
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
		DB:          authStore,
		Queries:     authStore.Queries(),
		Database:    database,
		KeyRotation: kr,
		BruteForce:  bruteForce,
		Policy:      auth.DefaultPasswordPolicy(),
		VKClient:    vkClient,
		SiteName:      cfg.SiteName,
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

	// Feature flag checker
	featureChecker := features.New(database)

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

	// Hot-reloadable system settings (AUDIT-BE-009). Previously config.NewSettings
	// was never instantiated, so documented operational limits were dead and
	// services used compiled-in constants. Instantiate once and inject where limits
	// are read.
	settingsService := config.NewSettings(database, vkClient.Redis())
	contentService.SetSettings(settingsService)

	// BL-014: apply the configurable iframe embed allowlist from system_settings
	// at startup. The content sanitizer reads this list at render time under a
	// read lock, so a later settings change requires a process restart or a
	// dedicated reload hook. Empty/unset restores the compiled-in defaults.
	iframeHostsCSV := settingsService.GetStringOr(context.Background(), pgtype.UUID{}, "content.sanitization.allowed_iframe_hosts", "")
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
	// handlers will return 503 on SSR requests instead of panicking.
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

	// General Tags subsystem (Sections 08, 49) — distinct from forum_tags.
	// Composes contentService + followService directly.
	tagsService := tags.NewService(database)
	tagsHandlers := tags.NewHandlers(tagsService, contentService, followService)

	// Notification System subsystem (Section 35 — Notification System).
	notificationService := notifications.NewService(database)
	notificationHandlers := notifications.NewHandlers(notificationService)
	// BL-002: wire content mention notification dispatcher (fire-and-forget, Rule 8).
	contentHandlers.SetMentionNotifier(commentNotificationAdapter{svc: notificationService})

	// Comments subsystem (Section 49 — Unified Comments).
	commentsService := comments.NewService(database)
	commentsService.SetSettings(settingsService) // BL-009: configurable comment edit window
	commentsHandlers := comments.NewHandlers(commentsService)
	commentsHandlers.SetSpamPipeline(spamPipeline)
	// BL-001: wire comment notification dispatcher (fire-and-forget, Rule 8).
	commentsHandlers.SetNotifier(commentNotificationAdapter{svc: notificationService})

	// Interactions subsystem (Sections 41, 52 — Blocks & Mutes).
	interactionsService := interactions.NewService(database)
	interactionsService.SetValkey(vkClient.Redis()) // VALKEY-001: mute key cache invalidation
	interactionsHandlers := interactions.NewHandlers(interactionsService)

	worker.RegisterSettings(settingsService)
	worker.RegisterRedis(vkClient.Redis()) // TASK-003: RSS feed caching in Valkey
	// Inject into services with hot-reloadable moderation/ops limits (AUDIT-BE-009).
	spamPipeline.SetSettings(settingsService)
	notificationService.SetAsynqClient(asynqClient)
	notificationService.SetSettings(settingsService)
	authHandlers.AsynqClient = asynqClient
	authHandlers.Settings = settingsService
	oauthHandlers.Settings = settingsService
	kr.Settings = settingsService
	contentHandlers.AsynqClient = asynqClient
	// Wire password policy from system_settings (BE-054).
	if defaultTenantPW, err := tenantStore.GetDefaultTenant(context.Background()); err == nil && defaultTenantPW != nil {
		authHandlers.Policy = auth.PasswordPolicyFromSettings(context.Background(), settingsService, defaultTenantPW.ID)
	}
	// Wire link preview blacklist from system_settings (AUDIT-BE-010, BE-037).
	// The blacklist is comma-separated domains stored under 'content.link_preview.blacklist'.
	// Resolve the default tenant ID for settings lookup.
	if defaultTenant, err := tenantStore.GetDefaultTenant(context.Background()); err == nil && defaultTenant != nil {
		if blacklistStr, err := settingsService.GetSetting(context.Background(), defaultTenant.ID, "content.link_preview.blacklist"); err == nil && blacklistStr != "" {
			blacklist := strings.Split(blacklistStr, ",")
			for i := range blacklist {
				blacklist[i] = strings.TrimSpace(blacklist[i])
			}

			linkPreviewFetcher.SetBlacklist(blacklist)
			slog.Info("link preview: blacklist loaded from system_settings", "count", len(blacklist))
		}
	} else {
		slog.Warn("link preview: could not resolve default tenant for blacklist", "error", err)
	}

	// Feed System subsystem (Section 32 — Activity Feed / Timeline).
	feedService := feed.NewService(database, vkClient.Redis())
	feedService.SetEnqueuer(taskEnqueuer)
	feedHandlers := feed.NewHandlers(feedService)
	feedHandlers.SetSettings(settingsService)

	// Trending & Discovery subsystem (Phase 28).
	trendingStore := trending.NewStore(database)
	trendingHandlers := trending.NewHandlers(trendingStore)

	// Register trending processor with worker for background tasks.
	trendingProc := trending.NewProcessor(database, slog.Default())
	trendingAdapter := trending.NewWorkerAdapter(trendingProc)
	worker.RegisterTrendingProcessor(trendingAdapter)

	// Blog Extension subsystem (Section 24 — Blog Extension).
	blogService := blog.NewService(database)
	blogHandlers := blog.NewHandlers(blogService, contentService, cfg.SiteURL, cfg.SiteName)

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
	orgService := organizations.NewService(database)
	orgHandlers := organizations.NewHandlers(orgService, followService)

	// Wallet & Credits subsystem (Section 28 — Wallet & Credits).
	walletService := wallet.NewService(database)
	walletService.SetSettings(settingsService) // SETTING-007/008: global conversion rate + min convert points
	walletHandlers := wallet.NewHandlers(walletService)
	walletService.SetEnqueuer(taskEnqueuer)

	// Register wallet processor with worker for background tasks.
	worker.RegisterWalletProcessor(walletService)

	// Payment Core subsystem (Section 17 — Payment Core).
	paymentService := payments.NewService(database, &walletCreditAdapter{svc: walletService})
	paymentService.SetEnqueuer(taskEnqueuer)

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

	// Advertising subsystem (Section 25 — Ad Placement).
	adService := advertising.NewService(database, settingsService)
	adHandlers := advertising.NewHandlers(adService)
	adHandlers.AsynqClient = asynqClient

	// Ticket System subsystem (Section 38 — Ticket System).
	ticketService := tickets.NewServiceWithEnqueuer(database, taskEnqueuer)
	ticketHandlers := tickets.NewHandlers(ticketService)

	// Coupons & External Affiliate Providers subsystem (Section 55).
	couponService := coupons.NewServiceWithEnqueuer(database, taskEnqueuer)
	couponHandlers := coupons.NewHandlers(couponService)

	// User Affiliate Program subsystem (Section 56).
	affiliateService := affiliate.NewServiceWithEnqueuer(database, taskEnqueuer)
	affiliateService.SetSettings(settingsService) // SETTING-002/003: configurable commission/limits
	affiliateHandlers := affiliate.NewHandlers(affiliateService)
	affiliateHandlers.AsynqClient = asynqClient

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
	gdprHandlers := gdpr.NewHandlers(gdprService)

	// Register GDPR processor with worker for background tasks.
	gdprAdapter := gdpr.NewWorkerAdapter(gdprService)
	worker.RegisterGDPRProcessor(gdprAdapter)

	// Menus — Navigation menus with RBAC-gated items (ADR-054).
	menusService := menus.NewService(database)
	menusHandlers := menus.NewHandlers(menusService)

	// Email Templates — Tenant-customizable transactional emails (ADR-055).
	emailTemplatesService := emailtemplates.NewService(database)
	emailTemplatesHandlers := emailtemplates.NewHandlers(emailTemplatesService)

	// Redirects — Admin-managed URL redirect rules (ADR-056).
	redirectsService := redirects.NewService(database, vkClient.Redis())
	redirectsService.SetEnqueuer(taskEnqueuer)
	redirectsHandlers := redirects.NewHandlers(redirectsService)

	// Media subsystem (Section 34 — Media System).
	mediaService := media.NewService(database)

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

	// Documentation System subsystem (Section 44).
	docsService := docs.NewService(database)
	docsService.SetCache(vkClient.Redis()) // VALKEY-003: cache doc pages + nav
	docsHandlers := docs.NewHandlers(docsService, cfg.SiteURL, cfg.SiteName)
	docsHandlers.AsynqClient = asynqClient // SEARCH-001: enqueue Meilisearch index sync

	// Admin Settings subsystem.
	adminHandlers := admin.NewHandlers(database, gen.New(database.Writer))
	adminHandlers.SetRBAC(rbacResolver)
	adminHandlers.SetValkey(vkClient.Redis())
	adminHandlers.SetSettings(settingsService)
	adminHandlers.SetGDPR(gdprService)

	// Bot & Crawler Management subsystem (Section 43).
	botStore := bots.NewStoreWithDB(database)
	botService := bots.NewServiceFromStore(botStore)
	botHandlers := bots.NewHandlers(botService)

	// Spam Prevention & Content Safety admin handlers (Section 41).
	spamHandlers := spam.NewHandlers(spamStore)

	// Platform Analytics & Site Reports subsystem (Section 40).
	analyticsService := analytics.NewServiceWithEnqueuer(database, taskEnqueuer)
	analyticsHandlers := analytics.NewHandlers(analyticsService)

	// Learning Platform subsystem (Section 36).
	learningStore := learning.NewStore(database)
	learningHandlers := learning.NewHandlers(learningStore)
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
	universityHandlers := university.NewHandlers(universityService)
	// Register university processor for the worker's 5 university Asynq tasks.
	universityProcessor := university.NewProcessor(universityService)
	worker.RegisterUniversityProcessor(universityProcessor)

	// Newsletter subsystem (Section 33).
	newsletterService := newsletter.NewService(database)
	newsletterService.SetAsynqClient(asynqClient)
	newsletterHandlers := newsletter.NewHandlers(newsletterService)

	// Referral Program subsystem (Section 34).
	referralService := referral.NewService(database)
	referralHandlers := referral.NewHandlers(referralService)

	// Mentorship subsystem (Section 35).
	mentorshipService := mentorship.NewService(database)
	mentorshipHandlers := mentorship.NewHandlers(mentorshipService)

	// Page Builder subsystem (Section 16 — Page Builder admin CRUD).
	pagebuilderService := pagebuilder.NewService(database)
	pagebuilderHandlers := pagebuilder.NewHandlers(pagebuilderService)

	// Register analytics processor with worker for background tasks.
	analyticsAdapter := analytics.NewWorkerAdapter(analyticsService)
	worker.RegisterAnalyticsProcessor(analyticsAdapter)

	// AI Core subsystem (Section 20 — AI Core).
	aiDispatcher := ai.NewDispatcher()

	// Register Xiaomi MiMo as the primary content generation provider.
	if cfg.XiaomiAPIKey != "" {
		xiaomiProvider := ai.NewXiaomiProvider(cfg.XiaomiAPIKey, "")
		aiDispatcher.RegisterProvider("content_generation", xiaomiProvider)
		slog.Info("ai: registered xiaomi provider", "model", ai.XiaomiModelFlash)
	}

	aiStore := ai.NewStore(database.Reader, database.Writer)
	aiHandlers := ai.NewHandlers(aiDispatcher, aiStore)
	aiHandlers.SetHonchoClient(honcho.NewClient(cfg.HonchoURL))
	aiHandlers.AsynqClient = asynqClient

	aiToolRegistry := ai.NewToolRegistry(database)

	aiToolHandlers := ai.NewToolHandlers(aiToolRegistry)

	// Content Embedder — generates vector embeddings via the AI dispatcher
	// and stores them in content_scores.relevance_embedding. Registered with
	// the worker so the ai:generate_content_embedding handler can invoke it.
	contentEmbedder := ai.NewContentEmbedder(aiDispatcher, database.Writer)
	worker.RegisterContentEmbedder(contentEmbedder)

	// Cache flusher for admin-only cache invalidation.
	cacheFlusher := analytics.NewCacheFlusher(vkClient)
	analyticsHandlers.SetCacheFlusher(cacheFlusher)

	// Register AI and moderation processor adapters so the ai:process_task
	// and content:scan_moderation background handlers use real implementations
	// instead of noops. Must come before RegisterNoopProcessors() so the noop
	// guard (if nil → register noop) sees the real adapters.
	aiProcessorAdapter := ai.NewWorkerAdapter(aiDispatcher)
	worker.RegisterAIProcessor(aiProcessorAdapter)

	moderationProcessorAdapter := spam.NewWorkerAdapter(spamPipeline)
	worker.RegisterModerationProcessor(moderationProcessorAdapter)

	// Register no-op implementations for any remaining processor interface
	// that hasn't been wired to a real service yet. This ensures all Asynq
	// task handlers degrade gracefully instead of crashing on a nil interface.
	worker.RegisterNoopProcessors()

	// Admin Impersonation subsystem (Section 46).
	impersonationStore := impersonation.NewStore(vkClient.Redis())
	impersonationService := impersonation.NewService(impersonationStore, database.Writer)
	impersonationHandlers := impersonation.NewHandlers(impersonationService)

	// Public API & Developer Portal subsystem (Section 47).
	publicapi.SetSiteURL(cfg.SiteURL)
	publicAPIStore := publicapi.NewStore(database.Writer)
	publicAPIService := publicapi.NewService(publicAPIStore)
	publicAPIHandlers := publicapi.NewHandlers(publicAPIService)
	publicAPIHandlers.SetQueries(gen.New(database.Reader))

	// User Profile & Preferences subsystem.
	usersService := users.NewService(database)
	usersHandlers := users.NewHandlers(usersService)

	// Meilisearch client for search (graceful if unreachable — NewClient
	// logs a warning and returns a usable client so the server boots even
	// when the search engine is down).
	meiliClient, err := search.NewClient(cfg.MeilisearchURL, cfg.MeilisearchKey)
	if err != nil {
		slog.Warn("meilisearch client not created", "error", err)
	}

	// Search subsystem.
	// The indexer syncs content rows into the posts index from the reader
	// pool; it is registered with the worker so the search_index_sync task
	// can upsert documents asynchronously.
	searchIndexer := search.NewIndexer(meiliClient, gen.New(database.Reader))
	worker.RegisterSearchIndexer(searchIndexer)

	searchHandlers := search.NewHandlers(meiliClient)
	searchHandlers.SetQueries(gen.New(database.Reader))
	searchHandlers.SetSettings(settingsService)

	// Onboarding subsystem (Section 09).
	onboardingService := onboarding.NewService(database, settingsService)
	onboardingHandlers := onboarding.NewHandlers(onboardingService)

	// SEO & Performance subsystem (Section 26).
	seoQueries := gen.New(database.Writer)
	sitemapHandler := seo.NewSitemapHandler(seoQueries, database.Reader)
	ogImageHandler := seo.NewOGImageHandler(database.Reader, cfg.SiteName)

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

	// Infrastructure routes bypass tenant middleware
	r.Get("/health", h.HealthCheck)
	r.Get("/ready", h.ReadinessCheck)

	// Public bot-facing endpoints (robots.txt and llms.txt)
	// These require tenant resolution but no authentication.
	r.Group(func(r chi.Router) {
		r.Use(tenant.Middleware(tenantStore))
		r.Get("/robots.txt", bots.RobotsHandler(botStore, vkClient))
		r.Get("/llms.txt", bots.LLMsHandler(botStore, vkClient, cfg.SiteName, cfg.SiteDescription))
		r.Get("/llms-full.txt", bots.LLMsHandler(botStore, vkClient, cfg.SiteName, cfg.SiteDescription))
		r.Get("/sitemap.xml", sitemapHandler.ServeSitemap)
		r.Get("/sitemap-content.xml", sitemapHandler.ServeContentSitemap)
		r.Get("/sitemap-spaces.xml", sitemapHandler.ServeSpacesSitemap)
		r.Get("/sitemap-tags.xml", sitemapHandler.ServeTagsSitemap)
		r.Get("/sitemap-users.xml", sitemapHandler.ServeUsersSitemap)
		r.Get("/og/{id}.png", ogImageHandler.ServeOGImage)
		// Universal RSS feeds (public — no auth required, tenant-scoped)
		r.Get("/feed.xml", rsspkg.HandleFeed(gen.New(database.Reader), "all", cfg.SiteURL, cfg.SiteName))
		r.Get("/spaces/{slug}/feed.xml", rsspkg.HandleSpaceFeed(gen.New(database.Reader), cfg.SiteURL, cfg.SiteName))
		r.Get("/tags/{slug}/feed.xml", rsspkg.HandleTagFeed(gen.New(database.Reader), cfg.SiteURL, cfg.SiteName))
		r.Get("/users/{username}/feed.xml", rsspkg.HandleUserFeed(gen.New(database.Reader), cfg.SiteURL, cfg.SiteName))

		// Public settings endpoint (no auth required, tenant-scoped)
		r.Get("/site/settings/public", configHandlers.GetPublicSettings)

		// API Playground & OpenAPI Spec (Section 44 — docs subdomain)
		r.Get("/docs/api/playground", docsHandlers.APIPlaygroundHandler)
		// catalog: GET /docs/openapi.json (single canonical spec, §6.4 dedupe)
		r.Get("/docs/openapi.json", docsHandlers.OpenAPIHandler)
	})

	// API routes go through tenant resolution
	r.Group(func(r chi.Router) {
		r.Use(tenant.Middleware(tenantStore))
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
						AND (expires_at IS NULL OR expires_at > now())
					)`, userID).Scan(&banned)
				if err != nil || !banned {
					return false, nil
				}

				_ = database.Reader.QueryRow(ctx,
					`SELECT expires_at FROM user_moderation_actions
					WHERE user_id = $1 AND action IN ('ban', 'suspension')
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
		r.Use(bots.BotDetectionMiddleware(botStore))

		// CSRF double-submit enforcement for all mutating /api/v1 requests
		// (Bug Hunter §5). Cookie-authenticated browser requests (the admin
		// SPA) are protected; non-cookie auth (Authorization: Bearer,
		// X-API-Key) is exempt — those clients are CSRF-immune and would
		// otherwise break. The public /auth/* subtree (login, register,
		// password reset) is also exempt: those endpoints are reached before
		// any token cookie exists and are not state-changing on behalf of an
		// already-authenticated session. Safe GET/OPTIONS pass through.
		r.Use(csrf.Middleware([]string{"/"}, func(r *http.Request) bool {
			return csrf.IsNonCookieAuthenticated(r) ||
				strings.HasPrefix(r.URL.Path, "/api/v1/auth/") ||
				strings.HasPrefix(r.URL.Path, "/api/v1/forms/")
		}))

		// CSRF token bootstrap endpoint — issues the double-submit cookie and
		// echoes the token so the SPA can attach it as X-CSRF-Token. The SPA
		// calls this once at startup and again after a 403 CSRF rejection.
		r.Get("/csrf", csrf.TokenHandler(cfg.CookieDomain, cfg.IsProduction()))

		// Impersonation middleware (SEC-002) — injects impersonated identity
		// into request context when an active impersonation session exists.
		// Runs after auth + bot detection so the impersonator is authenticated
		// first, then the impersonated user's identity is swapped in.
		r.Use(impersonation.ImpersonationMiddleware(impersonationStore))

		// TOTP-for-admin enforcement (SEC-006) — when the
		// "security.totp_required_for_admin" setting is enabled, admin-level
		// users (per rbac.HasAdminCapability) without an active TOTP
		// enrollment are rejected with 403 totp_required on /api/v1/admin/*.
		r.Use(auth.RequireTOTPForAdmin(settingsService, rbacResolver, totpStore))

		// Auth routes (public - no auth required, tenant-scoped)
			// Tight rate limit to throttle brute-force attempts.
			r.Route("/auth", func(r chi.Router) {
				r.Use(mw.RateLimit(vkClient.Redis(), "tight"))
				r.Post("/register", authHandlers.Register)
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
			featureHandlers := features.NewHandlers(featureChecker)
			r.Get("/feature-flags", featureHandlers.HandleGetFeatures)

			// Feature flags admin toggle (requires auth + RBAC permission)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "features.manage")).
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

			// Field value routes
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/custom-field-values/{entity_type}/{entity_id}", cfHandlers.ListFieldValues)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.values.set")).
				Put("/custom-field-values", cfHandlers.SetFieldValue)

			// Form routes
			// Public submission (optional auth — anonymous users can submit forms)
			r.With(auth.OptionalAuth(kr, vkClient.Redis())).Post("/forms/{id}/submissions", cfHandlers.SubmitForm)
			// Read (require auth)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/forms", cfHandlers.ListForms)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/forms/{id}", cfHandlers.GetForm)
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

			// Admin field-group routes — the same underlying handlers as the
			// /custom-field-groups block above, mounted a second time under
			// /admin per catalog (an admin-facing management surface distinct
			// from the tenant-scoped API the SPA consumes at the bare paths).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.group.view")).
				Get("/admin/field-groups", cfHandlers.ListFieldGroups)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.group.create")).
				Post("/admin/field-groups", cfHandlers.CreateFieldGroup)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.group.edit")).
				Patch("/admin/field-groups/{id}", cfHandlers.UpdateFieldGroup)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "custom_fields.group.delete")).
				Delete("/admin/field-groups/{id}", cfHandlers.DeleteFieldGroup)
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

			// Space type routes — reads stay bare (any user creating a space needs
			// the type list for e.g. a dropdown); writes move under /admin per
			// catalog (management of the type definitions themselves is admin-only).
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/space-types", spaceHandlers.ListSpaceTypes)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/space-types/{id}", spaceHandlers.GetSpaceType)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "space_types.create")).
				Get("/admin/space-types", spaceHandlers.ListSpaceTypes)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "space_types.create")).
				Post("/admin/space-types", spaceHandlers.CreateSpaceType)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "space_types.edit")).
				Patch("/admin/space-types/{id}", spaceHandlers.UpdateSpaceType)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "space_types.delete")).
				Delete("/admin/space-types/{id}", spaceHandlers.DeleteSpaceType)

			// Space routes
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/spaces", spaceHandlers.ListSpaces)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/spaces", spaceHandlers.ListMySpaces)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/spaces/moderated", spaceHandlers.ListMyModeratedSpaces)
			// NOTE (P5): /me/spaces/owned has no wiring — the spaces table has no
			// owner_id/created_by column (consistent with the geo-first model where
			// Country/State/City spaces aren't owned by anyone); needs a schema
			// decision, not just a query.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService)).
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
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService)).
				Get("/spaces/{slug}/members", spaceHandlers.ListMembers)

			// Space follow/unfollow (Section 31)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService)).
				Post("/spaces/{slug}/follow", spaceHandlers.ToggleFollow)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService)).
				Delete("/spaces/{slug}/follow", spaceHandlers.ToggleFollow)

			// Space mute/unmute (Section 52)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService)).
				Post("/spaces/{slug}/mute", spaceHandlers.MuteSpace)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService)).
				Delete("/spaces/{slug}/mute", spaceHandlers.UnmuteSpace)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService)).
				Get("/spaces/{slug}/mute-status", spaceHandlers.GetMuteStatus)

			// Space content & pinned posts
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService)).
				Get("/spaces/{slug}/content", spaceHandlers.ListSpaceContent)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService)).
				Get("/spaces/{slug}/pinned", spaceHandlers.ListPinned)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService), rbac.RequirePermission(rbacResolver, "spaces.moderate")).
				Post("/spaces/{slug}/pinned", spaceHandlers.AddPinned)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService), rbac.RequirePermission(rbacResolver, "spaces.moderate")).
				Delete("/spaces/{slug}/pinned/{content_id}", spaceHandlers.RemovePinned)

			// Dedicated leave (catalog: DELETE /spaces/:id/leave), split from
			// the join/leave toggle above.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService)).
				Delete("/spaces/{slug}/leave", spaceHandlers.Leave)
			// catalog: POST /spaces/:id/report — takes a raw ID (like the other
			// report routes), not the slug the rest of this block resolves on.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "moderation.report")).Post("/spaces/{id}/report", reportsHandlers.ReportSpace)

			// Space rule routes (slug-resolved via SpaceResolver middleware)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService)).
				Get("/spaces/{slug}/rules", spaceHandlers.ListSpaceRules)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService), rbac.RequirePermission(rbacResolver, "spaces.moderate")).
				Post("/spaces/{slug}/rules", spaceHandlers.CreateSpaceRule)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService), rbac.RequirePermission(rbacResolver, "spaces.moderate")).
				Patch("/spaces/{slug}/rules/{ruleId}", spaceHandlers.UpdateSpaceRule)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService), rbac.RequirePermission(rbacResolver, "spaces.moderate")).
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
			// Create (require auth + moderate rate limit + permission)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "moderate"), rbac.RequirePermission(rbacResolver, "content.create")).
				Post("/content", contentHandlers.CreateContent)
			// Read (require auth)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/content", contentHandlers.ListContent)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/content/{slug}", contentHandlers.GetContent)
			// Content types (public — for frontend content type selection)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/content-types", contentHandlers.ListContentTypes)
			// Update/Delete (require auth + permission)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.edit")).
				Patch("/content/{id}", contentHandlers.UpdateContent)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.delete")).
				Delete("/content/{id}", contentHandlers.DeleteContent)
			// Lifecycle: publish/archive/schedule are status transitions (content.edit);
			// view/share-count just require auth.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.edit")).
				Post("/content/{id}/publish", contentHandlers.PublishContent)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.edit")).
				Post("/content/{id}/archive", contentHandlers.ArchiveContent)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.edit")).
				Post("/content/{id}/schedule", contentHandlers.ScheduleContent)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/content/{id}/view", contentHandlers.RecordContentView)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/content/{id}/share-count", contentHandlers.GetContentShareCount)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "moderation.report")).Post("/content/{id}/report", reportsHandlers.ReportContent)
			// Revisions (require auth + permission to view)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.revisions.view")).
				Get("/content/{id}/revisions", contentHandlers.ListRevisions)
			// Pin/Unpin (require auth + content.pin permission)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.pin")).
				Post("/content/{id}/pin", contentHandlers.PinContent)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.pin")).
				Delete("/content/{id}/pin", contentHandlers.UnpinContent)
			// Reactions (require auth + permission)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.reactions.add")).Post("/content/{id}/reactions", contentHandlers.AddReaction)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.reactions.remove")).Delete("/content/{id}/reactions/{key}", contentHandlers.RemoveReaction)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/content/{id}/reactions", contentHandlers.ListReactions)

			// Reaction type routes (admin-configurable emoji set)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/reaction-types", contentHandlers.ListReactionTypes)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.reaction_types.manage")).
				Post("/reaction-types", contentHandlers.CreateReactionType)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.reaction_types.manage")).
				Patch("/reaction-types/{key}", contentHandlers.UpdateReactionType)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.reaction_types.manage")).
				Delete("/reaction-types/{key}", contentHandlers.DeleteReactionType)

			// JSON-LD template routes
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/content/jsonld-templates", contentHandlers.ListJSONLDTemplates)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.jsonld.manage")).
				Put("/content/jsonld-templates", contentHandlers.UpsertJSONLDTemplate)

			// Page builder block types (Section 16 — Page Builder).
			// Any authenticated user can view the block registry; no
			// special permission required.
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/page-builder/block-types", contentHandlers.ListBlockTypes)

			// Renderer routes (Section 15 — Block Editor SSR)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/content/render", rendererHandlers.Render)

			// Link preview routes (Section 15 — Link Preview Fetch)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/link-preview", linkPreviewHandlers.Fetch)

			// Content Bookmarks (Section 14 — Content System API)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "bookmarks.create")).Post("/content/{id}/bookmark", contentHandlers.AddBookmark)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "bookmarks.create")).Delete("/content/{id}/bookmark", contentHandlers.RemoveBookmark)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/content/{id}/bookmark", contentHandlers.GetBookmarkStatus)

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

			// Comments (Section 49 — Unified Comment System)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/comments/{id}", commentsHandlers.GetComment)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/content/{id}/comments", commentsHandlers.ListComments)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comments.create")).Post("/content/{id}/comments", commentsHandlers.CreateComment)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comments.edit_own")).Patch("/comments/{id}", commentsHandlers.UpdateComment)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comments.delete_own")).Delete("/comments/{id}", commentsHandlers.DeleteComment)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comments.vote")).Post("/comments/{id}/vote", commentsHandlers.VoteComment)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comments.vote")).Delete("/comments/{id}/vote", commentsHandlers.RemoveVote)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "moderation.report")).Post("/comments/{id}/report", reportsHandlers.ReportComment)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.pin")).
				Post("/comments/{id}/pin", commentsHandlers.PinComment)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.pin")).
				Delete("/comments/{id}/pin", commentsHandlers.UnpinComment)

			// Follow System routes (Section 31 — Follow System)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "follows.toggle")).Post("/follows", followHandlers.ToggleFollow)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/following", followHandlers.ListMyFollowing)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/followed-tags", followHandlers.ListMyFollowedTags)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/users/{user_id}/followers", followHandlers.GetFollowers)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/users/{user_id}/following", followHandlers.GetFollowing)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/users/{user_id}/mutual/{other_user_id}", followHandlers.CheckMutual)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/users/{user_id}/follow-status", followHandlers.GetFollowStatus)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/spaces/{space_id}/followers", followHandlers.GetFollowers)

			// User Blocks (Section 41 — user-facing, non-admin)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "interactions.block.manage")).Post("/users/{user_id}/block", interactionsHandlers.ToggleBlock)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "interactions.block.manage")).Delete("/users/{user_id}/block", interactionsHandlers.ToggleBlock)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/blocked-users", interactionsHandlers.ListBlocked)

			// User Mutes (Section 52)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "interactions.mute.manage")).Post("/users/{user_id}/mute", interactionsHandlers.MuteUser)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "interactions.mute.manage")).Delete("/users/{user_id}/mute", interactionsHandlers.UnmuteUser)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/users/{user_id}/mute-status", interactionsHandlers.GetUserMuteStatus)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "moderation.report")).Post("/users/{id}/report", reportsHandlers.ReportUser)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/muted-users", interactionsHandlers.ListMutedUsers)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/muted-spaces", interactionsHandlers.ListMutedSpaces)

			// Notification System routes (Section 35 — Notification System)
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
			// NOTE (§7.1): catalog is DELETE /me/push-tokens/:id; handler currently keys
			// by endpoint in the body. Path-param (token id) migration deferred to P4.
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/me/push-tokens", notificationHandlers.HandlePushUnsubscribe)

			// Feed routes (Section 32 — Activity Feed / Timeline)
			r.Get("/feed/card-style", feedHandlers.GetCardStyle)                                     // public — no auth required
			r.Get("/explore", feedHandlers.GetExploreFeed)                                           // public — no auth required (catalog: /explore)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/feed", feedHandlers.GetPrimaryFeed)              // catalog: /feed (primary timeline)
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

			// Geo routes (Phase 30 — Geo IP; catalog: Sections 05, 37)
			// Public: returns geo data for the current request IP (resolved by middleware)
			r.Get("/geo/me", geoHandlers.HandleGeoMe)
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

			// University admin CRUD (P5.6 — Task 11, catalog paths)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings")).
				Post("/admin/universities", universitiesHandlers.CreateUniversity)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings")).
				Patch("/admin/universities/{id}", universitiesHandlers.UpdateUniversity)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings")).
				Delete("/admin/universities/{id}", universitiesHandlers.DeleteUniversity)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings")).
				Post("/admin/universities/{id}/verify", universitiesHandlers.VerifyUniversity)

			// NOTE (P5): GET /universities/:id/compare deferred — the existing
			// comparison system is content-authored (pre-defined items/dimensions
			// attached to a comparison article), not ad-hoc entity-by-ID comparison.
			// Needs a product decision on which "compare" behavior is meant before
			// wiring: show existing authored comparisons mentioning this university,
			// or dynamically diff custom-field values between universities.

			// WebSocket + DM routes (Section 10 — Real-Time WebSockets)
			wsHub := websocket.NewHub()
			controlHandlers.SetWSHub(wsHub)
			// PubSub bridge for multi-instance WebSocket scaling.
			wsBridge := websocket.NewPubSubBridge(vkClient.Redis(), wsHub)
			wsHub.SetPubSub(wsBridge)

			// PGX-002: use a signal-aware context so the bridge stops on
			// SIGINT/SIGTERM instead of running forever on context.Background().
			wsCtx, wsCancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			_ = wsCancel // cancelled implicitly when the process receives the signal
			go wsBridge.Start(wsCtx)

			wsService := websocket.NewService(database, wsHub)
			wsDMStore := websocket.NewDMStore(gen.New(database.Reader), gen.New(database.Writer))
			wsDMService := websocket.NewDMService(wsDMStore, wsHub)
			wsDMService.AsynqClient = asynqClient
			wsHandlers := websocket.NewHandlers(wsService, wsDMService, wsHub, vkClient, kr)
			wsHandlers.SetSpamPipeline(spamPipeline)

			// WebSocket + DM routes — gated by feature.dm (Section 10)
			r.With(features.Gate(featureChecker, "feature.dm")).Group(func(r chi.Router) {
				// WebSocket — tight rate limit on upgrade to prevent abuse
				// (10 connections per minute per IP, per ADR-022).
				r.With(mw.RateLimit(vkClient.Redis(), "tight")).Get("/ws", wsHandlers.HandleWebSocket)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "dm.view")).Post("/auth/ws-ticket", wsHandlers.GetWSTicket)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/ws/online", wsHandlers.GetOnlineUsers)

				// Direct Messaging
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/dm/conversations", wsHandlers.ListConversations)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "dm.request")).Post("/dm/conversations", wsHandlers.InitiateConversation)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/dm/conversations/{id}/messages", wsHandlers.ListMessages)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "dm.send")).Post("/dm/conversations/{id}/messages", wsHandlers.SendMessage)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "dm.send")).Patch("/dm/conversations/{id}/messages/{mid}", wsHandlers.EditMessage)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "dm.delete")).Delete("/dm/conversations/{id}/messages/{mid}", wsHandlers.DeleteMessage)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/dm/conversations/{id}/read", wsHandlers.MarkAsRead)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "moderation.report")).Post("/dm/conversations/{id}/messages/{mid}/report", reportsHandlers.ReportDMMessage)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/dm/requests/{id}/accept", wsHandlers.AcceptMessageRequest)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/dm/requests/{id}/decline", wsHandlers.DeclineMessageRequest)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/dm/requests/{id}/block", wsHandlers.BlockMessageRequest)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "dm.manage_privacy")).Put("/dm/privacy", wsHandlers.UpdateDMPrivacy)
			})

			// Media routes (Section 34 — Media System)
			mediaHandlers := media.NewHandlers(mediaService)
			mediaHandlers.AsynqClient = asynqClient

			// Media serving endpoint (no auth required for serving files)
			r.Get("/media/serve/{bucket}/*", mediaHandlers.ServeMedia)

			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.upload")).Post("/media/upload", mediaHandlers.InitiateUpload)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/media/confirm/{id}", mediaHandlers.ConfirmUpload)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/media/{id}", mediaHandlers.GetMedia)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Patch("/media/{id}", mediaHandlers.UpdateMetadata)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.delete_own")).Delete("/media/{id}", mediaHandlers.DeleteMedia)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/media", mediaHandlers.ListMedia)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/media", mediaHandlers.ListMyMedia)
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
				Delete("/admin/media/{id}", mediaHandlers.DeleteMedia)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.edit_any")).
				Patch("/admin/media/{id}", mediaHandlers.AdminUpdateMetadata)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.quarantine.manage")).
				Post("/admin/media/{id}/status", mediaHandlers.AdminUpdateMediaStatus)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.upload")).
				Post("/admin/media/upload", mediaHandlers.AdminUpload)

			// Forum Q&A / Solved routes — gated by feature.forum (Section 08)
			forumService := forum.NewService(database)
			forumService.SetSettings(settingsService)
			forumHandlers := forum.NewHandlers(forumService)

			r.With(features.Gate(featureChecker, "feature.forum")).Group(func(r chi.Router) {
				// Thread routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.thread.create")).Post("/forum", forumHandlers.CreateThread)
				r.Get("/forum", forumHandlers.ListThreads)
				r.Get("/forum/{slug}", forumHandlers.GetThread)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.thread.edit_own")).
					Patch("/forum/{id}", forumHandlers.UpdateThread)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.thread.delete_own")).
					Delete("/forum/{id}", forumHandlers.DeleteThread)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.thread.solve")).
					Post("/forum/{id}/solve", forumHandlers.SolveThread)

				// Reply routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.reply.create")).Post("/forum/{id}/replies", forumHandlers.CreateReply)
				r.Get("/forum/{id}/replies", forumHandlers.ListReplies)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.reply.edit_own")).
					Patch("/forum/replies/{id}", forumHandlers.UpdateReply)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.reply.delete_own")).
					Delete("/forum/replies/{id}", forumHandlers.DeleteReply)

				// Vote route
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.vote")).Post("/forum/vote", forumHandlers.Vote)

				// Poll routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.poll.create")).
					Post("/forum/polls", forumHandlers.CreatePoll)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.poll.vote")).Post("/forum/polls/{id}/vote", forumHandlers.VotePoll)
				r.Get("/forum/polls/{id}", forumHandlers.GetPoll)

				// Tag routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.tag.create")).
					Post("/forum/tags", forumHandlers.CreateTag)
				r.Get("/forum/tags", forumHandlers.ListTags)
				r.Get("/forum/tags/{slug}", forumHandlers.GetTagBySlug)
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
				// blog_post), so this reuses the existing content.* permission
				// nodes rather than inventing blog-specific ones.
				r.Get("/blog", blogHandlers.ListPosts)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "moderate"), rbac.RequirePermission(rbacResolver, "content.create")).
					Post("/blog", blogHandlers.CreatePost)
				r.Get("/blog/{slug}", blogHandlers.GetPost)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.edit")).
					Patch("/blog/{id}", contentHandlers.UpdateContent)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.delete")).
					Delete("/blog/{id}", contentHandlers.DeleteContent)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.edit")).
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
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/blog/reviews", blogHandlers.ListReviews)
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
				// (reviewer-scoped list) kept as a documented extra.
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "reviews.create")).Post("/reviews/{entity_type}/{entity_id}", reviewHandlers.CreateReview)
				r.Get("/reviews", reviewHandlers.ListReviews)
				r.Get("/reviews/{entity_type}/{entity_id}", reviewHandlers.ListReviews)
				r.Get("/reviews/{entity_type}/{entity_id}/aggregate", reviewHandlers.GetAggregate)
				r.Get("/reviews/{id}", reviewHandlers.GetReview)
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
				r.With(auth.RequireAuth(kr, vkClient.Redis())).
					Post("/comparisons/auto-dimensions", comparisonHandlers.AutoGenerateDimensions)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/comparisons/{id}", comparisonHandlers.GetComparison)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/comparisons/{id}/matrix", comparisonHandlers.GetComparisonMatrix)

				// Public comparison list (P5.6 — Task 2)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/comparisons", comparisonHandlers.ListComparisons)
				// Comparison update/delete at top level (P5.6 — Task 2, catalog paths)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comparison.edit_own")).
					Patch("/comparisons/{id}", comparisonHandlers.UpdateComparisonItem)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comparison.delete_own")).
					Delete("/comparisons/{id}", comparisonHandlers.DeleteComparisonItem)

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
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/profiles/types", professionalHandlers.ListProfessionalProfileTypes)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/profiles/types/{slug}", professionalHandlers.GetProfessionalProfileType)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.type.create")).
					Patch("/profiles/types/{id}", professionalHandlers.UpdateProfessionalProfileType)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.type.create")).
					Delete("/profiles/types/{id}", professionalHandlers.DeleteProfessionalProfileType)

				// Profile CRUD routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.create")).
					Post("/profiles", professionalHandlers.CreateProfessionalProfile)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/profiles", professionalHandlers.ListProfessionalProfiles)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/profiles/{slug}", professionalHandlers.GetProfessionalProfile)
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
			r.With(auth.OptionalAuth(kr, vkClient.Redis())).Get("/organizations", orgHandlers.ListPublicOrganizations)
			r.With(auth.OptionalAuth(kr, vkClient.Redis())).Get("/organizations/{slug}", orgHandlers.GetPublicOrganization)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "organizations.edit_own")).Patch("/organizations/{id}", orgHandlers.UpdateOrganization)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "organizations.delete_own")).Delete("/organizations/{id}", orgHandlers.DeleteOrganization)
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), requireActiveOrgByID(orgService)).Get("/organizations/{id}/members", orgHandlers.ListPublicMembers)
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), requireActiveOrgByID(orgService)).Get("/organizations/{id}/content", contentHandlers.ListByOrganization)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), requireActiveOrgByID(orgService)).Get("/organizations/{id}/analytics", orgHandlers.GetOrganizationAnalytics)
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), requireActiveOrgByID(orgService)).Get("/organizations/{id}/media", mediaHandlers.ListByOrganization)
			// Org follow/unfollow (Section 31 — universal polymorphic follow)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "follows.toggle")).Post("/organizations/{id}/follow", orgHandlers.ToggleFollow)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "follows.toggle")).Delete("/organizations/{id}/follow", orgHandlers.ToggleFollow)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "organizations.members.manage")).Post("/organizations/{id}/members", orgHandlers.AddMember)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "organizations.members.manage")).Delete("/organizations/{id}/members/{userId}", orgHandlers.RemoveMember)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "organizations.members.manage")).Patch("/organizations/{id}/members/{userId}", orgHandlers.UpdateMemberRole)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "organizations.invite")).Post("/organizations/{id}/invitations", orgHandlers.InviteMember)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/organizations/{id}/invitations", orgHandlers.ListInvitations)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "organizations.invite")).Post("/organizations/invitations/{token}/accept", orgHandlers.AcceptInvitation)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "organizations.claim")).Post("/claim-ownership", orgHandlers.ClaimOwnership)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/claim-ownership", orgHandlers.ListClaims)
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
			// Public ad serving and event tracking (catalog: /ads/serve, /ads/events —
			// §8: reachable by guests, OptionalAuth so a signed-in user's frequency
			// cap still applies; usergroup targeting itself is deferred to P5).
			r.With(auth.OptionalAuth(kr, vkClient.Redis())).
				Get("/ads/serve", adHandlers.RenderSlot)
			r.With(auth.OptionalAuth(kr, vkClient.Redis())).
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

			// Ticket System routes (Section 38 — Ticket System)
			// User portal (catalog: /tickets, /me/tickets)
			r.Get("/ticket-types", ticketHandlers.ListTicketTypes)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Post("/tickets", ticketHandlers.CreateTicket)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/me/tickets", ticketHandlers.ListMyTickets)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/me/tickets/{id}", ticketHandlers.GetMyTicket)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Post("/me/tickets/{id}/comments", ticketHandlers.AddReply)

			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Post("/me/tickets/{id}/attachments", ticketHandlers.AddAttachment)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/me/tickets/{id}/attachments", ticketHandlers.ListAttachments)

			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.admin")).
				Get("/admin/tickets", ticketHandlers.ListTickets)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.admin")).
				Get("/admin/tickets/{id}", ticketHandlers.GetTicket)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.admin")).
				Patch("/admin/tickets/{id}", ticketHandlers.UpdateTicket)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.admin")).
				Post("/admin/tickets/{id}/notes", ticketHandlers.AddInternalNote)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.admin")).
				Get("/admin/tickets/{id}/events", ticketHandlers.GetTicketEvents)
			// Catalog alias: /admin/tickets/:id/comments → same handler as /notes (P5.6 — Task 5)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.admin")).
				Post("/admin/tickets/{id}/comments", ticketHandlers.AddInternalNote)
			// Delete ticket (P5.6 — Task 5 — soft-delete via UpdateTicket status)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.admin")).
				Delete("/admin/tickets/{id}", ticketHandlers.UpdateTicket)
			// Resolve ticket (P5.6 — Task 5 — status transition via UpdateTicket)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.admin")).
				Post("/admin/tickets/{id}/resolve", ticketHandlers.UpdateTicket)

			// Ticket type management
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.type.manage")).
				Get("/admin/ticket-types", ticketHandlers.ListTicketTypes)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.type.manage")).
				Post("/admin/ticket-types", ticketHandlers.CreateTicketType)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.type.manage")).
				Put("/admin/ticket-types/{id}", ticketHandlers.UpdateTicketType)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tickets.type.manage")).
				Delete("/admin/ticket-types/{id}", ticketHandlers.DeleteTicketType)

			// i18n Translation management (admin) — catalog: /admin/i18n/*.
			// NOTE: Upsert/UpsertBatch are MANUAL value-setting handlers, not the
			// catalog's AI-driven POST /admin/i18n/strings/:key/translate or
			// /ai-translate-batch (no AI dispatcher call exists here) — kept under
			// the /strings prefix as documented manual-upsert extras rather than
			// mislabeled as the AI routes, which remain unimplemented (P5).
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

			// i18n public endpoints (no auth) — catalog: /i18n/packs, /i18n/packs/:lang_code
			r.Get("/i18n/packs", i18nHandlers.HandleListPublicLocales)
			r.Get("/i18n/packs/{lang_code}", i18nHandlers.GetLocaleTranslations)

			// GDPR / NDPR Compliance routes (Section 19)
			// User-facing endpoints
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Post("/consent", gdprHandlers.RecordConsent)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/consent", gdprHandlers.GetUserConsents)
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
				Get("/admin/data-exports", gdprHandlers.ListPendingDataExports)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gdpr.deletions.view")).
				Get("/admin/deletions", gdprHandlers.ListPendingDeletions)

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

				r.Get("/public/content/{id}", publicAPIHandlers.GetContentByID)
				r.Get("/public/spaces", publicAPIHandlers.ListSpaces)
				r.Get("/public/spaces/{slug}", publicAPIHandlers.GetSpaceBySlug)
				r.Get("/public/tags", publicAPIHandlers.ListTags)
				r.Get("/public/users/{id}", publicAPIHandlers.GetUserByID)
				r.Get("/public/search", publicAPIHandlers.SearchContent)
				r.With(csrf.EnsureCookie(cfg.CookieDomain, cfg.IsProduction())).Get("/public/me", publicAPIHandlers.GetMyProfile)
			})

				// User Profile & Preferences routes (catalog: /me, /me/profile)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), csrf.EnsureCookie(cfg.CookieDomain, cfg.IsProduction())).Get("/me", usersHandlers.GetMyProfile)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Patch("/me/profile", usersHandlers.UpdateMyProfile)
			// NOTE (§6.3 dedupe): /me/preferences overlaps /me/notification-preferences;
			// kept as a documented extra pending confirmation of distinct semantics.
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/preferences", usersHandlers.GetMyPreferences)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Put("/me/preferences", usersHandlers.UpdateMyPreferences)

			// User /me/ account management routes (Section 50)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/me/change-password", usersHandlers.ChangePassword)
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
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/drafts", usersHandlers.ListMyDrafts)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/drafts/{id}", usersHandlers.GetMyDraft)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/scheduled", usersHandlers.ListMyScheduled)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/bookmarks", usersHandlers.ListMyBookmarks)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/organizations", orgHandlers.ListMyOrganizations)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/bookmark-folders", bookmarkFoldersHandlers.ListFolders)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/me/bookmark-folders", bookmarkFoldersHandlers.CreateFolder)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Patch("/me/bookmark-folders/{id}", bookmarkFoldersHandlers.UpdateFolder)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/me/bookmark-folders/{id}", bookmarkFoldersHandlers.DeleteFolder)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/analytics", usersHandlers.GetMyAnalytics)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/analytics/content", usersHandlers.GetMyContentAnalytics)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/analytics/followers", usersHandlers.GetMyFollowerGrowth)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/me/deactivate", usersHandlers.DeactivateAccount)
			// GDPR deletion + data export — served by the REAL gdpr handlers
			// (queue-backed export via data:export_user_data + Honcho hard erasure),
			// not the users-package stubs. Consolidated here from /account/* (§6).
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/me", gdprHandlers.RequestAccountDeletion)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "moderate")).Post("/me/data-export", gdprHandlers.RequestDataExport)
			// NOTE: replaces the users-package stub that hardcoded "no_pending_export"
			// regardless of actual state (same class of bug as the DELETE /me fix).
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/data-export/status", gdprHandlers.GetMyDataExportStatus)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/data-export/download", gdprHandlers.GetMyDataExportDownload)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/reports", reportsHandlers.ListMyReports)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.users.list")).Get("/users", usersHandlers.ListUsers)
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
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/onboarding/tour/dismiss", onboardingHandlers.DismissTour)

			// Admin onboarding settings
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings.view")).Get("/admin/onboarding/settings", onboardingHandlers.GetAdminSettings)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings")).Put("/admin/onboarding/settings", onboardingHandlers.UpdateAdminSettings)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings.view")).Get("/admin/onboarding/templates", onboardingHandlers.ListTemplates)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings")).Post("/admin/onboarding/templates", onboardingHandlers.CreateTemplate)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings")).Patch("/admin/onboarding/templates/{id}", onboardingHandlers.UpdateTemplate)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.settings")).Delete("/admin/onboarding/templates/{id}", onboardingHandlers.DeleteTemplate)

			// Search routes
			r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "moderate")).Get("/search", searchHandlers.Search)
			// Rate-limit autocomplete to prevent enumeration/abuse (SEC-010).
			r.With(mw.RateLimit(vkClient.Redis(), "moderate")).Get("/search/autocomplete", searchHandlers.Autocomplete)
			// catalog: GET /spaces/search — space-discovery search (distinct from
			// /spaces/{id}/search below, which searches one space's content).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "moderate")).Get("/spaces/search", searchHandlers.SearchSpaces)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/spaces/{id}/search", searchHandlers.SearchSpace)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/tags/{slug}/search", searchHandlers.SearchTag)

			// General Tags (Sections 08, 49) — distinct from forum tags.
			r.Get("/tags", tagsHandlers.ListTags)
			r.Get("/tags/{slug}", tagsHandlers.GetTag)
			r.Get("/tags/{slug}/content", tagsHandlers.GetTagContent)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "moderate")).Post("/tags/{slug}/follow", tagsHandlers.ToggleTagFollow)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "moderate")).Delete("/tags/{slug}/follow", tagsHandlers.ToggleTagFollow)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/tags/{slug}/followers", tagsHandlers.GetTagFollowers)
			// Admin tag CRUD (previously missing entirely — content_tags/
			// AddContentTag was unreachable since there was no way to create a tag).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tags.create")).
				Post("/admin/tags", tagsHandlers.CreateTag)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tags.edit")).
				Patch("/admin/tags/{id}", tagsHandlers.UpdateTag)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tags.delete")).
				Delete("/admin/tags/{id}", tagsHandlers.DeleteTag)
			// Admin tag listing (reuses the tenant-scoped ListTags handler).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tags.edit")).
				Get("/admin/tags", tagsHandlers.ListTags)
			// Tag moderators (Section 49).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tags.edit")).
				Get("/admin/tags/{id}/moderators", tagsHandlers.ListTagModerators)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tags.edit")).
				Post("/admin/tags/{id}/moderators", tagsHandlers.AddTagModerator)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tags.edit")).
				Delete("/admin/tags/{id}/moderators/{userId}", tagsHandlers.RemoveTagModerator)

			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.analytics.search")).
				Get("/admin/search/analytics", searchHandlers.SearchAdminAnalytics)

			// AI Core routes (Section 20 — AI Core)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.generate")).
				Post("/ai/generate", aiHandlers.HandleGenerate)

			// JapaBot chat — gated by feature.ai_chat, moderate rate limit.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.ai_chat"), mw.RateLimit(vkClient.Redis(), "moderate")).
				Post("/ai/chat", aiHandlers.HandleChat)

			// Content AI Assist endpoints (catalog: /ai/assist/*), moderate rate limit.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "moderate")).
				Post("/ai/assist/improve", aiHandlers.HandleImprove)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "moderate")).
				Post("/ai/assist/summarize", aiHandlers.HandleSummarize)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "moderate")).
				Post("/ai/assist/translate", aiHandlers.HandleTranslate)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "moderate")).
				Post("/ai/assist/generate-tags", aiHandlers.HandleGenerateTags)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "moderate")).
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

			// AI Context admin routes — view/delete user AI context via Honcho
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.context.admin_view")).
				Get("/admin/ai/context/{user_id}", aiHandlers.HandleGetUserContext)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.context.admin_delete_entry")).
				Delete("/admin/ai/context/{user_id}/{entry_id}", aiHandlers.HandleDeleteUserContextEntry)

			// AI Usage admin route (stub until ai_usage_log table exists)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.context.admin_view")).
				Get("/admin/ai/usage", aiHandlers.HandleGetAIUsage)
			// AI models admin route — lists available AI models
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.tools.manage")).
				Get("/admin/ai/models", aiHandlers.HandleListModels)

			// AI categories (public — for frontend AI category selection)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/ai/categories", aiHandlers.HandleListCategories)

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
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "integrations.webhooks.manage")).
				Post("/admin/webhooks/{id}/retry/{delivery_id}", webhookHandlers.RetryDelivery)
			// Catalog: PATCH /admin/webhooks/:id (P5.6 — Task 8)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "integrations.webhooks.manage")).
				Patch("/admin/webhooks/{id}", webhookHandlers.UpdateWebhook)
			// Catalog: POST /admin/webhooks/:id/verify (P5.6 — Task 8 — reuses TestWebhook)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "integrations.webhooks.manage")).
				Post("/admin/webhooks/{id}/verify", webhookHandlers.TestWebhook)
			// Catalog: POST /admin/webhooks/:id/deliveries/:delivery_id/retry (P5.6 — Task 8)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "integrations.webhooks.manage")).
				Post("/admin/webhooks/{id}/deliveries/{delivery_id}/retry", webhookHandlers.RetryDelivery)

			// Webhook event types (public — for frontend webhook configuration)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/webhook-events", webhookHandlers.ListWebhookEvents)

			// Newsletter admin routes (Section 33 — Newsletter)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "newsletter.admin")).
				Get("/admin/newsletter/subscribers", newsletterHandlers.ListSubscribers)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "newsletter.admin")).
				Get("/admin/newsletter/subscribers/{id}", newsletterHandlers.GetSubscriber)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "newsletter.admin")).
				Post("/admin/newsletter/subscribers", newsletterHandlers.CreateSubscriber)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "newsletter.admin")).
				Delete("/admin/newsletter/subscribers/{id}", newsletterHandlers.DeleteSubscriber)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "newsletter.admin")).
				Get("/admin/newsletter/campaigns", newsletterHandlers.ListCampaigns)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "newsletter.admin")).
				Get("/admin/newsletter/campaigns/{id}", newsletterHandlers.GetCampaign)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "newsletter.admin")).
				Post("/admin/newsletter/campaigns", newsletterHandlers.CreateCampaign)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "newsletter.admin")).
				Patch("/admin/newsletter/campaigns/{id}", newsletterHandlers.UpdateCampaign)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "newsletter.admin")).
				Delete("/admin/newsletter/campaigns/{id}", newsletterHandlers.DeleteCampaign)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "newsletter.admin")).
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
			r.Get("/affiliate-program", affiliateHandlers.GetProgramInfo)

			// Enrollment (auth required)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Post("/affiliate-program/enroll", affiliateHandlers.Enroll)

			// User's own affiliate links (auth required)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/me/affiliate-links", affiliateHandlers.ListMyLinks)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Post("/me/affiliate-links", affiliateHandlers.CreateMyLink)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/me/affiliate-links/{id}", affiliateHandlers.GetMyLink)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Patch("/me/affiliate-links/{id}", affiliateHandlers.UpdateMyLink)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Delete("/me/affiliate-links/{id}", affiliateHandlers.DeleteMyLink)

			// Earnings summary (auth required)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
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

			// Referral Program admin routes (Section 34 — Referral)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "referral.admin")).
				Get("/admin/referral/program", referralHandlers.GetProgram)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "referral.admin")).
				Put("/admin/referral/program", referralHandlers.UpdateProgram)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "referral.admin")).
				Get("/admin/referral/referrals", referralHandlers.ListReferrals)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "referral.admin")).
				Get("/admin/referral/referrals/{id}", referralHandlers.GetReferral)

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
				Get("/admin/mentorship/matches", mentorshipHandlers.ListMatches)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "mentorship.admin")).
				Patch("/admin/mentorship/matches/{id}", mentorshipHandlers.UpdateMatch)

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
				Get("/admin/docs/pages/{id}", docsHandlers.GetDocPage)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "docs.admin")).
				Post("/admin/docs/pages", docsHandlers.CreateDocPage)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "docs.admin")).
				Patch("/admin/docs/pages/{id}", docsHandlers.UpdateDocPage)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "docs.admin")).
				Delete("/admin/docs/pages/{id}", docsHandlers.DeleteDocPage)
			// Doc page approve/publish (P5.6 — Task 3 — status transitions via UpdateDocPage)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "docs.admin")).
				Post("/admin/docs/pages/{id}/approve", docsHandlers.UpdateDocPage)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "docs.admin")).
				Post("/admin/docs/pages/{id}/publish", docsHandlers.UpdateDocPage)
			// AI doc generation (P5.6 — Task 3 — stub, delegates to ai/generate)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "docs.admin")).
				Post("/admin/docs/ai-generate", aiHandlers.HandleGenerate)

			// Learning Platform routes (Section 36)
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

			// Japa University creator-economy routes (ADR-063 — upgrade to Learn).
			// Instructor lifecycle
			// AUDIT-FRESH-DEAD-NODES: wired university.instructor.apply RBAC gate.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.instructor.apply")).Post("/learn/university/instructor/apply", universityHandlers.Apply)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/learn/university/instructor/me", universityHandlers.GetMyInstructorProfile)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Patch("/learn/university/instructor/me", universityHandlers.UpdateMyInstructorProfile)
			r.Get("/learn/university/instructors/{id}", universityHandlers.GetPublicInstructor)

			// Creator track management
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/learn/university/my/tracks", universityHandlers.ListMyTracks)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/learn/university/my/tracks", universityHandlers.CreateMyTrack)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Patch("/learn/university/my/tracks/{id}", universityHandlers.UpdateMyTrack)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/learn/university/my/tracks/{id}/submit", universityHandlers.SubmitMyTrack)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/learn/university/my/tracks/{id}/stats", universityHandlers.GetMyTrackStats)

			// Marketplace catalog
			r.Get("/learn/university/catalog", universityHandlers.ListCatalog)
			r.Get("/learn/university/catalog/{slug}", universityHandlers.GetCatalogTrack)

			// Earnings & payouts
			// AUDIT-FRESH-DEAD-NODES: wired university.payout.view_own RBAC gate.
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/learn/university/my/earnings", universityHandlers.GetMyEarnings)
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
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.course.approve")).
				Get("/admin/university/tracks/pending", universityHandlers.AdminListPendingTracks)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.course.approve")).
				Post("/admin/university/tracks/{id}/approve", universityHandlers.AdminApproveTrack)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.payout.process")).
				Get("/admin/university/payouts", universityHandlers.AdminListPayouts)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "university.payout.process")).
				Post("/admin/university/payouts/{id}/process", universityHandlers.AdminProcessPayout)

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

			// Public page-templates endpoints (read-only, no auth required)
			r.Get("/page-templates", pagebuilderHandlers.ListTemplates)
			r.Get("/page-templates/{id}", pagebuilderHandlers.GetTemplate)

			// Admin widgets CRUD (P5.6 — Task 27, catalog: /admin/widgets)
			r.Group(func(r chi.Router) {
				r.Use(auth.RequireAuth(kr, vkClient.Redis()))
				r.Use(rbac.RequirePermission(rbacResolver, "pagebuilder.blocks.manage"))
				r.Get("/admin/widgets", pagebuilderHandlers.ListBlocks)
				r.Post("/admin/widgets", pagebuilderHandlers.CreateBlock)
				r.Patch("/admin/widgets/{id}", pagebuilderHandlers.UpdateBlock)
				r.Delete("/admin/widgets/{id}", pagebuilderHandlers.DeleteBlock)
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
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.rbac.manage")).
				Post("/admin/user-groups/{id}/members", rbacHandlers.AddGroupMember)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.rbac.manage")).
				Delete("/admin/user-groups/{id}/members/{userId}", rbacHandlers.RemoveGroupMember)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.rbac.manage")).
				Get("/admin/permissions", rbacHandlers.ListPermissions)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.rbac.manage")).
				Get("/admin/roles", rbacHandlers.ListRoles)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.rbac.manage")).
				Post("/admin/roles", rbacHandlers.CreateRole)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.rbac.manage")).
				Get("/admin/roles/{id}/permissions", rbacHandlers.ListRolePermissions)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.rbac.manage")).
				Post("/admin/roles/{id}/permissions", rbacHandlers.AssignPermissionToRole)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.rbac.manage")).
				Get("/admin/user-groups/{id}/roles", rbacHandlers.ListGroupRoles)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.rbac.manage")).
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

			// Admin Broadcast route
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.broadcast")).
				Post("/admin/broadcasts", adminHandlers.Broadcast)

			// ═══════════════════════════════════════════════════════════
			// P5.6 — Batch route wiring for CATALOG_ONLY routes
			// ═══════════════════════════════════════════════════════════

			// Task 4: Notifications admin — notification type CRUD
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "notifications.manage")).
				Get("/admin/notification-types", notificationHandlers.ListNotificationTypes)
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
				Post("/spaces/{id}/pinned/reorder", spaceHandlers.ReorderPinned)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spaces.moderate")).
				Patch("/spaces/{id}/pinned/reorder", spaceHandlers.ReorderPinned)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/spaces/suggest-parents", spaceHandlers.SuggestParentSpaces)

			// Task 13: Professional profiles admin + public extras
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Post("/admin/profiles/{id}/verify", professionalHandlers.VerifyProfile)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Delete("/admin/profiles/{id}/verify", professionalHandlers.UnverifyProfile)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/profiles/{id}/content", professionalHandlers.ListProfileContent)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.member.manage")).
				Post("/profiles/{id}/members/invite", professionalHandlers.InviteMember)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/profiles/invitations/accept", professionalHandlers.AcceptProfileInvitation)

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
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Post("/me/subscription/reactivate", paymentHandlers.ReactivateSubscription)

			// Task 16: Advertising admin
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.campaign.manage")).
				Patch("/admin/ads/campaigns/{id}", adHandlers.UpdateCampaign)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ad.slot.manage")).
				Get("/admin/ads/slot-types/{id}", adHandlers.GetSlotType)

			// Task 17: Wallet admin
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wallet.admin.rules.manage")).
				Patch("/admin/wallet/rules/{id}", walletHandlers.CreateOrUpdateRule)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wallet.admin.adjust")).
				Post("/admin/wallet/award", walletHandlers.AdminAdjust)

			// Task 18: Blog review queue
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.review.manage")).
				Get("/admin/blog/review-queue", blogHandlers.ListReviews)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.review.manage")).
				Post("/admin/blog/review-queue/{id}/approve", blogHandlers.UpdateReview)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.review.manage")).
				Post("/admin/blog/review-queue/{id}/reject", blogHandlers.UpdateReview)
			// Admin blog post management (Section 24 blog extension).
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.review.manage")).
				Get("/admin/blog", blogHandlers.AdminListPosts)
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
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gdpr.data_exports.view")).
				Get("/admin/gdpr/exports", gdprHandlers.ListPendingDataExports)

			// Task 21: Users admin moderation
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.users.list")).
				Get("/admin/users/{id}/ai-context", aiHandlers.HandleGetUserContext)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.users.list")).
				Delete("/admin/users/{id}/ai-context/{entryId}", aiHandlers.HandleDeleteUserContextEntry)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Post("/admin/users/{id}/ban", adminHandlers.BanUser)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Delete("/admin/users/{id}/ban", adminHandlers.UnbanUser)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Get("/admin/users/{id}/bans", adminHandlers.ListUserBans)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.rbac.manage")).
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
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Post("/admin/users/{id}/restrict", adminHandlers.RestrictUser)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Delete("/admin/users/{id}/restrict/{operation}", adminHandlers.LiftRestriction)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
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
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.admin.manage")).
				Get("/admin/affiliate-program/commission-rates", affiliateHandlers.GetCommissionRates)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.admin.manage")).
				Patch("/admin/affiliate-program/commission-rates", affiliateHandlers.UpdateCommissionRates)

			// Affiliate enhancements: author revenue sharing + unregistered link detection
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.admin.manage")).
				Get("/admin/affiliate/scan-results", couponHandlers.ListAffiliateScanResults)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.admin.manage")).
				Patch("/admin/affiliate/scan-results/{id}", couponHandlers.ReviewAffiliateScanResult)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "affiliate.admin.manage")).
				Get("/admin/affiliate/commissions", couponHandlers.ListAffiliateCommissions)
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
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "docs.admin")).
				Get("/admin/seo/validate/{id}", sitemapHandler.ValidateSEO)

			// Task 27: Pagebuilder admin extra + public pages
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pages.manage")).
				Post("/admin/pages/{id}/save-as-template", pagebuilderHandlers.SaveAsTemplate)
			r.Get("/pages", pagebuilderHandlers.ListPublishedPages)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pages.manage")).
				Post("/pages", pagebuilderHandlers.CreatePage)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pages.manage")).
				Patch("/pages/{id}", pagebuilderHandlers.UpdatePage)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pages.manage")).
				Delete("/pages/{id}", pagebuilderHandlers.DeletePage)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "pages.manage")).
				Patch("/pages/{id}/blocks", pagebuilderHandlers.ReorderBlocks)
			r.Get("/widgets/{id}", pagebuilderHandlers.GetBlock)

			// Task 28: Me additions
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Patch("/me/account", usersHandlers.UpdateMyProfile)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "api.keys.self_manage")).
				Get("/me/api-keys/{id}", publicAPIHandlers.GetAPIKey)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "api.keys.self_manage")).
				Patch("/me/api-keys/{id}", publicAPIHandlers.UpdateAPIKey)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Delete("/me/push-tokens/{id}", notificationHandlers.HandlePushUnsubscribe)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/me/spaces/owned", spaceHandlers.ListMySpaces)

			// Task 29: AI conversations
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/ai/conversations", aiHandlers.HandleListConversations)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/ai/conversations/{id}", aiHandlers.HandleGetConversation)

			// Task 30: DM requests
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/dm/requests", wsHandlers.ListMessageRequests)

			// Task 32: Forum extras
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.thread.solve")).
				Delete("/forum/{id}/solve", forumHandlers.SolveThread)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.vote")).
				Delete("/forum/{id}/vote", forumHandlers.Vote)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.tag.create")).
				Post("/forum/{id}/tags", forumHandlers.AddThreadTag)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.vote")).
				Post("/forum/{id}/vote", forumHandlers.Vote)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.poll.create")).
				Post("/forum/{id}/poll", forumHandlers.CreatePoll)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.vote")).
				Post("/poll-options/{id}/vote", forumHandlers.VotePoll)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.vote")).
				Delete("/poll-options/{id}/vote", forumHandlers.VotePoll)

			// Task 33: Content public extras
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/content/{id}/analytics", analyticsHandlers.GetContentAnalytics)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/content/{id}/discussions", commentsHandlers.ListComments)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Post("/content/{id}/discussions", commentsHandlers.CreateComment)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.reactions.add")).
				Post("/content/{id}/react", contentHandlers.AddReaction)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.reactions.remove")).
				Delete("/content/{id}/react", contentHandlers.RemoveReaction)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.pin")).
				Patch("/content/{id}/spaces", contentHandlers.UpdateContent)

			// Task 34: Users public extras
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/users/{id}/analytics", usersHandlers.GetMyAnalytics)
			// Public profile Posts tab — author-scoped published+public content (was
			// wired to ListContent, which ignored {id} and returned all tenant content).
			// Public per Section 32; gated to 404 non-active users.
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), requireActiveProfileUser(usersService, "id")).
				Get("/users/{id}/content", contentHandlers.ListByAuthor)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "follows.toggle")).
				Post("/users/{id}/follow", followHandlers.ToggleFollow)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "follows.toggle")).
				Delete("/users/{id}/follow", followHandlers.ToggleFollow)

			// Task 35: Suggestions/content (trending)
			r.With(auth.OptionalAuth(kr, vkClient.Redis())).
				Get("/suggestions/content", trendingHandlers.GetTrendingContent)

			// Task 11: Universities compare
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Get("/universities/{id}/compare", universitiesHandlers.CompareUniversities)
		})
	})

	// Payment webhook routes — inside /api/v1 with a tighter rate limit.
	// Payment providers (Paystack, Stripe, NOWPayments) can send
	// high-frequency webhook traffic.
	// NOTE: These are registered inside the existing /api/v1 group above.

	// Super-admin Control Panel routes (Section 21).
	// Tenant-scoped, requires auth + control.panel.access permission.
	r.Group(func(r chi.Router) {
		r.Use(tenant.Middleware(tenantStore))
		r.Use(auth.RequireAuth(kr, vkClient.Redis()))
		r.Use(rbac.RequirePermission(rbacResolver, "control.panel.access"))
		// Audit finding C1: enforce double-submit CSRF on all mutating /control/* requests.
		// NOTE: the /control group is mounted at the TOP LEVEL (r.Route("/control", ...)
		// outside the /api/v1 group), so real request paths are "/control/...", NOT
		// "/api/v1/control/...". The prefix MUST be "/control/" or protection is silently
		// never applied.
		r.Use(csrf.Middleware([]string{"/control/"}, csrf.IsNonCookieAuthenticated))
		// Audit finding M1: (re)issue the csrf_token cookie for control-panel
		// requests so the SPA always has a valid double-submit token available.
		r.Use(csrf.EnsureCookie(cfg.CookieDomain, cfg.IsProduction()))

		r.Route("/control", func(r chi.Router) {
			// Tenants
			r.Get("/tenants", controlHandlers.ListTenants)
			r.Post("/tenants", controlHandlers.CreateTenant)
			r.Get("/tenants/{id}", controlHandlers.GetTenant)
			r.Patch("/tenants/{id}", controlHandlers.UpdateTenant)
			r.Delete("/tenants/{id}", controlHandlers.DeleteTenant)
			r.Post("/tenants/{id}/suspend", controlHandlers.SuspendTenant)
			r.Post("/tenants/{id}/unsuspend", controlHandlers.UnsuspendTenant)

			// Per-tenant Settings
			r.Get("/tenants/{id}/settings", controlHandlers.GetTenantSettings)
			r.Patch("/tenants/{id}/settings", controlHandlers.UpdateTenantSettings)
			r.Patch("/tenants/{id}/settings/{key}", controlHandlers.UpdateTenantSettingKey)
			r.Post("/tenants/{id}/settings/{key}/reset", controlHandlers.ResetTenantSetting)

			// System Settings
			r.Get("/system-settings", controlHandlers.ListSystemSettings)
			r.Patch("/system-settings", controlHandlers.UpdateSystemSettings)
			r.Patch("/system-settings/{key}", controlHandlers.UpdateSystemSetting)
			r.Post("/system-settings/{key}/reset", controlHandlers.ResetSystemSetting)

			// Feature Flags
			r.Get("/feature-flags", controlHandlers.ListFeatureFlags)
			r.Patch("/feature-flags/{flag_name}", controlHandlers.SetFeatureFlag)

			// Infrastructure Health
			r.Get("/health", controlHandlers.HealthCheck)
			r.Get("/health/db", controlHandlers.DBHealthCheck)
			r.Post("/health/db/vacuum", controlHandlers.DBVacuum)

			// Queue Monitor
			r.Get("/queue/stats", controlHandlers.QueueStats)
			r.Get("/queue/{queue_name}/tasks", controlHandlers.ListQueueTasks)
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
			r.Get("/api-keys", publicAPIHandlers.ListAllKeys)
			r.Post("/api-keys", controlHandlers.CreateAPIKey)
			r.Post("/api-keys/batch-delete", controlHandlers.DeleteAPIKeysBatch)
			r.Get("/api-keys/{id}", controlHandlers.GetAPIKey)
			r.Patch("/api-keys/{id}", controlHandlers.UpdateAPIKey)
			r.Delete("/api-keys/{id}", controlHandlers.DeleteAPIKey)
			r.Post("/api-keys/{id}/suspend", publicAPIHandlers.SuspendKey)
			r.Delete("/api-keys/{id}/suspend", publicAPIHandlers.UnsuspendKey)
			r.Get("/api-keys/analytics", controlHandlers.GetAPIKeyAnalytics)

			// AI Budget
			r.Get("/ai-budget", controlHandlers.GetAIBudget)
			r.Patch("/ai-budget", controlHandlers.UpdateAIBudget)

			// Audit Log (full cross-tenant view)
			r.Get("/audit-logs", controlHandlers.ListAuditLogs)
			r.Delete("/audit-logs/{id}", controlHandlers.DeleteAuditLog)
			r.Post("/audit-logs/clear", controlHandlers.ClearAuditLogs)

			// Task types catalog (drives cron job editor type dropdown)
			r.Get("/task-types", controlHandlers.ListTaskTypes)

			// API scopes catalog (drives API key scope editor dropdown)
			r.Get("/api-scopes", rbacHandlers.ListScopes)

			// Deployments
			r.Get("/deployments", controlHandlers.ListDeployments)
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

// commentNotificationAdapter adapts notifications.Service.DispatchNotification
// (returns DispatchResult) to the comments.NotificationDispatcher and
// content.MentionNotifier interfaces (both return only error). The
// DispatchResult is discarded — comment/mention notifications are
// fire-and-forget per Rule 8.
type commentNotificationAdapter struct {
	svc *notifications.Service
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
