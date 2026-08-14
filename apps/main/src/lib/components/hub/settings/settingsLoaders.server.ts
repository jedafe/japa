import { apiGet, apiList, ApiError, type ApiContext } from '$lib/server/api';
import type { MeProfile, PrivacySettings, AffiliateEarnings, GeoCountry, NotificationPreference, MentorProfile } from '$lib/types';

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
	// Section 62 / ADR-080: newsletter subscriber preferences (digest +
	// campaign toggles). Best-effort: degrades to null when the user is not
	// subscribed or the feature is disabled, so the section can render a
	// subscribe CTA instead of the toggles.
	newsletter: async (ctx) => ({
		preferences: await apiGet<unknown>(ctx, '/newsletter/preferences').catch(() => null)
	}),
	'api-keys': async (ctx) => (await apiList<unknown>(ctx, '/me/api-keys')).items,
	affiliate: async (ctx) => {
		// GET /affiliate-program — public program info (commission rates, T&Cs,
		// eligibility). Always available so the enrollment CTA can render even
		// before the user enrolls (Section 56).
		const program = await apiGet<unknown>(ctx, '/affiliate-program').catch(() => null);

		// GET /me/affiliate-links — the caller's affiliate links (CRUD list).
		// A 403 means the user has not enrolled yet (affiliate.links.manage is
		// granted on enrollment) — we flag `enrolled: false` so the section can
		// show an enrollment CTA instead of the create form.
		let enrolled = true;
		let links: unknown[] = [];
		try {
			links = (await apiList<unknown>(ctx, '/me/affiliate-links')).items;
		} catch (e) {
			if (e instanceof ApiError && e.status === 403) enrolled = false;
		}

		// GET /me/affiliate-earnings — aggregate earnings summary. Best-effort:
		// degrades to null when unavailable or the user is not enrolled.
		const earnings = enrolled
			? await apiGet<AffiliateEarnings>(ctx, '/me/affiliate-earnings').catch(() => null)
			: null;

		return { program, links, earnings, enrolled };
	},
	wallet: async (ctx) => ({
		wallet: await apiGet<unknown>(ctx, '/me/wallet'),
		ledger: (await apiList<unknown>(ctx, '/me/wallet/ledger')).items,
		settings: await apiGet<unknown>(ctx, '/me/wallet/settings').catch(() => null)
	}),
	subscription: async (ctx) => ({
		// GET /me/subscription — current subscription (null when none active).
		subscription: await apiGet<unknown>(ctx, '/me/subscription').catch(() => null),
		// GET /pricing/plans — available plans for the subscribe flow (Rule 16).
		plans: await apiGet<unknown[]>(ctx, '/pricing/plans').catch(() => []),
		// GET /payment-providers — enabled payment providers for the checkout.
		providers: await apiGet<unknown[]>(ctx, '/payment-providers').catch(() => []),
		// GET /wallet/currencies — currency symbols for price formatting (Rule 16).
		currencies: await apiGet<unknown[]>(ctx, '/wallet/currencies').catch(() => [])
	}),
	danger: async () => null,

	// §102 Mentorship Matching (ADR-110): mentor settings — toggle is_mentor,
	// set capacity, areas, bio. GET /me/mentor-profile returns the caller's
	// mentor columns on user_profiles. Best-effort: degrades to null when
	// the user lacks the mentorship.become_mentor permission or the feature
	// is disabled (the section renders a permission/disabled notice).
	mentorship: async (ctx) => ({
		profile: await apiGet<MentorProfile>(ctx, '/me/mentor-profile').catch(() => null)
	})
};
