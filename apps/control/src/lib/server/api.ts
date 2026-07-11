import { env } from '$env/dynamic/private';

/**
 * Server-only base URL for the Go backend API.
 * Configured via PRIVATE_API_BASE_URL environment variable.
 * NOTE: /control routes are mounted at the top level (not under /api/v1),
 * so this base URL must NOT include /api/v1.
 */
export const API_BASE = env.PRIVATE_API_BASE_URL ?? 'http://localhost:8080';

/** Base URL with /api/v1 prefix for routes that live under the API group (auth, me, i18n). */
export const API_V1_BASE = `${API_BASE}/api/v1`;
