import { apiGet, apiList, ApiError, type ApiContext } from '$lib/server/api';
import type { MeProfile, PrivacySettings, AffiliateEarnings, GeoCountry, NotificationPreference, MentorProfile, Payment, DeviceInfo } from '$lib/types';
import type { AIPrivacyStatus } from '$lib/ai/agentTypes';

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
	profile: (ctx) => apiGet<MeProfile>(ctx, '/me').catch(() => null),
	account: (ctx) => apiGet<MeProfile>(ctx, '/me').catch(() => null),
	privacy: async (ctx) => ({
		settings: await apiGet<PrivacySettings>(ctx, '/me/privacy').catch(() => null),
		// Rule 16: allowed enum values come from the API, not hardcoded UI arrays.
		options: await apiGet<{ dm_privacy: string[]; profile_visibility: string[] }>(ctx, '/me/privacy/options')
			.catch(() => ({ dm_privacy: [] as string[], profile_visibility: [] as string[] }))
	}),

	// ADR-064a: location settings needs the user's profile (for
	// origin_country/destination_country custom fields) and the full
	// country list for the cascading picker. Both best-effort: a failure
	// degrades to empty values so the section still renders.
	location: async (ctx) => ({
		profile: await apiGet<MeProfile>(ctx, '/me').catch(() => ({})),
		countries: (await apiList<GeoCountry>(ctx, '/geo/countries').then((r) => r.items).catch(() => []))
	}),

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
		// GET /consent/config — valid consent types and current policy version
		// (Rule 16: never hardcode platform consent types). Falls back to
		// empty values so the client can retry without showing stale defaults.
		consentConfig: await apiGet<{ consent_types: string[]; policy_version: string }>(ctx, '/consent/config')
			.catch(() => ({ consent_types: [] as string[], policy_version: '' })),
		// GET /me/data-export/status — current export status.
		// Backend returns { status: "no_pending_export" } or the full
		// DataExportRequest struct with status: pending|processing|completed|failed.
		exportStatus: await apiGet<unknown>(ctx, '/me/data-export/status').catch(() => ({ status: 'no_pending_export' }))
	}),

	security: async (ctx) => ({
		totp: await apiGet<{ enabled: boolean }>(ctx, '/me/totp/status').catch(() => ({ enabled: false }))
	}),
	sessions: async (ctx) => ({
		sessions: (await apiList<unknown>(ctx, '/me/sessions').catch(() => ({ items: [] }))).items
	}),
	connected: async (ctx) => (await apiList<unknown>(ctx, '/me/connected-accounts').catch(() => ({ items: [] }))).items,
	// §120 connected apps — the caller's OAuth agent grants. Best-effort:
	// degrades to an empty list when the endpoint is unavailable.
	'connected-apps': async (ctx) => (await apiList<unknown>(ctx, '/me/oauth/grants').catch(() => ({ items: [] }))).items,

	// AI section — merged loader feeding the privacy + BYOK key tabs.
	// BYOK half: provider storefront + caller's keys, both best-effort.
	// Privacy half: /me/ai-privacy + /me (the tab resolves the user id for
	// PUT /me/ai-privacy).
	ai: async (ctx) => ({
		privacy: {
			privacy: await apiGet<AIPrivacyStatus>(ctx, '/me/ai-privacy').catch(() => ({ profile_ai_exempt: false })),
			profile: await apiGet<MeProfile>(ctx, '/me').catch(() => ({ id: '' }))
		},
		keys: {
			providers: await apiGet<unknown>(ctx, '/ai/byok/providers').catch(() => []),
			keys: await apiGet<unknown>(ctx, '/me/ai-keys').catch(() => [])
		}
	}),
	// Section 77 / ADR-074 — registered mobile push devices (Devices settings section).
	// Best-effort: degrades to an empty list when the route is unavailable.
	devices: async (ctx) => ({
		devices: await apiGet<DeviceInfo[]>(ctx, '/me/devices').catch(() => []),
		// Section 35/50 — web push subscriptions. Loaded here because the
		// Devices section manages all push-capable clients.
		pushTokens: await apiGet<Array<{ id: string; endpoint: string }>>(ctx, '/me/push-tokens').catch(() => [])
	}),
	notifications: (ctx) =>
		apiGet<NotificationPreference[]>(ctx, '/me/notification-preferences').catch(() => []),
	// Section 62 / ADR-080: newsletter subscriber preferences (digest +
	// campaign toggles). Best-effort: degrades to null when the user is not
	// subscribed or the feature is disabled, so the section can render a
	// subscribe CTA instead of the toggles.
	newsletter: async (ctx) => ({
		preferences: await apiGet<unknown>(ctx, '/newsletter/preferences').catch(() => null)
	}),
	'api-keys': async (ctx) => (await apiList<unknown>(ctx, '/me/api-keys').catch(() => ({ items: [] }))).items,
	affiliate: async (ctx) => {
		// GET /affiliate-program — public program info (commission rates, T&Cs,
		// eligibility). Always available so the enrollment CTA can render even
		// before the user enrolls (Section 56).
		const program = await apiGet<unknown>(ctx, '/affiliate-program').catch(() => null);

		// GET /me/affiliate-links — the caller's affiliate links (CRUD list).
		// A 403 means the user has not enrolled yet (affiliate.links.manage is
		// granted on enrollment) — we flag `enrolled: false` so the section can
		// show an enrollment CTA instead of the create form. Page 1 of
		// page_size=20; the section loads further pages client-side (the
		// endpoint is page/page_size-paginated but returns no total, so the
		// client infers "more" from a full page).
		let enrolled = true;
		let links: unknown[] = [];
		try {
			links = (await apiList<unknown>(ctx, '/me/affiliate-links?page=1&page_size=20')).items;
		} catch (e) {
			if (e instanceof ApiError && e.status === 403) enrolled = false;
		}

		// GET /me/affiliate-earnings — aggregate earnings summary. Best-effort:
		// degrades to null when unavailable or the user is not enrolled.
		const earnings = enrolled
			? await apiGet<AffiliateEarnings>(ctx, '/me/affiliate-earnings').catch(() => null)
			: null;

		return { program, links, earnings, enrolled, linksPageSize: 20 };
	},
	wallet: async (ctx) => {
		const walletSettings = await apiGet<{
			settings?: { enable_earning?: boolean; daily_earning_cap?: number };
			min_purchase_credits?: number;
			min_withdrawal_credits?: number;
		}>(ctx, '/me/wallet/settings').catch(() => null);

		const pointsRes = await apiGet<{ balance?: number; entries?: any[] }>(ctx, '/me/wallet/points?page_size=50').catch(() => null);
		const creditsRes = await apiGet<{ balance?: number; entries?: any[] }>(ctx, '/me/wallet/credits?page_size=50').catch(() => null);

		function describe(raw: any): string | null {
			const ref = raw?.reference?.toString().trim();
			const reason = raw?.reason?.toString().trim();
			return ref || reason || null;
		}

		function mapEntries(entries: any[] | undefined, type: string) {
			return (entries ?? []).map((raw: any) => ({
				id: raw.id,
				type,
				operation: raw.operation,
				amount: raw.amount,
				description: describe(raw),
				created_at: raw.created_at
			}));
		}

		// Each ledger stream is page_size/page_offset-paginated server-side.
		// The merged `ledger` is capped at 50 entries per stream — the counts
		// tell the History tab how many arrived so it can page further
		// client-side ("more" inferred from a full page; no total returned).
		const ledger = [
			...mapEntries(pointsRes?.entries, 'points'),
			...mapEntries(creditsRes?.entries, 'credits')
		].sort((a, b) => new Date(b.created_at ?? 0).getTime() - new Date(a.created_at ?? 0).getTime());

		const flags = await apiGet<Record<string, boolean>>(ctx, '/feature-flags').catch((): Record<string, boolean> => ({}));

		return {
			wallet: await apiGet<unknown>(ctx, '/me/wallet').catch(() => null),
			// Rule 16: currency list, credit packs, and user preference come from the API.
			currencies: (await apiGet<unknown[]>(ctx, '/currencies').catch(() => [] as unknown[])),
			creditPacks: (await apiList<unknown>(ctx, '/wallet/credit-packs').catch(() => ({ items: [] }))).items,
			preferredCurrency: await apiGet<{ preferred_currency: string }>(ctx, '/me/preferences/currency')
				.catch(() => ({ preferred_currency: '' })),
			settings: walletSettings,
			// Rule 3/16: min thresholds are operator-configured values from
			// GET /me/wallet/settings — when the endpoint is unavailable the
			// tabs get null and skip client-side minimum checks (the backend
			// still validates), rather than enforcing invented numbers.
			minPurchaseCredits: walletSettings?.min_purchase_credits ?? null,
			minWithdrawCredits: walletSettings?.min_withdrawal_credits ?? null,
			ledger,
			pointsLoaded: pointsRes?.entries?.length ?? 0,
			creditsLoaded: creditsRes?.entries?.length ?? 0,
			withdrawals: (await apiList<unknown>(ctx, '/me/wallet/withdrawals?page_size=20').catch(() => ({ items: [] }))).items,
			withdrawalMethods: (await apiList<unknown>(ctx, '/me/wallet/withdrawal-methods').catch(() => ({ items: [] }))).items,
			withdrawalProviders: await apiGet<string[]>(ctx, '/wallet/withdraw/providers').catch(() => []),
			withdrawEnabled: flags['feature.withdraw'] === true,
			// feature.wallet gates commerce endpoints (PTS-010); the section
			// itself shows under feature.points, so purchase/withdraw tabs
			// hide when commerce is off rather than rendering 404 states.
			walletEnabled: flags['feature.wallet'] === true
		};
	},
	subscription: async (ctx) => ({
		// GET /me/subscription — current subscription (null when none active).
		subscription: await apiGet<unknown>(ctx, '/me/subscription').catch(() => null),
		// GET /pricing/plans — available plans for the subscribe flow (Rule 16).
		plans: await apiGet<unknown[]>(ctx, '/pricing/plans').catch(() => []),
		// GET /payment-providers — enabled payment providers for the checkout.
		providers: await apiGet<unknown[]>(ctx, '/payment-providers').catch(() => []),
		// GET /currencies — public currency reference for price formatting (Rule 16).
		currencies: await apiGet<unknown[]>(ctx, '/currencies').catch(() => [])
	}),
	payments: async (ctx) => ({
		// GET /me/payments — paginated payment history for the billing Payments
		// section (Section 23). Best-effort: a missing wallet/payment feature
		// degrades to an empty list rather than blanking the whole settings hub.
		// First page of a page_size/page_offset-paginated endpoint; the section
		// loads further pages client-side (the response carries no total, so
		// "more" is inferred from a full page).
		payments: (await apiList<Payment>(ctx, '/me/payments?page_size=20').catch(() => ({ items: [] }))).items,
		pageSize: 20
	}),
	danger: async (ctx) => {
		// Section 50: the danger section needs the caller's username for the
		// account-deletion confirmation flow. It is part of /me, so we read it
		// here instead of relying on the parent layout (which may not expose it).
		const profile = await apiGet<MeProfile>(ctx, '/me').catch(() => null);
		return { username: profile?.username ?? '' };
	},

	// §102 Mentorship Matching (ADR-110): mentor settings — toggle is_mentor,
	// set capacity, areas, bio. GET /me/mentor-profile returns the caller's
	// mentor columns on user_profiles. Best-effort: degrades to null when
	// the user lacks the mentorship.become_mentor permission or the feature
	// is disabled (the section renders a permission/disabled notice).
	mentorship: async (ctx) => ({
		profile: await apiGet<MentorProfile>(ctx, '/me/mentor-profile').catch(() => null),
		// Geo country list for the mentor-card "Countries" autocomplete
		// repeater (same source as the Location section's country picker).
		countries: (await apiList<GeoCountry>(ctx, '/geo/countries').then((r) => r.items).catch(() => []))
	}),
};
