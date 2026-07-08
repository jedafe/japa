package server

import (
	"context"
	"crypto/rand"
	"log/slog"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/admin"
	"github.com/jedafe/japa/internal/advertising"
	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/analytics"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/blog"
	"github.com/jedafe/japa/internal/bots"
	"github.com/jedafe/japa/internal/comparison"
	"github.com/jedafe/japa/internal/comments"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/coupons"
	"github.com/jedafe/japa/internal/affiliate"
	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/docs"
	errtracking "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/features"
	"github.com/jedafe/japa/internal/feed"
	"github.com/jedafe/japa/internal/follows"
	"github.com/jedafe/japa/internal/forum"
	"github.com/jedafe/japa/internal/interactions"
	"github.com/jedafe/japa/internal/gdpr"
	"github.com/jedafe/japa/internal/geoip"
	"github.com/jedafe/japa/internal/honcho"
	"github.com/jedafe/japa/internal/i18n"
	"github.com/jedafe/japa/internal/impersonation"
	"github.com/jedafe/japa/internal/learning"
	"github.com/jedafe/japa/internal/linkpreview"
	"github.com/jedafe/japa/internal/media"
	"github.com/jedafe/japa/internal/mentorship"
	mw "github.com/jedafe/japa/internal/middleware"
	"github.com/jedafe/japa/internal/newsletter"
	"github.com/jedafe/japa/internal/notifications"
	"github.com/jedafe/japa/internal/onboarding"
	"github.com/jedafe/japa/internal/pagebuilder"
	"github.com/jedafe/japa/internal/payments"
	"github.com/jedafe/japa/internal/organizations"
	"github.com/jedafe/japa/internal/professional"
	"github.com/jedafe/japa/internal/publicapi"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/jedafe/japa/internal/referral"
	"github.com/jedafe/japa/internal/renderer"
	"github.com/jedafe/japa/internal/reviews"
	rsspkg "github.com/jedafe/japa/internal/rss"
	"github.com/jedafe/japa/internal/search"
	"github.com/jedafe/japa/internal/seo"
	"github.com/jedafe/japa/internal/spaces"
	"github.com/jedafe/japa/internal/spam"
	"github.com/jedafe/japa/internal/tags"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/tickets"
	"github.com/jedafe/japa/internal/users"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/wallet"
	"github.com/jedafe/japa/internal/webhooks"
	"github.com/jedafe/japa/internal/websocket"
	"github.com/jedafe/japa/internal/trending"
	"github.com/jedafe/japa/internal/universities"
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
	_ = spamPipeline // passed to handlers that need content safety checks

	// Auth handlers and DB store
	authStore := auth.NewDBStore(database)
	bruteForce := auth.NewValkeyBruteForceStore(vkClient.Redis())
	authHandlers := &auth.AuthHandlers{
		DB:          authStore,
		KeyRotation: kr,
		BruteForce:  bruteForce,
		Policy:      auth.DefaultPasswordPolicy(),
		VKClient:    vkClient,
		SiteName:    cfg.SiteName,
		SupportURL:  cfg.SiteURL + "/support",
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
	oauthHandlers := &auth.OAuthHandlers{
		Config: oauthCfg,
		Store:  oauthStore,
		KeyRot: kr,
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

	// Bootstrap permission keys and the super_admin role at startup. All
	// operations are idempotent; failures are logged but never crash the
	// server so a temporarily unhealthy database does not prevent boot.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	bootstrapPermissions(ctx, database, rbacStore)

	// Custom field and form subsystem (Section 06).
	cfService := customfields.NewService(database)
	embedResolver := customfields.NewEmbedResolver()
	cfHandlers := customfields.NewHandlers(cfService, embedResolver)

	// Spaces subsystem (Section 05 — Geo-First Space System).
	spaceService := spaces.NewService(database)
	spaceUnread := spaces.NewUnreadCounter(vkClient)
	spaceHandlers := spaces.NewHandlers(spaceService, spaceUnread)

	// Geo HTTP handlers — /api/v1/geo/*. The hierarchy endpoints reuse
	// spaceService (countries/states/cities are seeded as spaces).
	geoHandlers := geoip.NewHandlers(geoipReader, spaceService)

	// Universities subsystem (Section 39) — read-view over spaces of the
	// University space-type + their custom field values, no dedicated table.
	universitiesHandlers := universities.NewHandlers(spaceService, cfService)

	// Content subsystem (Section 14 — Content Type System).
	contentService := content.NewService(database)
	contentHandlers := content.NewHandlers(contentService)

	// Renderer subsystem (Section 15 — Block Editor SSR).
	rendererInstance := renderer.New("scripts/tiptap-ssr.ts", vkClient)
	rendererHandlers := renderer.NewHandlers(rendererInstance)

	// Link preview subsystem (Section 15 — Link Preview Fetch).
	linkPreviewFetcher := linkpreview.NewFetcher(nil, vkClient)
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

	// Comments subsystem (Section 49 — Unified Comments).
	commentsService := comments.NewService(database)
	commentsHandlers := comments.NewHandlers(commentsService)

	// Interactions subsystem (Sections 41, 52 — Blocks & Mutes).
	interactionsService := interactions.NewService(database)
	interactionsHandlers := interactions.NewHandlers(interactionsService)

	// Create Asynq client for enqueuing background tasks (push delivery, etc.)
	asynqClient := worker.NewClient(worker.DefaultConfig())
	// Shared adapter so feature services enqueue real tasks instead of the
	// per-package noopEnqueuer that silently drops them (AUDIT-BE-001).
	taskEnqueuer := worker.NewEnqueuer(asynqClient)
	// Hot-reloadable system settings (AUDIT-BE-009). Previously config.NewSettings
	// was never instantiated, so documented operational limits were dead and
	// services used compiled-in constants. Instantiate once and inject where limits
	// are read.
	settingsService := config.NewSettings(database, vkClient.Redis())
	worker.RegisterSettings(settingsService)
	// Inject into services with hot-reloadable moderation/ops limits (AUDIT-BE-009).
	spamPipeline.SetSettings(settingsService)
	notificationService.SetAsynqClient(asynqClient)
	authHandlers.AsynqClient = asynqClient
	contentHandlers.AsynqClient = asynqClient

	// Feed System subsystem (Section 32 — Activity Feed / Timeline).
	feedService := feed.NewService(database)
	feedHandlers := feed.NewHandlers(feedService)

	// Trending & Discovery subsystem (Phase 28).
	trendingStore := trending.NewStore(database)
	trendingHandlers := trending.NewHandlers(trendingStore)

	// Register trending processor with worker for background tasks.
	trendingProc := trending.NewProcessor(database, slog.Default())
	trendingAdapter := trending.NewWorkerAdapter(trendingProc)
	worker.RegisterTrendingProcessor(trendingAdapter)

	// Blog Extension subsystem (Section 24 — Blog Extension).
	blogService := blog.NewService(database)
	blogHandlers := blog.NewHandlers(blogService)

	// Reviews subsystem (Section 29 — Universal Ratings & Reviews).
	reviewService := reviews.NewService(database)
	reviewHandlers := reviews.NewHandlers(reviewService)

	// Comparison subsystem (Section 30 — Generic Comparison System).
	comparisonService := comparison.NewService(database)
	comparisonHandlers := comparison.NewHandlers(comparisonService)

	// Professional Profiles subsystem (Section 33 — Professional Profiles).
	professionalService := professional.NewService(database)
	professionalHandlers := professional.NewHandlers(professionalService, rbacResolver)

	// Organization system (Section 07).
	orgService := organizations.NewService(database)
	orgHandlers := organizations.NewHandlers(orgService)

	// Wallet & Credits subsystem (Section 28 — Wallet & Credits).
	walletService := wallet.NewService(database)
	walletHandlers := wallet.NewHandlers(walletService)

	// Payment Core subsystem (Section 17 — Payment Core).
	paymentService := payments.NewService(database, &walletCreditAdapter{svc: walletService})
	paymentHandlers := payments.NewHandlers(paymentService)
	paymentHandlers.StripeWebhookSecret = cfg.StripeWebhookSecret
	paymentHandlers.NOWPaymentsIPNKey = cfg.NOWPaymentsIPNSecret

	// Advertising subsystem (Section 25 — Ad Placement).
	adService := advertising.NewService(database)
	adHandlers := advertising.NewHandlers(adService)

	// Ticket System subsystem (Section 38 — Ticket System).
	ticketService := tickets.NewServiceWithEnqueuer(database, taskEnqueuer)
	ticketHandlers := tickets.NewHandlers(ticketService)

	// Coupons & External Affiliate Providers subsystem (Section 55).
	couponService := coupons.NewServiceWithEnqueuer(database, taskEnqueuer)
	couponHandlers := coupons.NewHandlers(couponService)

	// User Affiliate Program subsystem (Section 56).
	affiliateService := affiliate.NewServiceWithEnqueuer(database, taskEnqueuer)
	affiliateHandlers := affiliate.NewHandlers(affiliateService)

	// i18n Translation System.
	i18nService := i18n.NewService(database)
	i18nHandlers := i18n.NewHandlers(i18nService)

	// GDPR / NDPR Compliance subsystem (Section 19).
	gdprService := gdpr.NewServiceWithEnqueuer(database, taskEnqueuer)
	gdprService.SetHonchoEraser(honcho.NewClient(cfg.HonchoURL))
	gdprHandlers := gdpr.NewHandlers(gdprService)

	// Register GDPR processor with worker for background tasks.
	gdprAdapter := gdpr.NewWorkerAdapter(gdprService)
	worker.RegisterGDPRProcessor(gdprAdapter)

	// Media subsystem (Section 34 — Media System).
	mediaService := media.NewService(database)

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
	webhookService := webhooks.NewService(database)
	webhookService.SetSettings(settingsService)
	webhookHandlers := webhooks.NewHandlers(webhookService)
	webhookAdapter := webhooks.NewWorkerAdapter(webhookService)
	worker.RegisterWebhookProcessor(webhookAdapter)

	// Push Delivery — registers the VAPID push delivery implementation
	// so the Asynq push_delivery handler can send web push notifications.
	worker.RegisterPushDelivery(notificationService.PushDelivery())

	// Documentation System subsystem (Section 44).
	docsService := docs.NewService(database)
	docsHandlers := docs.NewHandlers(docsService)

	// Admin Settings subsystem.
	adminHandlers := admin.NewHandlers(database)

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

	// Register no-op implementations for any processor interface that hasn't
	// been wired to a real service yet. This ensures all Asynq task handlers
	// degrade gracefully instead of crashing on a nil interface.
	worker.RegisterNoopProcessors()

	// Admin Impersonation subsystem (Section 46).
	impersonationStore := impersonation.NewStore(vkClient.Redis())
	impersonationService := impersonation.NewService(impersonationStore, database.Writer)
	impersonationHandlers := impersonation.NewHandlers(impersonationService)

	// Public API & Developer Portal subsystem (Section 47).
	publicAPIStore := publicapi.NewStore(database.Writer)
	publicAPIService := publicapi.NewService(publicAPIStore)
	publicAPIHandlers := publicapi.NewHandlers(publicAPIService)

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
	onboardingService := onboarding.NewService(database)
	onboardingHandlers := onboarding.NewHandlers(onboardingService)

	// SEO & Performance subsystem (Section 26).
	seoQueries := gen.New(database.Writer)
	sitemapHandler := seo.NewSitemapHandler(seoQueries, database.Reader)
	ogImageHandler := seo.NewOGImageHandler(database.Reader)

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
		r.Get("/og/{id}.png", ogImageHandler.ServeOGImage)
		// Universal RSS feeds (public — no auth required, tenant-scoped)
		r.Get("/feed.xml", rsspkg.HandleFeed(database.Reader, "all"))
		r.Get("/blog/feed.xml", rsspkg.HandleFeed(database.Reader, "blog"))
		r.Get("/spaces/{slug}/feed.xml", rsspkg.HandleSpaceFeed(database.Reader))
		r.Get("/tags/{slug}/feed.xml", rsspkg.HandleTagFeed(database.Reader))
		r.Get("/users/{username}/feed.xml", rsspkg.HandleUserFeed(database.Reader))

		// API Playground & OpenAPI Spec (Section 44 — docs.japaconnect.com)
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

			// Bot detection middleware — identifies known bots from User-Agent
			// and stores bot info in request context for downstream handlers.
			// Runs after tenant middleware (needs tenant context) but before
			// route handlers.
			r.Use(bots.BotDetectionMiddleware(botStore))

			// Auth routes (public - no auth required, tenant-scoped)
			// Tight rate limit to throttle brute-force attempts.
			r.Route("/auth", func(r chi.Router) {
				r.Use(mw.RateLimit(vkClient.Redis(), "tight"))
				r.Post("/register", authHandlers.Register)
				r.Post("/login", authHandlers.Login)
				r.Post("/refresh", authHandlers.Refresh)
				r.Post("/logout", authHandlers.Logout)

				// Password reset flow
				r.Post("/forgot-password", authHandlers.HandleForgotPassword)
				r.Get("/reset-password", authHandlers.HandleResetPasswordPage)
				r.Post("/reset-password", authHandlers.HandleResetPassword)

				// Email verification
				r.Get("/verify-email", authHandlers.HandleVerifyEmail)
				r.Post("/resend-verification", authHandlers.HandleResendVerification)

				// OAuth social login routes.
				// DEFERRED (§8): catalog specifies POST /auth/oauth/:provider, but
				// HandleInitiate issues a 307 redirect and the frontend uses <a href>
				// GET links. Converting to POST requires the handler to return the
				// auth URL as JSON + frontend rework + full OAuth-flow retest. Left as
				// GET initiate until that change is made and verified.
				r.Get("/oauth/{provider}/initiate", oauthHandlers.HandleInitiate)
				r.Get("/oauth/{provider}/callback", oauthHandlers.HandleCallback)
			})

			// Feature flags endpoint (public - frontend needs flags to
			// render navigation, no auth required)
			featureHandlers := features.NewHandlers(featureChecker)
			r.Get("/feature-flags", featureHandlers.HandleGetFeatures)

			// Feature flags admin toggle (requires auth + RBAC permission)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "features.manage")).
				Patch("/admin/feature-flags/{flag_name}", featureHandlers.HandleSetFeatureFlag)

			// Custom field group routes
			// Read (require auth)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/custom-field-groups", cfHandlers.ListFieldGroups)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/custom-field-groups/{id}", cfHandlers.GetFieldGroup)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/custom-field-groups/{id}/fields", cfHandlers.ListFieldsByGroup)
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
			// Submission listing (require auth + permission)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forms.submissions.view")).
				Get("/forms/{id}/submissions", cfHandlers.ListFormSubmissions)

			// Space type routes
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/space-types", spaceHandlers.ListSpaceTypes)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/space-types/{id}", spaceHandlers.GetSpaceType)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "space_types.create")).
				Post("/space-types", spaceHandlers.CreateSpaceType)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "space_types.edit")).
				Patch("/space-types/{id}", spaceHandlers.UpdateSpaceType)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "space_types.delete")).
				Delete("/space-types/{id}", spaceHandlers.DeleteSpaceType)

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
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService), rbac.RequirePermissionWithInvalidation(rbacResolver, "spaces.moderate.assigned")).
				Post("/spaces/{slug}/moderators", spaceHandlers.AssignModerator)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), spaces.SpaceResolver(spaceService), rbac.RequirePermissionWithInvalidation(rbacResolver, "spaces.moderate.assigned")).
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
			// Update/Delete (require auth + permission)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.edit")).
				Patch("/content/{id}", contentHandlers.UpdateContent)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.delete")).
				Delete("/content/{id}", contentHandlers.DeleteContent)
			// Revisions (require auth + permission to view)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.revisions.view")).
				Get("/content/{id}/revisions", contentHandlers.ListRevisions)
			// Pin/Unpin (require auth + content.pin permission)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.pin")).
				Post("/content/{id}/pin", contentHandlers.PinContent)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.pin")).
				Delete("/content/{id}/pin", contentHandlers.UnpinContent)
			// Reactions (require auth — any authenticated user can react)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/content/{id}/reactions", contentHandlers.AddReaction)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/content/{id}/reactions/{key}", contentHandlers.RemoveReaction)
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
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/content/{id}/bookmark", contentHandlers.AddBookmark)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/content/{id}/bookmark", contentHandlers.RemoveBookmark)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/content/{id}/bookmark", contentHandlers.GetBookmarkStatus)

			// Content Tags (Section 14 — Content System API)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/content/{id}/tags", contentHandlers.ListContentTags)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.edit")).
				Post("/content/{id}/tags", contentHandlers.AddContentTag)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.edit")).
				Delete("/content/{id}/tags/{tagId}", contentHandlers.RemoveContentTag)

			// Content Shares (Section 14 — Content System API)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/content/{id}/share", contentHandlers.ShareContent)

			// Related Content (Section 14 — Content System API, pgvector cosine similarity)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/content/{id}/related", contentHandlers.GetRelatedContent)

			// Comments (Section 49 — Unified Comment System)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/content/{id}/comments", commentsHandlers.ListComments)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/content/{id}/comments", commentsHandlers.CreateComment)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Patch("/comments/{id}", commentsHandlers.UpdateComment)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/comments/{id}", commentsHandlers.DeleteComment)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/comments/{id}/vote", commentsHandlers.VoteComment)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/comments/{id}/vote", commentsHandlers.RemoveVote)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.pin")).
				Post("/comments/{id}/pin", commentsHandlers.PinComment)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.pin")).
				Delete("/comments/{id}/pin", commentsHandlers.UnpinComment)

			// Follow System routes (Section 31 — Follow System)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/follows", followHandlers.ToggleFollow)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/following", followHandlers.ListMyFollowing)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/followed-tags", followHandlers.ListMyFollowedTags)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/users/{user_id}/followers", followHandlers.GetFollowers)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/users/{user_id}/following", followHandlers.GetFollowing)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/users/{user_id}/mutual/{other_user_id}", followHandlers.CheckMutual)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/users/{user_id}/follow-status", followHandlers.GetFollowStatus)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/spaces/{space_id}/followers", followHandlers.GetFollowers)

			// User Blocks (Section 41 — user-facing, non-admin)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/users/{user_id}/block", interactionsHandlers.ToggleBlock)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/users/{user_id}/block", interactionsHandlers.ToggleBlock)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/blocked-users", interactionsHandlers.ListBlocked)

			// User Mutes (Section 52)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/users/{user_id}/mute", interactionsHandlers.MuteUser)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/users/{user_id}/mute", interactionsHandlers.UnmuteUser)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/users/{user_id}/mute-status", interactionsHandlers.GetUserMuteStatus)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/muted-users", interactionsHandlers.ListMutedUsers)

			// Space Mutes (Section 52)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/spaces/{space_id}/mute", interactionsHandlers.MuteSpace)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/spaces/{space_id}/mute", interactionsHandlers.UnmuteSpace)
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
			r.Get("/explore", feedHandlers.GetExploreFeed) // public — no auth required (catalog: /explore)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/feed", feedHandlers.GetPrimaryFeed) // catalog: /feed (primary timeline)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/feed/space/{space_id}", feedHandlers.GetSpaceFeed)
			// catalog: GET /spaces/:id/feed — same handler, param name matches (space_id).
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/spaces/{space_id}/feed", feedHandlers.GetSpaceFeed)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/feed/profile/{user_id}", feedHandlers.GetProfileFeed)

			// Trending & Discovery routes (Phase 28)
			// Public trending endpoints (no auth required)
			r.Get("/trending/content", trendingHandlers.GetTrendingContent)
			r.Get("/trending/tags", trendingHandlers.GetTrendingTags)
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
			// NOTE (P5): GET /universities/:id/compare deferred — the existing
			// comparison system is content-authored (pre-defined items/dimensions
			// attached to a comparison article), not ad-hoc entity-by-ID comparison.
			// Needs a product decision on which "compare" behavior is meant before
			// wiring: show existing authored comparisons mentioning this university,
			// or dynamically diff custom-field values between universities.

			// WebSocket + DM routes (Section 10 — Real-Time WebSockets)
			wsHub := websocket.NewHub()
			// PubSub bridge for multi-instance WebSocket scaling.
			wsBridge := websocket.NewPubSubBridge(vkClient.Redis(), wsHub)
			wsHub.SetPubSub(wsBridge)

			go wsBridge.Start(context.Background())

			wsService := websocket.NewService(database, wsHub)
			wsDMStore := websocket.NewDMStore(gen.New(database.Reader), gen.New(database.Writer))
			wsDMService := websocket.NewDMService(wsDMStore, wsHub)
			wsHandlers := websocket.NewHandlers(wsService, wsDMService, wsHub, vkClient, kr)

			// WebSocket + DM routes — gated by feature.dm (Section 10)
			r.With(features.Gate(featureChecker, "feature.dm")).Group(func(r chi.Router) {
				// WebSocket — tight rate limit on upgrade to prevent abuse
				// (10 connections per minute per IP, per ADR-022).
				r.With(mw.RateLimit(vkClient.Redis(), "tight")).Get("/ws", wsHandlers.HandleWebSocket)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/auth/ws-ticket", wsHandlers.GetWSTicket)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/ws/online", wsHandlers.GetOnlineUsers)

				// Direct Messaging
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/dm/conversations", wsHandlers.ListConversations)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/dm/conversations", wsHandlers.InitiateConversation)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/dm/conversations/{id}/messages", wsHandlers.ListMessages)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/dm/conversations/{id}/messages", wsHandlers.SendMessage)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Patch("/dm/conversations/{id}/messages/{mid}", wsHandlers.EditMessage)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/dm/conversations/{id}/messages/{mid}", wsHandlers.DeleteMessage)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/dm/conversations/{id}/read", wsHandlers.MarkAsRead)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/dm/message-requests/{id}/accept", wsHandlers.AcceptMessageRequest)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/dm/message-requests/{id}/decline", wsHandlers.DeclineMessageRequest)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Put("/dm/privacy", wsHandlers.UpdateDMPrivacy)
			})

			// Media routes (Section 34 — Media System)
			mediaHandlers := media.NewHandlers(mediaService)

			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/media/upload", mediaHandlers.InitiateUpload)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/media/confirm/{id}", mediaHandlers.ConfirmUpload)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/media/{id}", mediaHandlers.GetMedia)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Patch("/media/{id}", mediaHandlers.UpdateMetadata)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/media/{id}", mediaHandlers.DeleteMedia)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/media", mediaHandlers.ListMedia)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/media", mediaHandlers.ListMyMedia)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/media/bulk-delete", mediaHandlers.BulkDelete)

			// Media quarantine admin routes — review queue for virus scan false positives
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.quarantine.manage")).
				Get("/admin/media/quarantined", mediaHandlers.ListQuarantined)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.quarantine.manage")).
				Post("/admin/media/{id}/approve", mediaHandlers.ApproveQuarantined)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "media.quarantine.manage")).
				Post("/admin/media/{id}/reject", mediaHandlers.RejectQuarantined)

			// Forum Q&A / Solved routes — gated by feature.forum (Section 08)
			forumService := forum.NewService(database)
			forumService.SetSettings(settingsService)
			forumHandlers := forum.NewHandlers(forumService)

			r.With(features.Gate(featureChecker, "feature.forum")).Group(func(r chi.Router) {
				// Thread routes
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/forum", forumHandlers.CreateThread)
				r.Get("/forum", forumHandlers.ListThreads)
				r.Get("/forum/{slug}", forumHandlers.GetThread)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.thread.edit_own")).
					Patch("/forum/{id}", forumHandlers.UpdateThread)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.thread.delete_own")).
					Delete("/forum/{id}", forumHandlers.DeleteThread)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.thread.solve")).
					Post("/forum/{id}/solve", forumHandlers.SolveThread)

				// Reply routes
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/forum/{id}/replies", forumHandlers.CreateReply)
				r.Get("/forum/{id}/replies", forumHandlers.ListReplies)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.reply.edit_own")).
					Patch("/forum/replies/{id}", forumHandlers.UpdateReply)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.reply.delete_own")).
					Delete("/forum/replies/{id}", forumHandlers.DeleteReply)

				// Vote route
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/forum/vote", forumHandlers.Vote)

				// Poll routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.poll.create")).
					Post("/forum/polls", forumHandlers.CreatePoll)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/forum/polls/{id}/vote", forumHandlers.VotePoll)
				r.Get("/forum/polls/{id}", forumHandlers.GetPoll)

				// Tag routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.tag.create")).
					Post("/forum/tags", forumHandlers.CreateTag)
				r.Get("/forum/tags", forumHandlers.ListTags)
				r.Get("/forum/tags/{slug}", forumHandlers.GetTagBySlug)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/forum/{id}/tags/{tagId}", forumHandlers.AddThreadTag)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/forum/{id}/tags/{tagId}", forumHandlers.RemoveThreadTag)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.tag.edit")).Patch("/forum/tags/{id}", forumHandlers.UpdateTag)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "forum.tag.edit")).Delete("/forum/tags/{id}", forumHandlers.DeleteTag)

				// Bookmark routes
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/forum/bookmarks", forumHandlers.AddBookmark)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/forum/bookmarks/{id}", forumHandlers.RemoveBookmark)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/forum/bookmarks", forumHandlers.ListBookmarks)
			})

			// Blog Extension routes — gated by feature.blog (Section 24)
			r.With(features.Gate(featureChecker, "feature.blog")).Group(func(r chi.Router) {
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

				// Review routes (admin workflow — keep auth-required)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.review.create")).
					Post("/blog/reviews", blogHandlers.CreateReview)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/blog/reviews", blogHandlers.ListReviews)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "blog.review.manage")).
					Patch("/blog/reviews/{id}", blogHandlers.UpdateReview)

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
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/reviews/{entity_type}/{entity_id}", reviewHandlers.CreateReview)
			r.Get("/reviews", reviewHandlers.ListReviews)
			r.Get("/reviews/{entity_type}/{entity_id}", reviewHandlers.ListReviews)
			r.Get("/reviews/{entity_type}/{entity_id}/aggregate", reviewHandlers.GetAggregate)
			r.Get("/reviews/{id}", reviewHandlers.GetReview)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Patch("/reviews/{id}", reviewHandlers.UpdateReview)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/reviews/{id}", reviewHandlers.DeleteReview)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/reviews/{id}/flag", reviewHandlers.FlagReview)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Patch("/admin/reviews/{id}", reviewHandlers.ModerateReview)
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

				// Type access routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comparison.type.access.manage")).
					Put("/comparisons/types/{id}/access", comparisonHandlers.SetComparisonTypeAccess)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/comparisons/types/{id}/access", comparisonHandlers.ListComparisonTypeAccess)

				// Comparison CRUD routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comparison.create")).
					Post("/comparisons", comparisonHandlers.CreateComparison)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/comparisons/{id}", comparisonHandlers.GetComparison)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/comparisons/{id}/matrix", comparisonHandlers.GetComparisonMatrix)

				// Item routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comparison.item.manage")).
					Post("/comparisons/{id}/items", comparisonHandlers.CreateComparisonItem)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comparison.item.manage")).
					Patch("/comparisons/items/{id}", comparisonHandlers.UpdateComparisonItem)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "comparison.item.manage")).
					Delete("/comparisons/items/{id}", comparisonHandlers.DeleteComparisonItem)

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

				// Profile CRUD routes
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.create")).
					Post("/profiles", professionalHandlers.CreateProfessionalProfile)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/profiles", professionalHandlers.ListProfessionalProfiles)
				r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/profiles/{slug}", professionalHandlers.GetProfessionalProfile)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.edit_own")).
					Patch("/profiles/{id}", professionalHandlers.UpdateProfessionalProfile)
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "professional_profile.delete_own")).
					Delete("/profiles/{id}", professionalHandlers.DeleteProfessionalProfile)

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
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/organizations", orgHandlers.CreateOrganization)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/organizations", orgHandlers.ListOrganizations)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/organizations/{slug}", orgHandlers.GetOrganization)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Patch("/organizations/{id}", orgHandlers.UpdateOrganization)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/organizations/{id}", orgHandlers.DeleteOrganization)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/organizations/{id}/members", orgHandlers.ListMembers)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/organizations/{id}/members", orgHandlers.AddMember)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/organizations/{id}/members/{userId}", orgHandlers.RemoveMember)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Patch("/organizations/{id}/members/{userId}", orgHandlers.UpdateMemberRole)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/organizations/{id}/invitations", orgHandlers.InviteMember)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/organizations/{id}/invitations", orgHandlers.ListInvitations)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/organizations/invitations/{token}/accept", orgHandlers.AcceptInvitation)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/claim-ownership", orgHandlers.ClaimOwnership)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/claim-ownership", orgHandlers.ListClaims)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Patch("/claim-ownership/{id}", orgHandlers.ReviewClaim)

			// Payment Core routes (Section 17 — Payment Core)
			// Payment initiation
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "payments.create")).
				Post("/payments/intent", paymentHandlers.InitiatePayment)
			// Webhooks (public - no auth required)
			r.Post("/payments/webhook/paystack", paymentHandlers.HandlePaystackWebhook)
			r.Post("/payments/webhook/stripe", paymentHandlers.HandleStripeWebhook)
			r.Post("/payments/webhook/nowpayments", paymentHandlers.HandleNOWPaymentsWebhook)
			// Payment listing and details (catalog: /me/payments)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/payments", paymentHandlers.ListPayments)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/payments/{id}", paymentHandlers.GetPayment)
			// Refunds
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "payments.refund")).
				Post("/payments/{id}/refund", paymentHandlers.RefundPayment)
			// Subscriptions
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "payments.subscriptions.manage")).
				Post("/subscriptions", paymentHandlers.CreateSubscription)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/subscriptions", paymentHandlers.ListSubscriptions)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/subscription", paymentHandlers.GetActiveSubscription)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/me/subscription/cancel", paymentHandlers.CancelMySubscription)
			// NOTE (P5): /me/subscription/reactivate has no safe wiring yet —
			// CancelSubscription is a hard cutoff (status='canceled', cancel_at=now()),
			// so reactivating needs a product decision on billing-period recomputation
			// and possibly payment-gateway re-authorization, not just a status flip.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "payments.subscriptions.manage")).
				Patch("/subscriptions/{id}/cancel", paymentHandlers.CancelSubscription)
			// Discount codes
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "payments.discount_codes.manage")).
				Post("/discount-codes", paymentHandlers.CreateDiscountCode)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/discount-codes", paymentHandlers.ListDiscountCodes)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/payments/discount-codes/validate", paymentHandlers.ValidateDiscountCode)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "payments.discount_codes.manage")).
				Delete("/discount-codes/{id}", paymentHandlers.DeleteDiscountCode)

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
				// Conversion
				r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wallet.points.convert")).
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

			// Ticket System routes (Section 38 — Ticket System)
			// User portal (catalog: /tickets, /me/tickets)
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
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gdpr.consents.view")).
				Get("/admin/consents", gdprHandlers.ListConsentsByTenant)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gdpr.data_exports.view")).
				Get("/admin/data-exports", gdprHandlers.ListPendingDataExports)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "gdpr.deletions.view")).
				Get("/admin/deletions", gdprHandlers.ListPendingDeletions)

			// Bot & Crawler Management routes (Section 43)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "bots.admin")).
				Get("/admin/bots", botHandlers.ListKnownBots)
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
				Delete("/admin/spam/ip-blocks/{id}", spamHandlers.DeleteIPBlock)
			// Censored word management
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spam.censored_words.manage")).
				Get("/admin/spam/censored-words", spamHandlers.ListCensoredWords)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "spam.censored_words.manage")).
				Post("/admin/spam/censored-words", spamHandlers.CreateCensoredWord)
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
			r.Get("/developer/swagger", publicapi.HandleSwaggerUI)

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

			// Admin API Key management (Section 47)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "api.keys.view.all")).
				Get("/admin/api-keys", publicAPIHandlers.ListAllKeys)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "api.keys.manage")).
				Post("/admin/api-keys/{id}/suspend", publicAPIHandlers.SuspendKey)

			// User Profile & Preferences routes (catalog: /me, /me/profile)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me", usersHandlers.GetMyProfile)
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
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/scheduled", usersHandlers.ListMyScheduled)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/bookmarks", usersHandlers.ListMyBookmarks)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/analytics", usersHandlers.GetMyAnalytics)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/me/deactivate", usersHandlers.DeactivateAccount)
			// GDPR deletion + data export — served by the REAL gdpr handlers
			// (queue-backed export via data:export_user_data + Honcho hard erasure),
			// not the users-package stubs. Consolidated here from /account/* (§6).
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/me", gdprHandlers.RequestAccountDeletion)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/data-export", gdprHandlers.RequestDataExport)
			// NOTE: replaces the users-package stub that hardcoded "no_pending_export"
			// regardless of actual state (same class of bug as the DELETE /me fix).
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/data-export/status", gdprHandlers.GetMyDataExportStatus)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/data-export/download", gdprHandlers.GetMyDataExportDownload)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.users.list")).Get("/users", usersHandlers.ListUsers)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/users/{username}", usersHandlers.GetUserByUsername)

			// Onboarding routes (Section 09)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/onboarding/status", onboardingHandlers.GetStatus)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Put("/onboarding/step", onboardingHandlers.UpdateStep)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/onboarding/complete", onboardingHandlers.Complete)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/onboarding/recommendations", onboardingHandlers.GetRecommendations)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/onboarding/tour/dismiss", onboardingHandlers.DismissTour)

			// Search routes
			r.With(auth.RequireAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "moderate")).Get("/search", searchHandlers.Search)
			r.Get("/search/autocomplete", searchHandlers.Autocomplete)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/spaces/{id}/search", searchHandlers.SearchSpace)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/tags/{slug}/search", searchHandlers.SearchTag)

			// General Tags (Sections 08, 49) — distinct from forum tags.
			r.Get("/tags", tagsHandlers.ListTags)
			r.Get("/tags/{slug}", tagsHandlers.GetTag)
			r.Get("/tags/{slug}/content", tagsHandlers.GetTagContent)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Post("/tags/{slug}/follow", tagsHandlers.ToggleTagFollow)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Delete("/tags/{slug}/follow", tagsHandlers.ToggleTagFollow)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/tags/{slug}/followers", tagsHandlers.GetTagFollowers)
			// NOTE (admin gaps): admin tag CRUD (POST/PATCH/DELETE /admin/tags) does
			// not exist yet — there is currently no way to create a general tag at
			// all, so content_tags/AddContentTag is effectively unreachable until
			// that's implemented.
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.analytics.search")).
				Get("/admin/search/analytics", searchHandlers.SearchAdminAnalytics)

			// AI Core routes (Section 20 — AI Core)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.generate")).
				Post("/ai/generate", aiHandlers.HandleGenerate)

			// JapaBot chat — gated by feature.ai_chat
			r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.ai_chat")).
				Post("/ai/chat", aiHandlers.HandleChat)

			// Content AI Assist endpoints (catalog: /ai/assist/*)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Post("/ai/assist/improve", aiHandlers.HandleImprove)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Post("/ai/assist/summarize", aiHandlers.HandleSummarize)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Post("/ai/assist/translate", aiHandlers.HandleTranslate)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
				Post("/ai/assist/generate-tags", aiHandlers.HandleGenerateTags)
			r.With(auth.RequireAuth(kr, vkClient.Redis())).
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
				Get("/admin/ai/tools/usage", aiToolHandlers.ListToolUsage)

			// AI Context admin routes — view/delete user AI context via Honcho
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.context.admin_view")).
				Get("/admin/ai/context/{user_id}", aiHandlers.HandleGetUserContext)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.context.admin_delete_entry")).
				Delete("/admin/ai/context/{user_id}/{entry_id}", aiHandlers.HandleDeleteUserContextEntry)

			// AI Usage admin route (stub until ai_usage_log table exists)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.context.admin_view")).
				Get("/admin/ai/usage", aiHandlers.HandleGetAIUsage)

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

			// Newsletter admin routes (Section 33 — Newsletter)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "newsletter.admin")).
				Get("/admin/newsletter/subscribers", newsletterHandlers.ListSubscribers)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "newsletter.admin")).
				Get("/admin/newsletter/subscribers/{id}", newsletterHandlers.GetSubscriber)
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
				Post("/admin/newsletter/campaigns/{id}/send", newsletterHandlers.HandleSendCampaign)

			// ── Coupons & External Affiliate Providers (Section 55) ────

			// Public coupon routes
			r.Get("/coupons", couponHandlers.ListPublicCoupons)
			r.Get("/coupons/{id}", couponHandlers.GetCoupon)
			r.Post("/coupons/{id}/click", couponHandlers.RecordCouponClick)

			// Render helpers (internal — called by Go renderer)
			r.Get("/render/coupon/{id}", couponHandlers.RenderCoupon)
			r.Get("/render/affiliate-link", couponHandlers.RenderAffiliateLink)

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

			// Admin: manage documentation pages
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "docs.admin")).
				Get("/admin/docs/{audience}", docsHandlers.ListAllDocPages)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "docs.admin")).
				Post("/admin/docs", docsHandlers.CreateDocPage)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "docs.admin")).
				Patch("/admin/docs/{id}", docsHandlers.UpdateDocPage)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "docs.admin")).
				Delete("/admin/docs/{id}", docsHandlers.DeleteDocPage)

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

			// Admin Moderation routes (stubs)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Get("/admin/moderation", adminHandlers.ListModeration)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Post("/admin/moderation/bulk", adminHandlers.BulkModeration)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).
				Post("/admin/moderation/{id}/appeal", adminHandlers.AppealModeration)

			// Admin Broadcast route (stub)
			r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.broadcast")).
				Post("/admin/broadcast", adminHandlers.Broadcast)
		})
	})

	return r
}

