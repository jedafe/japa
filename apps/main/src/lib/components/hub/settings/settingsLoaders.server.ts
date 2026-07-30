import { apiGet, apiList, type ApiContext } from '$lib/server/api';
import type { MeProfile, PrivacySettings, AffiliateEarnings, GeoCountry, NotificationPreference } from '$lib/types';

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
		// Returns string[] of locale codes (e.g. ["en", "fr"]).
		packs: (await apiList<unknown>(ctx, '/i18n/packs')).items,
		// GET /me/preferences — current language preference ({ language, dark_mode }).
		// GET /me — current timezone and content_country_preference.
		// Both best-effort: degrade to empty {} when unavailable.
		prefs: await apiGet<unknown>(ctx, '/me/preferences').catch(() => ({})),
		profile: await apiGet<unknown>(ctx, '/me').catch(() => ({})),
		// GET /geo/countries — country list for the content-country preference
		// selector (ADR-064). Best-effort: empty list when unavailable.
		countries: (await apiList<GeoCountry>(ctx, '/geo/countries').then((r) => r.items).catch(() => []))
	}),

	'data-privacy': async (ctx) => ({
		// GET /consent — consent records (Section 19). May be null when no
		// records exist yet; default to empty array.
		consents: (await apiList<unknown>(ctx, '/consent').catch(() => ({ items: [] }))).items ?? [],
		// GET /me/data-export/status — current export status.
		// Backend returns { status: "no_pending_export" } or the full
		// DataExportRequest struct with status: pending|processing|completed|failed.
		exportStatus: await apiGet<unknown>(ctx, '/me/data-export/status').catch(() => ({ status: 'no_pending_export' }))
	}),

	security: async (ctx) => ({
		totp: await apiGet<{ enabled: boolean }>(ctx, '/me/totp/status').catch(() => ({ enabled: false }))
	}),
	sessions: async (ctx) => ({
		sessions: (await apiList<unknown>(ctx, '/me/sessions')).items
	}),
	connected: async (ctx) => (await apiList<unknown>(ctx, '/me/connected-accounts')).items,
	notifications: (ctx) =>
		apiGet<NotificationPreference[]>(ctx, '/me/notification-preferences'),
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
	subscription: (ctx) => apiGet<unknown>(ctx, '/me/subscription').catch(() => null),
	danger: async () => null
};
