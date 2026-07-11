import { env } from '$env/dynamic/private';

/**
 * Server-only base URL for the Go backend API (origin only, no /api/v1).
 * Configured via PRIVATE_API_BASE_URL environment variable.
 */
export const API_BASE = env.PRIVATE_API_BASE_URL ?? 'http://localhost:8080';

/** Base URL with /api/v1 prefix for API routes. */
export const API_V1_BASE = `${API_BASE}/api/v1`;
