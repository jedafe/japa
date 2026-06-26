package server

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/features"
	"github.com/jedafe/japa/internal/geoip"
	"github.com/jedafe/japa/internal/linkpreview"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/jedafe/japa/internal/renderer"
	"github.com/jedafe/japa/internal/spaces"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/valkey"
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

	// Infrastructure routes bypass tenant middleware
	r.Get("/health", h.HealthCheck)
	r.Get("/ready", h.ReadinessCheck)

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
