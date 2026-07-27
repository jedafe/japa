import { apiGet, apiList, type ApiContext } from '$lib/server/api';
import type { MeProfile, PrivacySettings, AffiliateEarnings, GeoCountry } from '$lib/types';

/**
 * Server-only loader registry — one entry per settings section that needs
 * server-fetched data. Kept separate from the client meta registry so server
 * fetch code stays out of the browser bundle (the two-registry split).
 *
 * Each loader degrades independently — a missing/unavailable sub-fetch falls
 * back to a safe default rather than blanking the whole section.
 *
 * Ref: japaconnect-engineering/sections/23-main-site-frontend.md (Settings hub)
 */
export const SETTINGS_LOADERS: Record<string, (ctx: ApiContext) => Promise<unknown>> = {
	profile: (ctx) => apiGet<MeProfile>(ctx, '/me'),
	account: (ctx) => apiGet<MeProfile>(ctx, '/me'),
	privacy: (ctx) => apiGet<PrivacySettings>(ctx, '/me/privacy'),

	language: async (ctx) => ({
		// GET /i18n/packs — Rule 16: language list from API, not hardcoded.
		packs: (await apiList<unknown>(ctx, '/i18n/packs')).items,
		current: await apiGet<unknown>(ctx, '/me/account').catch(() => ({})),
		// GET /geo/countries — country list for the content-country preference
		// selector (ADR-064). Best-effort: empty list when unavailable.
		countries: (await apiList<GeoCountry>(ctx, '/geo/countries').then((r) => r.items).catch(() => []))
	}),

	'data-privacy': async (ctx) => ({
		// GET /consent — consent records (Section 19).
		consents: (await apiList<unknown>(ctx, '/consent')).items,
		// GET /me/data-export/status — current export status (none|pending|ready).
		exportStatus: await apiGet<unknown>(ctx, '/me/data-export/status').catch(() => ({ status: 'none' }))
	}),

	security: async (ctx) => ({
		totp: await apiGet<{ enabled: boolean }>(ctx, '/me/totp/status').catch(() => ({ enabled: false }))
	}),
	sessions: async (ctx) => ({
		sessions: (await apiList<unknown>(ctx, '/me/sessions')).items
	}),
	connected: async (ctx) => (await apiList<unknown>(ctx, '/me/connected-accounts')).items,
	notifications: (ctx) => apiGet<Record<string, unknown>>(ctx, '/me/notification-preferences'),
	'api-keys': async (ctx) => (await apiList<unknown>(ctx, '/me/api-keys')).items,
	affiliate: async (ctx) => ({
		// GET /me/affiliate-links — the caller's affiliate links (CRUD list).
		links: (await apiList<unknown>(ctx, '/me/affiliate-links')).items,
		// GET /me/affiliate-earnings — aggregate earnings summary (clicks,
		// conversions, total earned, by-product breakdown, recent events).
		// Best-effort: degrades to null when the endpoint is unavailable so
		// the links list still renders.
		earnings: await apiGet<AffiliateEarnings>(ctx, '/me/affiliate-earnings').catch(() => null)
	}),
	wallet: async (ctx) => ({
		wallet: await apiGet<unknown>(ctx, '/me/wallet'),
		ledger: (await apiList<unknown>(ctx, '/me/wallet/ledger')).items
	}),
	subscription: (ctx) => apiGet<unknown>(ctx, '/me/subscription'),
	danger: async () => null
};
