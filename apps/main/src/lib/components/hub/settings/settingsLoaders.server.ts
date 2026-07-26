import { apiGet, apiList, type ApiContext } from '$lib/server/api';
import type { MeProfile, PrivacySettings } from '$lib/types';

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

	security: async (ctx) => ({
		totp: await apiGet<{ enabled: boolean }>(ctx, '/me/totp/status').catch(() => ({ enabled: false }))
	}),
	sessions: async (ctx) => ({
		sessions: (await apiList<unknown>(ctx, '/me/sessions')).items
	}),
	connected: async (ctx) => (await apiList<unknown>(ctx, '/me/connected-accounts')).items,
	notifications: (ctx) => apiGet<Record<string, unknown>>(ctx, '/me/notification-preferences'),
	'api-keys': async (ctx) => (await apiList<unknown>(ctx, '/me/api-keys')).items,
	affiliate: async (ctx) => (await apiList<unknown>(ctx, '/me/affiliate-links')).items,
	wallet: async (ctx) => ({
		wallet: await apiGet<unknown>(ctx, '/me/wallet'),
		ledger: (await apiList<unknown>(ctx, '/me/wallet/ledger')).items
	}),
	subscription: (ctx) => apiGet<unknown>(ctx, '/me/subscription'),
	danger: async () => null
};