// bootstrapPermissions registers all custom field and form permission keys at
// startup, creates a super_admin role, assigns every permission to it, and
// grants the role to the super_admin system group. Every operation is
// idempotent (ON CONFLICT DO NOTHING) so repeated startups are safe. If any
// step fails the error is logged but the server still starts; individual
// permission checks will simply fail until the database is healthy.
//
// The database's Writer pool is used for writes and the Reader pool for
// reads, following the existing read/write split convention.
func bootstrapPermissions(ctx context.Context, database *db.DB, rbacStore *rbac.Store) {
	permissions := []rbac.Permission{
		{Key: "custom_fields.group.create", Description: "Create custom field groups"},
		{Key: "custom_fields.group.edit", Description: "Edit custom field groups"},
		{Key: "custom_fields.group.delete", Description: "Delete custom field groups"},
		{Key: "custom_fields.field.create", Description: "Create custom fields"},
		{Key: "custom_fields.field.edit", Description: "Edit custom fields"},
		{Key: "custom_fields.field.delete", Description: "Delete custom fields"},
		{Key: "custom_fields.values.set", Description: "Set custom field values"},
		{Key: "forms.create", Description: "Create forms"},
		{Key: "forms.edit", Description: "Edit forms"},
		{Key: "forms.delete", Description: "Delete forms"},
		{Key: "forms.submissions.view", Description: "View form submissions"},
		{Key: "spaces.create", Description: "Create spaces"},
		{Key: "spaces.edit", Description: "Edit spaces"},
		{Key: "spaces.delete", Description: "Delete spaces"},
		{Key: "spaces.moderate", Description: "Moderate spaces (manage rules and pins)"},
		{Key: "spaces.moderate.any", Description: "Moderate any space (for admin_only posting permission)"},
		{Key: "spaces.moderate.assigned", Description: "Assign or remove space moderators"},
		{Key: "space_types.create", Description: "Create space types"},
		{Key: "space_types.edit", Description: "Edit space types"},
		{Key: "space_types.delete", Description: "Delete space types"},
		{Key: "spaces.admin", Description: "Manage space permission presets"},
		{Key: "content.create", Description: "Create content"},
		{Key: "content.edit", Description: "Edit content"},
		{Key: "content.delete", Description: "Delete content"},
		{Key: "content.revisions.view", Description: "View content revisions"},
		{Key: "content.pin", Description: "Pin content to spaces"},
		{Key: "content.reaction_types.manage", Description: "Manage reaction types"},
		{Key: "content.reactions.add", Description: "Add reactions to content"},
		{Key: "content.reactions.remove", Description: "Remove reactions from content"},
		{Key: "content.scores.view", Description: "View content scores"},
		{Key: "content.jsonld.manage", Description: "Manage JSON-LD templates"},
		{Key: "content.scheduled.view", Description: "View scheduled content"},
		{Key: "content.page_builder.create", Description: "Create pages with page builder"},
		{Key: "content.page_builder.edit", Description: "Edit pages with page builder"},
		{Key: "content.page_builder.delete", Description: "Delete page builder pages"},
		{Key: "content.render", Description: "Render content via TipTap SSR"},
		{Key: "link_preview.fetch", Description: "Fetch link previews from external URLs"},
		{Key: "follows.toggle", Description: "Follow/unfollow entities"},
		{Key: "follows.view", Description: "View follower/following lists"},
		{Key: "notifications.view", Description: "View notifications"},
		{Key: "notifications.manage", Description: "Manage notification types"},
		{Key: "dm.send", Description: "Send direct messages"},
		{Key: "dm.view", Description: "View direct messages"},
		{Key: "dm.manage_privacy", Description: "Manage DM privacy settings"},
		{Key: "media.upload", Description: "Upload media files"},
		{Key: "media.view_own", Description: "View own uploaded media"},
		{Key: "media.view_all", Description: "View all media (admin)"},
		{Key: "media.delete_own", Description: "Delete own uploaded media"},
		{Key: "media.delete_any", Description: "Delete any media (admin)"},
		{Key: "media.video.upload", Description: "Upload video files"},
		{Key: "media.library.view_own", Description: "View own media library"},
		{Key: "media.library.view_all", Description: "View all media library (admin)"},
		{Key: "media.quarantine.manage", Description: "Review and manage quarantined media (approve/reject)"},
		// Forum Q&A / Solved (Section 08)
		{Key: "forum.thread.create", Description: "Create forum threads"},
		{Key: "forum.thread.edit_own", Description: "Edit own forum threads"},
		{Key: "forum.thread.edit_any", Description: "Edit any forum thread (admin)"},
		{Key: "forum.thread.delete_own", Description: "Delete own forum threads"},
		{Key: "forum.thread.delete_any", Description: "Delete any forum thread (admin)"},
		{Key: "forum.thread.solve", Description: "Mark forum threads as solved"},
		{Key: "forum.reply.create", Description: "Create forum replies"},
		{Key: "forum.reply.edit_own", Description: "Edit own forum replies"},
		{Key: "forum.reply.edit_any", Description: "Edit any forum reply (admin)"},
		{Key: "forum.reply.delete_own", Description: "Delete own forum replies"},
		{Key: "forum.reply.delete_any", Description: "Delete any forum reply (admin)"},
		{Key: "forum.vote", Description: "Vote on forum threads and replies"},
		{Key: "forum.bookmark.manage", Description: "Manage forum bookmarks"},
		{Key: "forum.poll.create", Description: "Create forum polls"},
		{Key: "forum.poll.vote", Description: "Vote on forum polls"},
		{Key: "forum.tag.create", Description: "Create forum tags"},
		{Key: "forum.tag.edit", Description: "Edit forum tags"},
		{Key: "forum.tag.moderate", Description: "Moderate forum tags"},
		{Key: "forum.admin", Description: "Admin-only forum management"},
		// Blog Extension (Section 24)
		{Key: "blog.create", Description: "Create blog posts"},
		{Key: "blog.edit_own", Description: "Edit own blog posts"},
		{Key: "blog.edit_any", Description: "Edit any blog post (admin)"},
		{Key: "blog.publish", Description: "Publish blog posts"},
		{Key: "blog.feature", Description: "Feature/unfeature blog posts"},
		{Key: "blog.series.create", Description: "Create blog series"},
		{Key: "blog.series.edit", Description: "Edit blog series"},
		{Key: "blog.series.delete", Description: "Delete blog series"},
		{Key: "blog.review.create", Description: "Create content reviews"},
		{Key: "blog.review.manage", Description: "Manage content review status"},
		{Key: "blog.rss.view", Description: "View RSS feed"},
		// Universal Ratings & Reviews (Section 29)
		{Key: "reviews.create", Description: "Create reviews"},
		{Key: "reviews.edit_own", Description: "Edit own reviews"},
		{Key: "reviews.edit_any", Description: "Edit any review (admin)"},
		{Key: "reviews.delete_own", Description: "Delete own reviews"},
		{Key: "reviews.delete_any", Description: "Delete any review (admin)"},
		{Key: "reviews.flag", Description: "Flag reviews for moderation"},
		{Key: "reviews.moderate", Description: "Moderate review status"},
		{Key: "reviews.aggregate.view", Description: "View review aggregates"},
		// Generic Comparison System (Section 30)
		{Key: "comparison.type.create", Description: "Create comparison types"},
		{Key: "comparison.type.edit", Description: "Edit comparison types"},
		{Key: "comparison.type.delete", Description: "Delete comparison types"},
		{Key: "comparison.type.access.manage", Description: "Manage comparison type access permissions"},
		{Key: "comparison.create", Description: "Create comparisons"},
		{Key: "comparison.edit_own", Description: "Edit own comparisons"},
		{Key: "comparison.edit_any", Description: "Edit any comparison (admin)"},
		{Key: "comparison.delete_own", Description: "Delete own comparisons"},
		{Key: "comparison.delete_any", Description: "Delete any comparison (admin)"},
		{Key: "comparison.item.manage", Description: "Manage comparison items"},
		{Key: "comparison.dimension.manage", Description: "Manage comparison dimensions"},
		{Key: "comparison.value.manage", Description: "Manage comparison values"},
		// Professional Profiles (Section 33)
		{Key: "professional_profile.type.create", Description: "Create professional profile types"},
		{Key: "professional_profile.type.view", Description: "View professional profile types"},
		{Key: "professional_profile.create", Description: "Create professional profiles"},
		{Key: "professional_profile.edit_own", Description: "Edit own professional profiles"},
		{Key: "professional_profile.edit_any", Description: "Edit any professional profile (admin)"},
		{Key: "professional_profile.delete_own", Description: "Delete own professional profiles"},
		{Key: "professional_profile.delete_any", Description: "Delete any professional profile (admin)"},
		{Key: "professional_profile.member.manage", Description: "Manage professional profile members"},
		{Key: "professional_profile.invite", Description: "Send professional profile invitations"},
		{Key: "professional_profile.author.switch", Description: "Switch authorship to professional profile"},
		// Payment Core (Section 17)
		{Key: "payments.create", Description: "Create payments"},
		{Key: "payments.view_own", Description: "View own payments"},
		{Key: "payments.view_any", Description: "View any payment (admin)"},
		{Key: "payments.refund", Description: "Refund payments"},
		{Key: "payments.webhook", Description: "Handle payment webhooks"},
		{Key: "payments.subscriptions.manage", Description: "Manage subscriptions"},
		{Key: "payments.discount_codes.manage", Description: "Manage discount codes"},
		// Wallet & Credits (Section 28)
		{Key: "wallet.points.balance", Description: "View points balance"},
		{Key: "wallet.credits.balance", Description: "View credits balance"},
		{Key: "wallet.points.earn", Description: "Earn points"},
		{Key: "wallet.credits.spend", Description: "Spend credits"},
		{Key: "wallet.points.convert", Description: "Convert points to credits"},
		{Key: "wallet.credits.purchase", Description: "Purchase credits"},
		{Key: "wallet.admin.adjust", Description: "Admin balance adjustment"},
		{Key: "wallet.admin.rules.manage", Description: "Manage activity point rules"},
		{Key: "wallet.admin.settings.manage", Description: "Manage wallet settings"},
		// Advertising (Section 25)
		{Key: "ad.campaign.manage", Description: "Manage advertising campaigns"},
		{Key: "ad.slot.manage", Description: "Manage advertising slot types"},
		{Key: "ad.stats.view", Description: "View advertising analytics"},
		{Key: "ad.event.track", Description: "Track ad impressions and clicks"},
		// Spam Prevention & Content Safety (Section 41)
		{Key: "spam.ip_block.manage", Description: "Manage IP blocks"},
		{Key: "spam.censored_words.manage", Description: "Manage censored word list"},
		{Key: "spam.user_block.manage", Description: "Block/unblock users"},
		// Ticket System (Section 38)
		{Key: "tickets.submit", Description: "Submit support tickets"},
		{Key: "tickets.view_own", Description: "View own tickets"},
		{Key: "tickets.admin", Description: "Admin ticket management"},
		{Key: "tickets.type.manage", Description: "Manage ticket types"},
		// GDPR / NDPR Compliance (Section 19)
		{Key: "gdpr.audit_logs.view", Description: "View audit logs (super_admin)"},
		{Key: "gdpr.consents.view", Description: "View consent records (admin)"},
		{Key: "gdpr.data_exports.view", Description: "View data export requests (admin)"},
		{Key: "gdpr.deletions.view", Description: "View account deletion requests (admin)"},
		// Bot & Crawler Management (Section 43)
		{Key: "bots.admin", Description: "Administer known bots and crawler policies"},
		// Platform Analytics & Site Reports (Section 40)
		{Key: "analytics.admin", Description: "View analytics dashboard and security scan results"},
		{Key: "analytics.cache.flush", Description: "Flush platform caches (admin-only)"},
		// Documentation System (Section 44)
		{Key: "docs.admin", Description: "Manage documentation pages (create, edit, delete, review)"},
		// Learning Platform (Section 36)
		{Key: "learning.view", Description: "View published learning tracks and lessons"},
		{Key: "learning.progress.update", Description: "Update lesson progress"},
		{Key: "learning.certificate.issue", Description: "Issue learning certificates"},
		{Key: "learning.admin.manage", Description: "Manage learning tracks, modules, and lessons"},
		// AI Core (Section 20)
		{Key: "ai.generate", Description: "Generate AI content via the AI gateway"},
		{Key: "ai.hitl.review", Description: "Review and approve/reject AI-generated output"},
		{Key: "ai.context.points_balance", Description: "View AI credit/points balance"},
		{Key: "ai.context.admin_view", Description: "View AI usage analytics (admin)"},
		// Platform Settings (admin)
		{Key: "admin.settings", Description: "Manage platform settings"},
		// Admin RBAC Management
		{Key: "admin.rbac.manage", Description: "Manage RBAC groups, roles, and permissions"},
		// Admin Moderation
		{Key: "admin.moderation", Description: "Manage content moderation queue"},
		// Admin Broadcast
		{Key: "admin.broadcast", Description: "Send broadcast messages to users"},
		// Admin Impersonation (Section 46)
		{Key: "admin.impersonate", Description: "Impersonate other users for support/debugging"},
		// Public API & Developer Portal (Section 47)
		{Key: "api.keys.self_manage", Description: "Create, list, and revoke own API keys"},
		{Key: "api.keys.view.all", Description: "View all API keys across all users (admin)"},
		{Key: "api.keys.manage", Description: "Suspend and manage API keys (admin)"},
		{Key: "api.keys.analytics.view", Description: "View API key usage analytics"},
		// i18n Translation System
		{Key: "i18n.manage", Description: "Manage i18n translations (create, update, delete)"},
		// Feature Flags
		{Key: "features.manage", Description: "Toggle feature flags for a tenant"},
		// AI Tool Registry (Section 22)
		{Key: "ai.tools.manage", Description: "Manage AI tool configurations and enablement"},
		// Webhook Admin CRUD (Section 23 — Integrations)
		{Key: "integrations.webhooks.manage", Description: "Manage webhook endpoint configurations"},
		// Newsletter (Section 33)
		{Key: "newsletter.admin", Description: "Manage newsletter subscribers and campaigns"},
		// Referral Program (Section 34)
		{Key: "referral.admin", Description: "Manage referral program settings and view referrals"},
		// Mentorship (Section 35)
		{Key: "mentorship.admin", Description: "Manage mentorship programs and matches"},
		// Page Builder (Section 16)
		{Key: "pagebuilder.blocks.manage", Description: "Manage page builder blocks (create, edit, delete)"},
		{Key: "pagebuilder.templates.manage", Description: "Manage page builder templates (create, edit, delete)"},
		{Key: "pagebuilder.themes.manage", Description: "Manage page builder themes (create, edit, delete, set default)"},
		{Key: "pages.manage", Description: "Manage pages (create, edit, delete)"},
		{Key: "pages.publish", Description: "Publish and archive pages"},
		// Geo IP (Phase 30)
		{Key: "geo.lookup", Description: "Resolve arbitrary IP addresses via GeoIP"},
	}

	var defaultTenantID pgtype.UUID
	if err := defaultTenantID.Scan("00000000-0000-0000-0000-000000000001"); err != nil {
		slog.Error("bootstrapping permissions: scanning default tenant ID", "error", err)
		return
	}

	// Register every permission key (idempotent).
	for _, perm := range permissions {
		if err := rbacStore.RegisterPermission(ctx, perm.Key, perm.Description); err != nil {
			slog.Error("bootstrapping permissions: registering permission", "key", perm.Key, "error", err)
		}
	}

	// Create the super_admin_role for the default tenant (idempotent).
	_, err := database.Writer.Exec(ctx, `
		INSERT INTO user_roles (tenant_id, name, slug)
		VALUES ($1, 'Super Admin Role', 'super_admin_role')
		ON CONFLICT (tenant_id, slug) DO NOTHING
	`, defaultTenantID)
	if err != nil {
		slog.Error("bootstrapping permissions: creating super_admin role", "error", err)
		return
	}

	// Resolve the role ID (the INSERT above may have been a no-op).
	var roleID pgtype.UUID

	err = database.Reader.QueryRow(ctx, `
		SELECT id FROM user_roles WHERE tenant_id = $1 AND slug = 'super_admin_role'
	`, defaultTenantID).Scan(&roleID)
	if err != nil {
		slog.Error("bootstrapping permissions: resolving super_admin role ID", "error", err)
		return
	}

	// Resolve the super_admin system group ID by slug.
	var groupID pgtype.UUID

	err = database.Reader.QueryRow(ctx, `
		SELECT id FROM user_groups WHERE tenant_id = $1 AND slug = 'super_admin'
	`, defaultTenantID).Scan(&groupID)
	if err != nil {
		slog.Error("bootstrapping permissions: resolving super_admin group ID", "error", err)
		return
	}

	// Assign every permission to the super_admin role.
	for _, perm := range permissions {
		if err := rbacStore.AssignPermissionToRole(ctx, roleID, perm.Key); err != nil {
			slog.Error("bootstrapping permissions: assigning permission to role", "key", perm.Key, "error", err)
		}
	}

	// Grant the super_admin role to the super_admin system group.
	if err := rbacStore.AssignRoleToGroup(ctx, groupID, roleID); err != nil {
		slog.Error("bootstrapping permissions: assigning role to group", "error", err)
		return
	}

	slog.Info("permissions bootstrapped", "permissions", len(permissions), "role", "super_admin_role")
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
