package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/admin"
	"github.com/jedafe/japa/internal/analytics"
	"github.com/jedafe/japa/internal/advertising"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/bots"
	"github.com/jedafe/japa/internal/docs"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/blog"
	"github.com/jedafe/japa/internal/comparison"
	"github.com/jedafe/japa/internal/feed"
	"github.com/jedafe/japa/internal/follows"
	"github.com/jedafe/japa/internal/forum"
	"github.com/jedafe/japa/internal/gdpr"
	"github.com/jedafe/japa/internal/media"
	"github.com/jedafe/japa/internal/onboarding"
	"github.com/jedafe/japa/internal/notifications"
	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/payments"
	"github.com/jedafe/japa/internal/professional"
	"github.com/jedafe/japa/internal/reviews"
	"github.com/jedafe/japa/internal/wallet"
	"github.com/jedafe/japa/internal/websocket"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/seo"
	"github.com/jedafe/japa/internal/features"
	"github.com/jedafe/japa/internal/geoip"
	"github.com/jedafe/japa/internal/linkpreview"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/jedafe/japa/internal/renderer"
	"github.com/jedafe/japa/internal/search"
	"github.com/jedafe/japa/internal/spaces"
	"github.com/jedafe/japa/internal/spam"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/tickets"
	"github.com/jedafe/japa/internal/users"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/worker"
)

