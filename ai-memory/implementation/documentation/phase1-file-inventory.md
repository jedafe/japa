# Phase 1 File Inventory

**Date:** 2026-06-25
**Phase:** 1
**Purpose:** Complete inventory of every file created or modified during Phase 1.

## Go Backend Source Files

### Entry Points

| File | Purpose |
|------|---------|
| `cmd/api/main.go` | API server with graceful shutdown |
| `cmd/migrate/main.go` | Migration runner |

### Config

| File | Purpose |
|------|---------|
| `internal/config/config.go` | JC_ env var loading with defaults |
| `internal/config/config_test.go` | Config tests |

### Database

| File | Purpose |
|------|---------|
| `internal/db/pool.go` | pgxpool writer/reader (MaxConns 25, MinConns 5) |
| `internal/db/pool_test.go` | Integration tests |

### Server

| File | Purpose |
|------|---------|
| `internal/server/server.go` | Chi router setup, middleware wiring, GeoIP integration |
| `internal/server/routes.go` | Route registration |
| `internal/server/middleware.go` | Request ID, logging, recovery, CORS |
| `internal/server/middleware_test.go` | Middleware tests |
| `internal/server/handlers.go` | Health/readiness handlers |
| `internal/server/handlers_test.go` | Handler tests |

### Tenant

| File | Purpose |
|------|---------|
| `internal/tenant/tenant.go` | Tenant middleware, context helpers (FromContext, IDFromContext) |
| `internal/tenant/store.go` | DBStore implementation, TenantStore interface |
| `internal/tenant/tenant_test.go` | Tenant tests |

### Auth

| File | Purpose |
|------|---------|
| `internal/auth/jwt.go` | JWT minting/validation with multi-kid rotation (HS256, 15-min TTL) |
| `internal/auth/jwt_test.go` | JWT tests (5 test functions) |
| `internal/auth/password.go` | bcrypt cost 12, password policy validation |
| `internal/auth/password_test.go` | Password tests (3 test functions) |
| `internal/auth/bruteforce.go` | Brute-force protection (exponential backoff, account+IP keyed) |
| `internal/auth/bruteforce_test.go` | Brute-force tests |
| `internal/auth/handlers.go` | Register, Login, Refresh, Logout HTTP handlers |
| `internal/auth/tokens.go` | Refresh token generation, SHA-256 hashing |
| `internal/auth/tokens_test.go` | Token tests |
| `internal/auth/middleware.go` | RequireAuth, OptionalAuth middleware |
| `internal/auth/middleware_test.go` | Auth middleware tests (6 test functions) |
| `internal/auth/store.go` | DBStore implementing Querier interface |

### RBAC

| File | Purpose |
|------|---------|
| `internal/rbac/rbac.go` | HasPermission with per-request cache, UserContext, 6-table JOIN |
| `internal/rbac/store.go` | CRUD: CreateGroup, DeleteGroup, AddUserToGroup, RegisterPermission, AssignPermissionToRole |
| `internal/rbac/middleware.go` | RequirePermission middleware (401 vs 403), FromContext helper |
| `internal/rbac/rbac_test.go` | RBAC tests (4 functions, 10 subtests) |

### Features

| File | Purpose |
|------|---------|
| `internal/features/registry.go` | Feature registry with 18 known features, DefaultFor and AllKeys helpers |
| `internal/features/features.go` | Checker with per-request cache, IsEnabled, GetAllEnabled, SetEnabled, Gate middleware |
| `internal/features/features_test.go` | Feature flag tests (5 test functions) |

### GeoIP

| File | Purpose |
|------|---------|
| `internal/geoip/geoip.go` | Reader with atomic.Pointer swap, GeoContext struct, FromContext helper |
| `internal/geoip/middleware.go` | GeoIP middleware (extracts real IP from X-Forwarded-For, X-Real-IP, RemoteAddr) |
| `internal/geoip/update.go` | Updater with DownloadAndUpdate (extracts .mmdb from .tar.gz, atomic swap) |
| `internal/geoip/geoip_test.go` | GeoIP tests (7 test functions) |

## Migration Files

| # | File | Tables / Extensions |
|---|------|---------------------|
| 1 | `migrations/000001_enable_extensions.sql` | uuid-ossp, vector (pgvector), pg_stat_statements, pg_trgm |
| 2 | `migrations/000002_create_tenants_table.sql` | tenants (with default tenant seed) |
| 3 | `migrations/000003_create_users_table.sql` | users, user_profiles (tenant-scoped) |
| 4 | `migrations/000004_create_system_settings.sql` | system_settings |
| 5 | `migrations/000005_create_auth_tables.sql` | refresh_tokens, totp_credentials, oauth_accounts (tenant-scoped) |
| 6 | `migrations/000006_create_rbac_tables.sql` | user_groups, user_roles, permissions, role_permissions, group_roles, user_group_memberships (with system group seeds) |
| 7 | `migrations/000007_create_feature_flags.sql` | feature_flags (with 18 seeded default flags) |

## Infrastructure Files

| File | Purpose |
|------|---------|
| `Dockerfile` | Multi-stage Go build |
| `docker-compose.yml` | 8 services: postgres, valkey, meilisearch, minio, minio-init, imgproxy, mailpit, go-api, nginx |
| `sqlc.yaml` | sqlc configuration |
| `Makefile` | Build, test, migrate, docker targets |
| `.env.example` | Example environment variables (JC_ prefix) |
| `.gitignore` | Go, Bun, editor, OS artifacts |
| `docker/nginx.conf` | Reverse proxy configuration |
| `docker/postgres-init.sql` | PostgreSQL initialization script |
| `.dockerignore` | Docker build exclusions |
| `.env` | Local development environment variables (JC_ prefix, auto-loaded by Docker Compose) |

## SvelteKit Apps (5 Apps)

Each app contains the following baseline files:

| File | Purpose |
|------|---------|
| `package.json` | Dependencies, scripts (dev, build, preview, test) |
| `svelte.config.js` | SvelteKit config with adapter |
| `vite.config.ts` | Vite config with Tailwind plugin, resolve.conditions |
| `src/app.css` | Tailwind import + dark mode CSS custom properties |
| `src/routes/+page.svelte` | Landing/placeholder page |
| `src/routes/+layout.svelte` | Root layout |
| `Dockerfile` | Container build for the app |
| `src/routes/page.test.ts` | Basic page render test (vitest) |

### App-Specific Files

| App | Port | Adapter | Extra Files |
|-----|------|---------|-------------|
| `apps/main` | 3000 | adapter-node (SSR) | `src/lib/server/api.ts`, `src/lib/types.ts`, `src/routes/+layout.server.ts` |
| `apps/admin` | 3001 | adapter-static (SPA) | `src/routes/+layout.server.ts` (auth check placeholder) |
| `apps/control` | 3002 | adapter-static (SPA) | - |
| `apps/learn` | 3003 | adapter-node (SSR) | - |
| `apps/docs` | 3004 | adapter-node (SSR) | - |

## File Count Summary

| Category | Count |
|----------|-------|
| Go source files | 30 |
| Migration files | 7 |
| Infrastructure files | 10 |
| SvelteKit app files | 40+ (8+ per app x 5 apps) |
| **Total** | **86+** |
