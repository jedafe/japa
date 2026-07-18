import { env } from '$env/dynamic/private';

/**
 * Server-only base URL for the Go backend API.
 * Configured via PRIVATE_API_BASE_URL environment variable.
 * Ref: internal/server/server.go — Control Panel routes are mounted under
 * /api/v1/control/* (inside the /api/v1 chi group), so callers must prepend
 * /api/v1 for control endpoints too.
 */
export const API_BASE = env.PRIVATE_API_BASE_URL ?? 'http://localhost:8080';

/** Base URL with /api/v1 prefix for routes that live under the API group (auth, me, i18n). */
export const API_V1_BASE = `${API_BASE}/api/v1`;