// NewServer creates a fully configured Chi router with middleware and routes.
func NewServer(database *db.DB, cfg *config.Config) *chi.Mux {
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

	// IP block middleware — reject blocked IPs before they reach any handler.
	// Runs after RealIP (so the resolved IP is available) but before all
	// other middleware and route handlers.
	r.Use(spam.IPBlockMiddleware(database))

	h := NewHandler(database)
	tenantStore := tenant.NewDBStore(database)

	// JWT key rotation - in production this would be loaded from config/secrets.
	// For development we generate a random key at startup.
	devKey := make([]byte, 32)
	if _, err := rand.Read(devKey); err != nil {
		slog.Error("generating JWT key", "error", err)
	}
	kr := auth.NewKeyRotation("key-dev", devKey)

	// Auth handlers and DB store
	authStore := auth.NewDBStore(database)
	noopBruteForce := &noopBruteForceStore{}
	authHandlers := &auth.AuthHandlers{
		DB:          authStore,
		KeyRotation: kr,
		BruteForce:  noopBruteForce,
		Policy:      auth.DefaultPasswordPolicy(),
	}

	// Feature flag checker
	featureChecker := features.New(database)

	// RBAC resolver and store for permission checks at the router level.
	rbacResolver := rbac.New(database)
	rbacStore := rbac.NewStore(database)

	// Valkey client for cache, rate limiting, edit locks, and space unread
	// counters. Created with graceful fallback so the server boots even when
	// Valkey is unreachable.
	vkClient := valkey.NewWithFallback(cfg.RedisAddr)
	slog.Info("valkey client created", "addr", cfg.RedisAddr)

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

	// Content subsystem (Section 14 — Content Type System).
	contentService := content.NewService(database)
	contentHandlers := content.NewHandlers(contentService)

	// Renderer subsystem (Section 15 — Block Editor SSR).
	rendererInstance := renderer.New("scripts/tiptap-ssr.ts", vkClient)
	rendererHandlers := renderer.NewHandlers(rendererInstance)

	// Link preview subsystem (Section 15 — Link Preview Fetch).
	linkPreviewFetcher := linkpreview.NewFetcher(nil, vkClient)
	linkPreviewHandlers := linkpreview.NewHandlers(linkPreviewFetcher)

	// Follow System subsystem (Section 31 — Follow System).
	followService := follows.NewService(database)
	followHandlers := follows.NewHandlers(followService)

	// Notification System subsystem (Section 35 — Notification System).
	notificationService := notifications.NewService(database)
	notificationHandlers := notifications.NewHandlers(notificationService)

		// Feed System subsystem (Section 32 — Activity Feed / Timeline).
		feedService := feed.NewService(database)
		feedHandlers := feed.NewHandlers(feedService)

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
		professionalHandlers := professional.NewHandlers(professionalService)

// Wallet & Credits subsystem (Section 28 — Wallet & Credits).
	walletService := wallet.NewService(database)
	walletHandlers := wallet.NewHandlers(walletService)

	// Payment Core subsystem (Section 17 — Payment Core).
	paymentService := payments.NewService(database, &walletCreditAdapter{svc: walletService})
	paymentHandlers := payments.NewHandlers(paymentService)

	// Advertising subsystem (Section 25 — Ad Placement).
	adService := advertising.NewService(database)
	adHandlers := advertising.NewHandlers(adService)

	// Ticket System subsystem (Section 38 — Ticket System).
	ticketService := tickets.NewService(database)
	ticketHandlers := tickets.NewHandlers(ticketService)

	// GDPR / NDPR Compliance subsystem (Section 19).
	gdprService := gdpr.NewService(database)
	gdprHandlers := gdpr.NewHandlers(gdprService)

	// Register GDPR processor with worker for background tasks.
	gdprAdapter := gdpr.NewWorkerAdapter(gdprService)
	worker.RegisterGDPRProcessor(gdprAdapter)

	// Documentation System subsystem (Section 44).
	docsService := docs.NewService(database)
	docsHandlers := docs.NewHandlers(docsService)

	// Admin Settings subsystem.
	adminHandlers := admin.NewHandlers(database)

	// Bot & Crawler Management subsystem (Section 43).
	botStore := bots.NewStoreWithDB(database)
	botService := bots.NewServiceFromStore(botStore)
	botHandlers := bots.NewHandlers(botService)

	// Platform Analytics & Site Reports subsystem (Section 40).
	analyticsService := analytics.NewService(database)
	analyticsHandlers := analytics.NewHandlers(analyticsService)

	// Register analytics processor with worker for background tasks.
	analyticsAdapter := analytics.NewWorkerAdapter(analyticsService)
	worker.RegisterAnalyticsProcessor(analyticsAdapter)

	// Cache flusher for admin-only cache invalidation.
	cacheFlusher := analytics.NewCacheFlusher(vkClient)

	// User Profile & Preferences subsystem.
	usersService := users.NewService(database)
	usersHandlers := users.NewHandlers(usersService)

	// Search subsystem.
	searchHandlers := search.NewHandlers()

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
	})

	// API routes go through tenant resolution
	r.Group(func(r chi.Router) {
		r.Use(tenant.Middleware(tenantStore))
		r.Route("/api/v1", func(r chi.Router) {
			// Auth routes (public - no auth required, tenant-scoped)
			r.Route("/auth", func(r chi.Router) {
				r.Post("/register", authHandlers.Register)
				r.Post("/login", authHandlers.Login)
				r.Post("/refresh", authHandlers.Refresh)
				r.Post("/logout", authHandlers.Logout)
			})

			// Feature flags endpoint (public - frontend needs flags to
			// render navigation, no auth required)
			r.Get("/features", func(w http.ResponseWriter, r *http.Request) {
				t := tenant.FromContext(r.Context())
				if t == nil {
					writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
					return
				}
				flags := featureChecker.GetAllEnabled(r.Context(), t.ID)
				writeJSON(w, http.StatusOK, map[string]interface{}{
					"data": flags,
				})
			})

			// Custom field group routes
			// Read (require auth)
			r.With(auth.RequireAuth(kr)).Get("/custom-field-groups", cfHandlers.ListFieldGroups)
			r.With(auth.RequireAuth(kr)).Get("/custom-field-groups/{id}", cfHandlers.GetFieldGroup)
			r.With(auth.RequireAuth(kr)).Get("/custom-field-groups/{id}/fields", cfHandlers.ListFieldsByGroup)
			// Write (require auth + permission)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "custom_fields.group.create")).
				Post("/custom-field-groups", cfHandlers.CreateFieldGroup)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "custom_fields.group.edit")).
				Patch("/custom-field-groups/{id}", cfHandlers.UpdateFieldGroup)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "custom_fields.group.delete")).
				Delete("/custom-field-groups/{id}", cfHandlers.DeleteFieldGroup)

			// Custom field routes (field-level operations, separate path prefix)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "custom_fields.field.create")).
				Post("/custom-field-groups/{id}/fields", cfHandlers.CreateField)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "custom_fields.field.edit")).
				Patch("/custom-fields/{id}", cfHandlers.UpdateField)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "custom_fields.field.delete")).
				Delete("/custom-fields/{id}", cfHandlers.DeleteField)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "custom_fields.field.edit")).
				Put("/custom-fields/{id}/permissions", cfHandlers.SetFieldPermission)

			// Field value routes
			r.With(auth.RequireAuth(kr)).Get("/custom-field-values/{entity_type}/{entity_id}", cfHandlers.ListFieldValues)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "custom_fields.values.set")).
				Put("/custom-field-values", cfHandlers.SetFieldValue)

			// Form routes
			// Public submission (optional auth — anonymous users can submit forms)
			r.With(auth.OptionalAuth(kr)).Post("/forms/{id}/submissions", cfHandlers.SubmitForm)
			// Read (require auth)
			r.With(auth.RequireAuth(kr)).Get("/forms", cfHandlers.ListForms)
			r.With(auth.RequireAuth(kr)).Get("/forms/{id}", cfHandlers.GetForm)
			// Write (require auth + permission)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "forms.create")).
				Post("/forms", cfHandlers.CreateForm)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "forms.edit")).
				Patch("/forms/{id}", cfHandlers.UpdateForm)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "forms.delete")).
				Delete("/forms/{id}", cfHandlers.DeleteForm)
			// Submission listing (require auth + permission)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "forms.submissions.view")).
				Get("/forms/{id}/submissions", cfHandlers.ListFormSubmissions)

			// Space type routes
			r.With(auth.RequireAuth(kr)).Get("/space-types", spaceHandlers.ListSpaceTypes)
			r.With(auth.RequireAuth(kr)).Get("/space-types/{id}", spaceHandlers.GetSpaceType)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "space_types.create")).
				Post("/space-types", spaceHandlers.CreateSpaceType)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "space_types.edit")).
				Patch("/space-types/{id}", spaceHandlers.UpdateSpaceType)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "space_types.delete")).
				Delete("/space-types/{id}", spaceHandlers.DeleteSpaceType)

			// Space routes
			r.With(auth.RequireAuth(kr)).Get("/spaces", spaceHandlers.ListSpaces)
			r.With(auth.RequireAuth(kr), spaces.SpaceResolver(spaceService)).
				Get("/spaces/{slug}", spaceHandlers.GetSpace)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "spaces.create")).
				Post("/spaces", spaceHandlers.CreateSpace)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "spaces.edit")).
				Patch("/spaces/{id}", spaceHandlers.UpdateSpace)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "spaces.delete")).
				Delete("/spaces/{id}", spaceHandlers.DeleteSpace)
			// Space membership (join/leave toggle)
			r.With(auth.RequireAuth(kr), spaces.SpaceResolver(spaceService)).
				Post("/spaces/{slug}/join", spaceHandlers.ToggleSpaceMembership)

			// Space rule routes (slug-resolved via SpaceResolver middleware)
			r.With(auth.RequireAuth(kr), spaces.SpaceResolver(spaceService)).
				Get("/spaces/{slug}/rules", spaceHandlers.ListSpaceRules)
			r.With(auth.RequireAuth(kr), spaces.SpaceResolver(spaceService), rbac.RequirePermission(rbacResolver, "spaces.moderate")).
				Post("/spaces/{slug}/rules", spaceHandlers.CreateSpaceRule)
			r.With(auth.RequireAuth(kr), spaces.SpaceResolver(spaceService), rbac.RequirePermission(rbacResolver, "spaces.moderate")).
				Patch("/spaces/{slug}/rules/{ruleId}", spaceHandlers.UpdateSpaceRule)
			r.With(auth.RequireAuth(kr), spaces.SpaceResolver(spaceService), rbac.RequirePermission(rbacResolver, "spaces.moderate")).
				Delete("/spaces/{slug}/rules/{ruleId}", spaceHandlers.DeleteSpaceRule)

			// Permission preset routes
			r.With(auth.RequireAuth(kr)).Get("/space-permission-presets", spaceHandlers.ListPermissionPresets)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "spaces.admin")).
				Put("/space-permission-presets/{name}", spaceHandlers.UpdatePermissionPreset)

			// Content routes (Section 14 — Content Type System)
			// Create (require auth + permission)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "content.create")).
				Post("/content", contentHandlers.CreateContent)
			// Read (require auth)
			r.With(auth.RequireAuth(kr)).Get("/content", contentHandlers.ListContent)
			r.With(auth.RequireAuth(kr)).Get("/content/{slug}", contentHandlers.GetContent)
			// Update/Delete (require auth + permission)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "content.edit")).
				Patch("/content/{id}", contentHandlers.UpdateContent)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "content.delete")).
				Delete("/content/{id}", contentHandlers.DeleteContent)
			// Revisions (require auth + permission to view)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "content.revisions.view")).
				Get("/content/{id}/revisions", contentHandlers.ListRevisions)
			// Pin/Unpin (require auth + content.pin permission)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "content.pin")).
				Post("/content/{id}/pin", contentHandlers.PinContent)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "content.pin")).
				Delete("/content/{id}/pin", contentHandlers.UnpinContent)
			// Reactions (require auth — any authenticated user can react)
			r.With(auth.RequireAuth(kr)).Post("/content/{id}/reactions", contentHandlers.AddReaction)
			r.With(auth.RequireAuth(kr)).Delete("/content/{id}/reactions/{key}", contentHandlers.RemoveReaction)
			r.With(auth.RequireAuth(kr)).Get("/content/{id}/reactions", contentHandlers.ListReactions)

			// Reaction type routes (admin-configurable emoji set)
			r.With(auth.RequireAuth(kr)).Get("/reaction-types", contentHandlers.ListReactionTypes)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "content.reaction_types.manage")).
				Post("/reaction-types", contentHandlers.CreateReactionType)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "content.reaction_types.manage")).
				Patch("/reaction-types/{key}", contentHandlers.UpdateReactionType)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "content.reaction_types.manage")).
				Delete("/reaction-types/{key}", contentHandlers.DeleteReactionType)

			// JSON-LD template routes
			r.With(auth.RequireAuth(kr)).Get("/content/jsonld-templates", contentHandlers.ListJSONLDTemplates)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "content.jsonld.manage")).
				Put("/content/jsonld-templates", contentHandlers.UpsertJSONLDTemplate)

			// Page builder block types (Section 16 — Page Builder).
			// Any authenticated user can view the block registry; no
			// special permission required.
			r.With(auth.RequireAuth(kr)).Get("/page-builder/block-types", contentHandlers.ListBlockTypes)

			// Renderer routes (Section 15 — Block Editor SSR)
			r.With(auth.RequireAuth(kr)).Post("/content/render", rendererHandlers.Render)

			// Link preview routes (Section 15 — Link Preview Fetch)
			r.With(auth.RequireAuth(kr)).Get("/link-preview", linkPreviewHandlers.Fetch)

			// Follow System routes (Section 31 — Follow System)
			r.With(auth.RequireAuth(kr)).Post("/follows", followHandlers.ToggleFollow)
			r.With(auth.RequireAuth(kr)).Get("/users/{user_id}/followers", followHandlers.GetFollowers)
			r.With(auth.RequireAuth(kr)).Get("/users/{user_id}/following", followHandlers.GetFollowing)
			r.With(auth.RequireAuth(kr)).Get("/users/{user_id}/mutual/{other_user_id}", followHandlers.CheckMutual)
			r.With(auth.RequireAuth(kr)).Get("/spaces/{space_id}/followers", followHandlers.GetFollowers)
			r.With(auth.RequireAuth(kr)).Get("/tags/{tag_id}/followers", followHandlers.GetFollowers)

			// Notification System routes (Section 35 — Notification System)
			r.With(auth.RequireAuth(kr)).Get("/notifications", notificationHandlers.ListNotifications)
			r.With(auth.RequireAuth(kr)).Get("/notifications/unread-count", notificationHandlers.GetUnreadCount)
			r.With(auth.RequireAuth(kr)).Patch("/notifications/{id}/read", notificationHandlers.MarkAsRead)
			r.With(auth.RequireAuth(kr)).Post("/notifications/read-all", notificationHandlers.MarkAllAsRead)
			r.With(auth.RequireAuth(kr)).Get("/notification-preferences", notificationHandlers.GetPreferences)
			r.With(auth.RequireAuth(kr)).Put("/notification-preferences/{type}", notificationHandlers.UpdatePreference)
			r.With(auth.RequireAuth(kr)).Get("/notification-types", notificationHandlers.ListNotificationTypes)

			// Feed routes (Section 32 — Activity Feed / Timeline)
			r.Get("/feed/explore", feedHandlers.GetExploreFeed) // public — no auth required
			r.With(auth.RequireAuth(kr)).Get("/feed/primary", feedHandlers.GetPrimaryFeed)
			r.With(auth.RequireAuth(kr)).Get("/feed/space/{space_id}", feedHandlers.GetSpaceFeed)
			r.With(auth.RequireAuth(kr)).Get("/feed/profile/{user_id}", feedHandlers.GetProfileFeed)

			// WebSocket + DM routes (Section 10 — Real-Time WebSockets)
			wsHub := websocket.NewHub()
			wsService := websocket.NewService(database, wsHub)
			wsDMStore := websocket.NewDMStore(gen.New(database.Reader), gen.New(database.Writer))
			wsDMService := websocket.NewDMService(wsDMStore, wsHub)
			wsHandlers := websocket.NewHandlers(wsService, wsDMService, wsHub, vkClient, kr)

			// WebSocket
			r.With(auth.RequireAuth(kr)).Post("/auth/ws-ticket", wsHandlers.GetWSTicket)
			r.Get("/ws", wsHandlers.HandleWebSocket) // ticket auth, not Bearer
			r.With(auth.RequireAuth(kr)).Get("/ws/online", wsHandlers.GetOnlineUsers)

			// Direct Messaging
			r.With(auth.RequireAuth(kr)).Get("/dm/conversations", wsHandlers.ListConversations)
			r.With(auth.RequireAuth(kr)).Get("/dm/conversations/{id}/messages", wsHandlers.ListMessages)
			r.With(auth.RequireAuth(kr)).Post("/dm/conversations/{id}/messages", wsHandlers.SendMessage)
			r.With(auth.RequireAuth(kr)).Post("/dm/conversations/{id}/read", wsHandlers.MarkAsRead)
			r.With(auth.RequireAuth(kr)).Post("/dm/message-requests/{id}/accept", wsHandlers.AcceptMessageRequest)
			r.With(auth.RequireAuth(kr)).Post("/dm/message-requests/{id}/decline", wsHandlers.DeclineMessageRequest)
			r.With(auth.RequireAuth(kr)).Put("/dm/privacy", wsHandlers.UpdateDMPrivacy)

			// Media routes (Section 34 — Media System)
			mediaService := media.NewService(database)
			mediaHandlers := media.NewHandlers(mediaService)

			r.With(auth.RequireAuth(kr)).Post("/media/upload", mediaHandlers.InitiateUpload)
			r.With(auth.RequireAuth(kr)).Post("/media/confirm/{id}", mediaHandlers.ConfirmUpload)
			r.With(auth.RequireAuth(kr)).Get("/media/{id}", mediaHandlers.GetMedia)
			r.With(auth.RequireAuth(kr)).Patch("/media/{id}", mediaHandlers.UpdateMetadata)
			r.With(auth.RequireAuth(kr)).Delete("/media/{id}", mediaHandlers.DeleteMedia)
			r.With(auth.RequireAuth(kr)).Get("/media", mediaHandlers.ListMedia)
			r.With(auth.RequireAuth(kr)).Post("/media/bulk-delete", mediaHandlers.BulkDelete)

			// Forum Q&A / Solved routes (Section 08)
			forumService := forum.NewService(database)
			forumHandlers := forum.NewHandlers(forumService)

			// Thread routes
			r.With(auth.RequireAuth(kr)).Post("/forum/threads", forumHandlers.CreateThread)
			r.With(auth.RequireAuth(kr)).Get("/forum/threads", forumHandlers.ListThreads)
			r.With(auth.RequireAuth(kr)).Get("/forum/threads/{slug}", forumHandlers.GetThread)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "forum.thread.edit_own")).
				Patch("/forum/threads/{id}", forumHandlers.UpdateThread)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "forum.thread.delete_own")).
				Delete("/forum/threads/{id}", forumHandlers.DeleteThread)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "forum.thread.solve")).
				Post("/forum/threads/{id}/solve", forumHandlers.SolveThread)

			// Reply routes
			r.With(auth.RequireAuth(kr)).Post("/forum/threads/{id}/replies", forumHandlers.CreateReply)
			r.With(auth.RequireAuth(kr)).Get("/forum/threads/{id}/replies", forumHandlers.ListReplies)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "forum.reply.edit_own")).
				Patch("/forum/replies/{id}", forumHandlers.UpdateReply)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "forum.reply.delete_own")).
				Delete("/forum/replies/{id}", forumHandlers.DeleteReply)

			// Vote route
			r.With(auth.RequireAuth(kr)).Post("/forum/vote", forumHandlers.Vote)

			// Poll routes
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "forum.poll.create")).
				Post("/forum/polls", forumHandlers.CreatePoll)
			r.With(auth.RequireAuth(kr)).Post("/forum/polls/{id}/vote", forumHandlers.VotePoll)
			r.With(auth.RequireAuth(kr)).Get("/forum/polls/{id}", forumHandlers.GetPoll)

			// Tag routes
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "forum.tag.create")).
				Post("/forum/tags", forumHandlers.CreateTag)
			r.With(auth.RequireAuth(kr)).Get("/forum/tags", forumHandlers.ListTags)
			r.With(auth.RequireAuth(kr)).Get("/forum/tags/{slug}", forumHandlers.GetTagBySlug)
			r.With(auth.RequireAuth(kr)).Post("/forum/threads/{id}/tags/{tagId}", forumHandlers.AddThreadTag)
			r.With(auth.RequireAuth(kr)).Delete("/forum/threads/{id}/tags/{tagId}", forumHandlers.RemoveThreadTag)

			// Bookmark routes
			r.With(auth.RequireAuth(kr)).Post("/forum/bookmarks", forumHandlers.AddBookmark)
			r.With(auth.RequireAuth(kr)).Delete("/forum/bookmarks/{id}", forumHandlers.RemoveBookmark)
			r.With(auth.RequireAuth(kr)).Get("/forum/bookmarks", forumHandlers.ListBookmarks)

			// Blog Extension routes (Section 24 — Blog Extension)
			// Series routes
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "blog.series.create")).
				Post("/blog/series", blogHandlers.CreateSeries)
			r.With(auth.RequireAuth(kr)).Get("/blog/series", blogHandlers.ListSeries)
			r.With(auth.RequireAuth(kr)).Get("/blog/series/{slug}", blogHandlers.GetSeries)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "blog.series.edit")).
				Patch("/blog/series/{id}", blogHandlers.UpdateSeries)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "blog.series.delete")).
				Delete("/blog/series/{id}", blogHandlers.DeleteSeries)

			// Series post routes
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "blog.series.edit")).
				Post("/blog/series/{id}/posts", blogHandlers.AddPostToSeries)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "blog.series.edit")).
				Delete("/blog/series/{id}/posts/{postId}", blogHandlers.RemovePostFromSeries)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "blog.series.edit")).
				Patch("/blog/series/{id}/posts/reorder", blogHandlers.ReorderSeriesPosts)

			// Review routes
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "blog.review.create")).
				Post("/blog/reviews", blogHandlers.CreateReview)
			r.With(auth.RequireAuth(kr)).Get("/blog/reviews", blogHandlers.ListReviews)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "blog.review.manage")).
				Patch("/blog/reviews/{id}", blogHandlers.UpdateReview)

			// Featured routes
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "blog.feature")).
				Post("/blog/featured", blogHandlers.FeaturePost)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "blog.feature")).
				Delete("/blog/featured/{contentId}", blogHandlers.UnfeaturePost)
			r.With(auth.RequireAuth(kr)).Get("/blog/featured", blogHandlers.ListFeatured)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "blog.feature")).
				Patch("/blog/featured/reorder", blogHandlers.ReorderFeatured)

			// RSS feed (public — no auth required)
			r.Get("/blog/feed.xml", blogHandlers.GetRSSFeed)

			// Universal Ratings & Reviews routes (Section 29)
			r.With(auth.RequireAuth(kr)).Post("/reviews", reviewHandlers.CreateReview)
			r.With(auth.RequireAuth(kr)).Get("/reviews", reviewHandlers.ListReviews)
			r.With(auth.RequireAuth(kr)).Get("/reviews/aggregate", reviewHandlers.GetAggregate)
			r.With(auth.RequireAuth(kr)).Get("/reviews/{id}", reviewHandlers.GetReview)
			r.With(auth.RequireAuth(kr)).Patch("/reviews/{id}", reviewHandlers.UpdateReview)
			r.With(auth.RequireAuth(kr)).Delete("/reviews/{id}", reviewHandlers.DeleteReview)
			r.With(auth.RequireAuth(kr)).Post("/reviews/{id}/flag", reviewHandlers.FlagReview)

			// Generic Comparison System routes (Section 30)
			// Type definition routes
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "comparison.type.create")).
				Post("/comparison/types", comparisonHandlers.CreateComparisonType)
			r.With(auth.RequireAuth(kr)).Get("/comparison/types", comparisonHandlers.ListComparisonTypes)
			r.With(auth.RequireAuth(kr)).Get("/comparison/types/{slug}", comparisonHandlers.GetComparisonType)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "comparison.type.edit")).
				Patch("/comparison/types/{id}", comparisonHandlers.UpdateComparisonType)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "comparison.type.delete")).
				Delete("/comparison/types/{id}", comparisonHandlers.DeleteComparisonType)

			// Type access routes
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "comparison.type.access.manage")).
				Put("/comparison/types/{id}/access", comparisonHandlers.SetComparisonTypeAccess)
			r.With(auth.RequireAuth(kr)).Get("/comparison/types/{id}/access", comparisonHandlers.ListComparisonTypeAccess)

			// Comparison CRUD routes
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "comparison.create")).
				Post("/comparison", comparisonHandlers.CreateComparison)
			r.With(auth.RequireAuth(kr)).Get("/comparison/{id}", comparisonHandlers.GetComparison)
			r.With(auth.RequireAuth(kr)).Get("/comparison/{id}/matrix", comparisonHandlers.GetComparisonMatrix)

			// Item routes
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "comparison.item.manage")).
				Post("/comparison/{id}/items", comparisonHandlers.CreateComparisonItem)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "comparison.item.manage")).
				Patch("/comparison/items/{id}", comparisonHandlers.UpdateComparisonItem)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "comparison.item.manage")).
				Delete("/comparison/items/{id}", comparisonHandlers.DeleteComparisonItem)

			// Dimension routes
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "comparison.dimension.manage")).
				Post("/comparison/{id}/dimensions", comparisonHandlers.CreateComparisonDimension)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "comparison.dimension.manage")).
				Patch("/comparison/dimensions/{id}", comparisonHandlers.UpdateComparisonDimension)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "comparison.dimension.manage")).
				Delete("/comparison/dimensions/{id}", comparisonHandlers.DeleteComparisonDimension)

			// Value routes
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "comparison.value.manage")).
				Put("/comparison/dimensions/{id}/values", comparisonHandlers.UpsertComparisonValues)

			// Professional Profiles routes (Section 33 — Professional Profiles)
			// Profile type routes
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "professional_profile.type.create")).
				Post("/professional-profiles/types", professionalHandlers.CreateProfessionalProfileType)
			r.With(auth.RequireAuth(kr)).Get("/professional-profiles/types", professionalHandlers.ListProfessionalProfileTypes)
			r.With(auth.RequireAuth(kr)).Get("/professional-profiles/types/{slug}", professionalHandlers.GetProfessionalProfileType)

			// Profile CRUD routes
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "professional_profile.create")).
				Post("/professional-profiles", professionalHandlers.CreateProfessionalProfile)
			r.With(auth.RequireAuth(kr)).Get("/professional-profiles", professionalHandlers.ListProfessionalProfiles)
			r.With(auth.RequireAuth(kr)).Get("/professional-profiles/{slug}", professionalHandlers.GetProfessionalProfile)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "professional_profile.edit_own")).
				Patch("/professional-profiles/{id}", professionalHandlers.UpdateProfessionalProfile)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "professional_profile.delete_own")).
				Delete("/professional-profiles/{id}", professionalHandlers.DeleteProfessionalProfile)

			// Member management routes
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "professional_profile.member.manage")).
				Post("/professional-profiles/{id}/members", professionalHandlers.AddMember)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "professional_profile.member.manage")).
				Delete("/professional-profiles/{id}/members/{userId}", professionalHandlers.RemoveMember)
			r.With(auth.RequireAuth(kr)).Get("/professional-profiles/{id}/members", professionalHandlers.ListMembers)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "professional_profile.member.manage")).
				Patch("/professional-profiles/{id}/members/{userId}", professionalHandlers.UpdateMemberRole)

			// Invitation routes
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "professional_profile.invite")).
				Post("/professional-profiles/{id}/invitations", professionalHandlers.CreateInvitation)
			r.With(auth.RequireAuth(kr)).Get("/professional-profiles/{id}/invitations", professionalHandlers.ListInvitations)
			r.With(auth.RequireAuth(kr)).Post("/professional-profiles/invitations/{token}/accept", professionalHandlers.AcceptInvitation)

			// Author context route
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "professional_profile.author.switch")).
				Get("/professional-profiles/{id}/author-context", professionalHandlers.GetAuthorContext)

			// Payment Core routes (Section 17 — Payment Core)
			// Payment initiation
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "payments.create")).
				Post("/payments/initiate", paymentHandlers.InitiatePayment)
			// Webhooks (public - no auth required)
			r.Post("/payments/webhook/paystack", paymentHandlers.HandlePaystackWebhook)
			r.Post("/payments/webhook/stripe", paymentHandlers.HandleStripeWebhook)
			// Payment listing and details
			r.With(auth.RequireAuth(kr)).Get("/payments", paymentHandlers.ListPayments)
			r.With(auth.RequireAuth(kr)).Get("/payments/{id}", paymentHandlers.GetPayment)
			// Refunds
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "payments.refund")).
				Post("/payments/{id}/refund", paymentHandlers.RefundPayment)
			// Subscriptions
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "payments.subscriptions.manage")).
				Post("/subscriptions", paymentHandlers.CreateSubscription)
			r.With(auth.RequireAuth(kr)).Get("/subscriptions", paymentHandlers.ListSubscriptions)
			r.With(auth.RequireAuth(kr)).Get("/subscriptions/active", paymentHandlers.GetActiveSubscription)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "payments.subscriptions.manage")).
				Patch("/subscriptions/{id}/cancel", paymentHandlers.CancelSubscription)
			// Discount codes
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "payments.discount_codes.manage")).
				Post("/discount-codes", paymentHandlers.CreateDiscountCode)
			r.With(auth.RequireAuth(kr)).Get("/discount-codes", paymentHandlers.ListDiscountCodes)
			r.With(auth.RequireAuth(kr)).Post("/discount-codes/validate", paymentHandlers.ValidateDiscountCode)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "payments.discount_codes.manage")).
				Delete("/discount-codes/{id}", paymentHandlers.DeleteDiscountCode)

			// Wallet & Credits routes (Section 28 — Wallet & Credits)
			// Balance and ledger
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "wallet.points.balance")).
				Get("/wallet/balance", walletHandlers.GetBalance)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "wallet.points.balance")).
				Get("/wallet/ledger", walletHandlers.GetLedger)
			// Conversion
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "wallet.points.convert")).
				Post("/wallet/convert", walletHandlers.ConvertPoints)
			// Settings (admin)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "wallet.admin.settings.manage")).
				Get("/wallet/settings", walletHandlers.GetSettings)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "wallet.admin.settings.manage")).
				Put("/wallet/settings", walletHandlers.UpdateSettings)
			// Activity rules (admin)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "wallet.admin.rules.manage")).
				Get("/wallet/rules", walletHandlers.ListRules)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "wallet.admin.rules.manage")).
				Post("/wallet/rules", walletHandlers.CreateOrUpdateRule)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "wallet.admin.rules.manage")).
				Patch("/wallet/rules/{id}/toggle", walletHandlers.ToggleRule)
			// Admin adjustments
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "wallet.admin.adjust")).
				Post("/wallet/admin/adjust", walletHandlers.AdminAdjust)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "wallet.admin.adjust")).
				Get("/wallet/admin/user/{userId}/balance", walletHandlers.AdminGetUserBalance)

			// Advertising routes (Section 25 — Ad Placement)
			// Admin campaign management
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "ad.campaign.manage")).
				Post("/admin/advertising/campaigns", adHandlers.CreateCampaign)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "ad.campaign.manage")).
				Get("/admin/advertising/campaigns", adHandlers.ListCampaigns)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "ad.campaign.manage")).
				Get("/admin/advertising/campaigns/{id}", adHandlers.GetCampaign)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "ad.campaign.manage")).
				Post("/admin/advertising/campaigns/{id}/activate", adHandlers.ActivateCampaign)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "ad.campaign.manage")).
				Post("/admin/advertising/campaigns/{id}/pause", adHandlers.PauseCampaign)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "ad.campaign.manage")).
				Post("/admin/advertising/campaigns/{id}/end", adHandlers.EndCampaign)
			// Public ad serving and event tracking
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "ad.event.track")).
				Get("/ads/render", adHandlers.RenderSlot)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "ad.event.track")).
				Post("/ads/events/ingest", adHandlers.IngestEvents)
			// Slot types (admin)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "ad.slot.manage")).
				Get("/admin/advertising/slot-types", adHandlers.ListSlotTypes)
			// Organization boosts
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "ad.campaign.manage")).
				Post("/admin/advertising/boosts", adHandlers.CreateBoost)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "ad.campaign.manage")).
				Get("/admin/advertising/boosts", adHandlers.ListBoosts)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "ad.campaign.manage")).
				Get("/admin/advertising/boosts/active", adHandlers.ListActiveBoosts)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "ad.campaign.manage")).
				Post("/admin/advertising/boosts/{id}/end", adHandlers.EndBoost)

			// Ticket System routes (Section 38 — Ticket System)
			// User portal (main site /support)
			r.With(auth.RequireAuth(kr)).
				Post("/support", ticketHandlers.CreateTicket)
			r.With(auth.RequireAuth(kr)).
				Get("/support/my", ticketHandlers.ListMyTickets)
			r.With(auth.RequireAuth(kr)).
				Get("/support/my/{id}", ticketHandlers.GetMyTicket)
			r.With(auth.RequireAuth(kr)).
				Post("/support/my/{id}/reply", ticketHandlers.AddReply)

			// Admin panel (/api/v1/admin/tickets)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "tickets.admin")).
				Get("/admin/tickets", ticketHandlers.ListTickets)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "tickets.admin")).
				Get("/admin/tickets/{id}", ticketHandlers.GetTicket)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "tickets.admin")).
				Patch("/admin/tickets/{id}", ticketHandlers.UpdateTicket)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "tickets.admin")).
				Post("/admin/tickets/{id}/notes", ticketHandlers.AddInternalNote)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "tickets.admin")).
				Get("/admin/tickets/{id}/events", ticketHandlers.GetTicketEvents)

			// Ticket type management
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "tickets.type.manage")).
				Get("/admin/ticket-types", ticketHandlers.ListTicketTypes)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "tickets.type.manage")).
				Post("/admin/ticket-types", ticketHandlers.CreateTicketType)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "tickets.type.manage")).
				Put("/admin/ticket-types/{id}", ticketHandlers.UpdateTicketType)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "tickets.type.manage")).
				Delete("/admin/ticket-types/{id}", ticketHandlers.DeleteTicketType)

			// GDPR / NDPR Compliance routes (Section 19)
			// User-facing endpoints
			r.With(auth.RequireAuth(kr)).
				Post("/consent", gdprHandlers.RecordConsent)
			r.With(auth.RequireAuth(kr)).
				Get("/consent", gdprHandlers.GetUserConsents)
			r.With(auth.RequireAuth(kr)).
				Delete("/consent/{type}", gdprHandlers.RevokeConsent)
			r.With(auth.RequireAuth(kr)).
				Post("/account/export", gdprHandlers.RequestDataExport)
			r.With(auth.RequireAuth(kr)).
				Post("/account/delete", gdprHandlers.RequestAccountDeletion)

			// Admin endpoints
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "gdpr.audit_logs.view")).
				Get("/admin/audit-logs", gdprHandlers.ListAuditLogs)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "gdpr.consents.view")).
				Get("/admin/consents", gdprHandlers.ListConsentsByTenant)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "gdpr.data_exports.view")).
				Get("/admin/data-exports", gdprHandlers.ListPendingDataExports)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "gdpr.deletions.view")).
				Get("/admin/deletions", gdprHandlers.ListPendingDeletions)

			// Bot & Crawler Management routes (Section 43)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "bots.admin")).
				Get("/admin/bots", botHandlers.ListKnownBots)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "bots.admin")).
				Post("/admin/bots", botHandlers.CreateKnownBot)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "bots.admin")).
				Put("/admin/bots/{id}", botHandlers.UpdateKnownBot)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "bots.admin")).
				Delete("/admin/bots/{id}", botHandlers.DeleteKnownBot)

			// Platform Analytics & Site Reports routes (Section 40)
			// User dashboard
			r.With(auth.RequireAuth(kr)).
				Post("/analytics/events", analyticsHandlers.TrackEvent)
			r.With(auth.RequireAuth(kr)).
				Get("/analytics/dashboard", analyticsHandlers.GetUserDashboard)

			// Admin endpoints
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "analytics.admin")).
				Get("/admin/analytics/overview", analyticsHandlers.GetAdminOverview)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "analytics.admin")).
				Get("/admin/analytics/events", analyticsHandlers.ListEvents)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "analytics.admin")).
				Get("/admin/analytics/security", analyticsHandlers.ListSecurityScanResults)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "analytics.admin")).
				Post("/admin/analytics/security/{id}/resolve", analyticsHandlers.ResolveSecurityScanResult)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "analytics.admin")).
				Post("/admin/analytics/refresh", analyticsHandlers.RefreshViews)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "analytics.admin")).
				Post("/admin/analytics/security/scan", analyticsHandlers.RunSecurityScan)
			r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "analytics.cache.flush")).
				Post("/admin/analytics/cache/flush", func(w http.ResponseWriter, r *http.Request) {
					var input struct {
						Scope string `json:"scope"`
					}
					if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
						writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
						return
					}
					scope := analytics.FlushScope(input.Scope)
					if scope == "" {
						scope = analytics.FlushAll
					}
					deleted, err := cacheFlusher.FlushCache(r.Context(), scope)
					if err != nil {
						writeError(w, http.StatusInternalServerError, "flush_error", err.Error())
						return
					}
					writeJSON(w, http.StatusOK, map[string]any{
						"data": map[string]any{
							"status":  "flushed",
							"scope":   scope,
							"deleted": deleted,
						},
					})
				})

		// User Profile & Preferences routes
		r.With(auth.RequireAuth(kr)).Get("/users/me", usersHandlers.GetMyProfile)
		r.With(auth.RequireAuth(kr)).Patch("/users/me", usersHandlers.UpdateMyProfile)
		r.With(auth.RequireAuth(kr)).Get("/users/me/preferences", usersHandlers.GetMyPreferences)
		r.With(auth.RequireAuth(kr)).Put("/users/me/preferences", usersHandlers.UpdateMyPreferences)
		r.With(auth.RequireAuth(kr)).Get("/users", usersHandlers.ListUsers)
		r.With(auth.RequireAuth(kr)).Get("/users/{username}", usersHandlers.GetUserByUsername)

		// Onboarding routes (Section 09)
		r.With(auth.RequireAuth(kr)).Get("/onboarding/status", onboardingHandlers.GetStatus)
		r.With(auth.RequireAuth(kr)).Put("/onboarding/step", onboardingHandlers.UpdateStep)
		r.With(auth.RequireAuth(kr)).Post("/onboarding/complete", onboardingHandlers.Complete)
		r.With(auth.RequireAuth(kr)).Get("/onboarding/recommendations", onboardingHandlers.GetRecommendations)
		r.With(auth.RequireAuth(kr)).Post("/onboarding/tour/dismiss", onboardingHandlers.DismissTour)

		// Search routes
		r.With(auth.RequireAuth(kr)).Get("/search", searchHandlers.Search)

		// Documentation System routes (Section 44)
		// Public: read published pages for an audience
		r.Get("/docs/{audience}", docsHandlers.ListDocPages)
		r.Get("/docs/{audience}/{slug}", docsHandlers.GetDocPage)

		// Admin: manage documentation pages
		r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "docs.admin")).
			Get("/admin/docs/{audience}", docsHandlers.ListAllDocPages)
		r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "docs.admin")).
			Post("/admin/docs", docsHandlers.CreateDocPage)
		r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "docs.admin")).
			Patch("/admin/docs/{id}", docsHandlers.UpdateDocPage)
		r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "docs.admin")).
			Delete("/admin/docs/{id}", docsHandlers.DeleteDocPage)

		// Platform Settings (admin)
		r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "admin.settings")).
			Get("/admin/settings", adminHandlers.ListSettings)
		r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "admin.settings")).
			Put("/admin/settings/{category}", adminHandlers.UpdateSettings)

		// Admin Moderation routes (stubs)
		r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "admin.moderation")).
			Get("/admin/moderation", adminHandlers.ListModeration)
		r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "admin.moderation")).
			Post("/admin/moderation/bulk", adminHandlers.BulkModeration)
		r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "admin.moderation")).
			Post("/admin/moderation/{id}/appeal", adminHandlers.AppealModeration)

		// Admin Broadcast route (stub)
		r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "admin.broadcast")).
			Post("/admin/broadcast", adminHandlers.Broadcast)
	})
})

	return r
}

// noopBruteForceStore is a no-op implementation for development.
// In production this is replaced by a Valkey-backed store.
type noopBruteForceStore struct{}

func (n *noopBruteForceStore) GetFailCount(ctx context.Context, key string) (int, error) {
	return 0, nil
}
func (n *noopBruteForceStore) IncrFailCount(ctx context.Context, key string) error {
	return nil
}
func (n *noopBruteForceStore) ResetFailCount(ctx context.Context, key string) error {
	return nil
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
		// Platform Settings (admin)
		{Key: "admin.settings", Description: "Manage platform settings"},
		// Admin Moderation
		{Key: "admin.moderation", Description: "Manage content moderation queue"},
		// Admin Broadcast
		{Key: "admin.broadcast", Description: "Send broadcast messages to users"},
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
// gracefully drain in-flight tasks and close the Redis connection.
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
