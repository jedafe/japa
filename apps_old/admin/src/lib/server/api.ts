import { env } from '$env/dynamic/private';

/**
 * Server-only base URL for the Go backend API.
 * Configured via PRIVATE_API_BASE_URL environment variable.
 */
export const API_BASE = env.PRIVATE_API_BASE_URL ?? 'http://localhost:8080/api/v1';
