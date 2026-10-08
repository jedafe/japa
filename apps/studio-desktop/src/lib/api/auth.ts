/**
 * Desktop studio authentication.
 * Exchanges the configured API key for a JWT access/refresh token pair
 * and keeps the access token fresh.
 *
 * Ref: sections/105-japa-content-studio-desktop.md
 */

import { ensureSettingsLoaded, loadSettings, saveSettings } from '$lib/services/settings';
import type { StudioSettings } from '$lib/services/settings';

export interface ExchangeKeyResponse {
	access_token: string;
	refresh_token: string;
	token_type?: string;
	expires_in?: number;
}

function baseUrl(settings: StudioSettings): string {
	return settings.baseUrl.replace(/\/$/, '');
}

function deviceId(): string {
	const settings = loadSettings();
	if (settings.deviceId) return settings.deviceId;
	const id = crypto.randomUUID ? crypto.randomUUID() : `${Date.now()}-${Math.random().toString(36).slice(2)}`;
	void saveSettings({ ...settings, deviceId: id });
	return id;
}

function headers(): Record<string, string> {
	return {
		'Content-Type': 'application/json',
		'X-Device-Id': deviceId(),
		// Identify the desktop surface for scoped feature-flag resolution
		// (X-JC-Surface; valid values in internal/features/eval.go).
		'X-JC-Surface': 'desktop'
	};
}

async function apiFetch<T>(url: string, init: RequestInit): Promise<{ ok: boolean; data?: T; error?: { code: string; message: string } }> {
	const res = await fetch(url, init);
	let body: unknown = {};
	try {
		body = await res.json();
	} catch {
		body = {};
	}
	if (!res.ok) {
		const err = body as { error?: { code: string; message: string } };
		return { ok: false, error: err.error ?? { code: 'unknown_error', message: res.statusText } };
	}
	return { ok: true, data: body as T };
}

export async function exchangeApiKey(): Promise<ExchangeKeyResponse | null> {
	await ensureSettingsLoaded();
	const settings = loadSettings();
	if (!settings.apiKey || !settings.baseUrl) return null;

	const res = await apiFetch<ExchangeKeyResponse>(`${baseUrl(settings)}/api/v1/auth/exchange-key`, {
		method: 'POST',
		headers: {
			...headers(),
			'X-API-Key': settings.apiKey
		}
	});

	if (res.ok && res.data) {
		await persistTokens(res.data);
		return res.data;
	}

	return null;
}

export async function refreshAccessToken(): Promise<ExchangeKeyResponse | null> {
	await ensureSettingsLoaded();
	const settings = loadSettings();
	if (!settings.refreshToken || !settings.baseUrl) return null;

	// /auth/refresh returns a Japa envelope { data: { access_token, refresh_token } }.
	// /auth/exchange-key returns the object directly, so this call uses a different shape.
	const res = await apiFetch<{ data: ExchangeKeyResponse }>(
		`${baseUrl(settings)}/api/v1/auth/refresh`,
		{
			method: 'POST',
			headers: headers(),
			body: JSON.stringify({ refresh_token: settings.refreshToken })
		}
	);

	if (res.ok && res.data?.data) {
		await persistTokens(res.data.data);
		return res.data.data;
	}

	// Refresh failed; try exchanging the API key again.
	return exchangeApiKey();
}

function getTokenExpiresAt(token: string, fallbackSeconds: number): number {
	if (typeof token !== 'string' || token === '') return Date.now() + fallbackSeconds * 1000;
	try {
		const payload = token.split('.')[1];
		if (!payload) return Date.now() + fallbackSeconds * 1000;
		const decoded = atob(payload.replace(/-/g, '+').replace(/_/g, '/'));
		const claims = JSON.parse(decoded) as { exp?: number };
		if (claims.exp) return claims.exp * 1000;
	} catch {
		// ignore decoding errors
	}
	return Date.now() + fallbackSeconds * 1000;
}

async function persistTokens(data: ExchangeKeyResponse): Promise<void> {
	const settings = loadSettings();
	const fallbackSeconds = data.expires_in ?? 15 * 60;
	const expiresAt = getTokenExpiresAt(data.access_token, fallbackSeconds);
	await saveSettings({
		...settings,
		accessToken: data.access_token,
		refreshToken: data.refresh_token,
		tokenExpiresAt: expiresAt
	});
}

let refreshPromise: Promise<string | null> | null = null;

/**
 * Returns a valid access token, exchanging or refreshing as needed.
 * Safe to call from multiple concurrent requests: it coalesces in-flight
 * refresh attempts behind a single promise.
 */
export async function getAccessToken(): Promise<string | null> {
	await ensureSettingsLoaded();
	if (refreshPromise) return refreshPromise;

	const settings = loadSettings();

	// Re-use an existing token until one minute before expiry.
	if (settings.accessToken && settings.tokenExpiresAt && settings.tokenExpiresAt > Date.now() + 60_000) {
		return settings.accessToken;
	}

	// No token, expired, or about to expire: refresh or exchange.
	refreshPromise = (async (): Promise<string | null> => {
		let data: ExchangeKeyResponse | null = null;
		if (settings.refreshToken) {
			data = await refreshAccessToken();
		}
		if (!data) {
			data = await exchangeApiKey();
		}
		return data?.access_token ?? null;
	})();

	try {
		return await refreshPromise;
	} finally {
		refreshPromise = null;
	}
}

/**
 * Forces a full re-auth (refresh or exchange) and returns true if a new
 * access token was obtained. Useful from the settings UI after rotating keys.
 */
export async function refreshAuth(): Promise<boolean> {
	await clearTokens();
	const token = await getAccessToken();
	return token !== null;
}

/**
 * Clears any cached tokens, forcing the next request to re-authenticate.
 */
export async function clearTokens(): Promise<void> {
	const settings = loadSettings();
	await saveSettings({
		...settings,
		accessToken: undefined,
		refreshToken: undefined,
		tokenExpiresAt: undefined
	});
}
