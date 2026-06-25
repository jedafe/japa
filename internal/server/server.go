package server

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/features"
	"github.com/jedafe/japa/internal/geoip"
	"github.com/jedafe/japa/internal/tenant"
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

			// Protected routes (require Bearer token)
			r.Group(func(r chi.Router) {
				r.Use(auth.RequireAuth(kr))
				// Future protected routes go here
			})
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
