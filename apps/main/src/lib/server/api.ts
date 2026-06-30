import { env } from '$env/dynamic/private';
import type { FeatureFlags } from '$lib/types';

/**
 * Base URL for the internal Go backend API.
 * Configured via the PRIVATE_API_BASE_URL environment variable.
 */
export const API_BASE_URL = env.PRIVATE_API_BASE_URL ?? 'http://localhost:8080';

export interface ApiResponse<T> {
	ok: boolean;
	data?: T;
	error?: string;
}

/**
 * Server-only fetch helper for the Go backend.
 *
 * This module MUST stay in `src/lib/server` so SvelteKit guarantees it never
 * reaches client bundles. Never re-export server secrets to the frontend.
 */
export async function apiFetch<T>(
	path: string,
	init: RequestInit = {},
	token?: string
): Promise<ApiResponse<T>> {
	const res = await fetch(`${API_BASE_URL}${path}`, {
		...init,
		headers: {
			'content-type': 'application/json',
			...(token ? { Authorization: `Bearer ${token}` } : {}),
			...((init.headers as Record<string, string>) ?? {})
		}
	});

	if (!res.ok) {
		return { ok: false, error: `API request failed: ${res.status}` };
	}

	try {
		const data = (await res.json()) as T;
		return { ok: true, data };
	} catch {
		return { ok: false, error: 'Invalid JSON response' };
	}
}

/** Fetch the platform feature flags from the Go backend. */
export async function getFeatureFlags(): Promise<FeatureFlags> {
	const result = await apiFetch<FeatureFlags>('/features');
	return result.ok && result.data ? result.data : {};
}