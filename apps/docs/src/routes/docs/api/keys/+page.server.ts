import { redirect } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiGet, apiPost, apiPatch, apiDelete, ApiError } from '$lib/server/api';
import { buildMeta } from '$lib/seo/meta';
import type { ApiKeySummary, ApiKeyCreated, ApiScopeOption, ApiKeyIpEntry } from '$lib/types';
import { env } from '$env/dynamic/public';
import { tServer, DEFAULT_SITE_NAME } from '$lib/i18n/dict';

// Login lives on the Main app, not the Docs app. Any /login redirect relative
// to the docs origin 404s, so always build the full URL against the main origin.
const mainOrigin = (env.PUBLIC_MAIN_ORIGIN ?? 'http://localhost:3000').replace(/\/$/, '');

/**
 * Developer Self-Service API Keys page (Ref: §47 ROADMAP Phase 06).
 * Auth required: guests are redirected to login. Lists, creates, renames,
 * rotates, revokes, and manages IP allowlists for the user's own API keys
 * via /me/api-keys. The full key is shown once on creation/rotation, then
 * only the prefix is visible.
 */
export const load: PageServerLoad = async (event) => {
	const site = (((await event.parent())?.siteSettings ?? {}) as Record<string, string>)['general.site_name'] ?? DEFAULT_SITE_NAME;
	if (!event.locals.user) {
		throw redirect(307, `${mainOrigin}/login?redirect=${encodeURIComponent(event.url.href)}`);
	}

	const ctx = apiContext(event);
	let keys: ApiKeySummary[] = [];
	try {
		keys = await apiGet<ApiKeySummary[]>(ctx, '/me/api-keys');
	} catch {
		keys = [];
	}

	// Canonical scope catalog from GET /developer/scopes (Rule 16 — the
	// backend catalog is the source of truth, not a hardcoded list). The
	// endpoint already withholds admin-only scopes (api.write.*, *).
	let scopes: ApiScopeOption[] = [];
	try {
		scopes = await apiGet<ApiScopeOption[]>(ctx, '/developer/scopes');
	} catch {
		scopes = [];
	}

	// IP allowlist entries per key (GET /me/api-keys/{id}/ip-allowlist).
	// Key count is bounded by api.keys.max_per_user, so a small parallel
	// fan-out is safe; failures degrade to an empty list per key.
	const allowlists: Record<string, ApiKeyIpEntry[]> = {};
	await Promise.all(
		keys.map(async (k) => {
			try {
				allowlists[k.id] = await apiGet<ApiKeyIpEntry[]>(
					ctx,
					`/me/api-keys/${k.id}/ip-allowlist`
				);
			} catch {
				allowlists[k.id] = [];
			}
		})
	);

	// Key limit from system_settings (Ref: §47 ROADMAP 06.04). The backend
	// exposes this via the public settings endpoint; degrade to a high
	// default if unavailable.
	let maxKeys = 5;
	try {
		const settings = await apiGet<Record<string, unknown>>(ctx, '/site/settings/public');
		// system_settings stores values as JSON — int settings can arrive as a
		// quoted string ("5") depending on how they were written, so accept both.
		const v = Number(settings['api.keys.max_per_user']);
		if (Number.isInteger(v) && v > 0) maxKeys = v;
	} catch {
		// keep default
	}

	const locale = event.locals.locale;
	const appName = tServer(locale, 'app.name', '{site} Docs', undefined, site);
	const title = `${tServer(locale, 'api.keys_title', 'API Keys', undefined, site)} · ${appName}`;
	const description = tServer(
		locale,
		'api.keys_desc',
		'Manage your developer API keys for the {site} platform.',
		undefined,
		site
	);

	const meta = buildMeta({
		title,
		description,
		path: '/docs/api/keys'
	});

	return { keys, scopes, allowlists, maxKeys, meta };
};

function failFrom(err: unknown, fallback: string): { error: string } {
	if (err instanceof ApiError) return { error: err.message };
	return { error: fallback };
}

export const actions: Actions = {
	/** Create a new API key (Ref: §47 ROADMAP 06.02). */
	create: async (event) => {
		if (!event.locals.user) {
			throw redirect(307, `${mainOrigin}/login?redirect=${encodeURIComponent(event.url.href)}`);
		}
		const ctx = apiContext(event);
		const form = await event.request.formData();
		const name = (form.get('name') as string | null)?.trim();
		const rate_limit_tier = (form.get('rate_limit_tier') as string | null) ?? 'basic';
		const scopesRaw = form.get('scopes') as string | null;
		const scopes = scopesRaw
			? scopesRaw.split(',').map((s) => s.trim()).filter(Boolean)
			: [];

		if (!name) {
			return { error: 'name is required' };
		}

		// Public /me/api-keys expects `name` and `rate_limit_tier` (Section 47).
		// The backend caps the requested tier to the user's allowed tier.
		try {
			const created = await apiPost<ApiKeyCreated>(ctx, '/me/api-keys', {
				name,
				rate_limit_tier,
				scopes
			});
			return { created };
		} catch (err) {
			return failFrom(err, 'Failed to create API key');
		}
	},

	/** Rename an API key — PATCH /me/api-keys/{id} (P5.6, name only). */
	rename: async (event) => {
		if (!event.locals.user) {
			throw redirect(307, `${mainOrigin}/login?redirect=${encodeURIComponent(event.url.href)}`);
		}
		const ctx = apiContext(event);
		const form = await event.request.formData();
		const id = form.get('id') as string | null;
		const name = (form.get('name') as string | null)?.trim();
		if (!id || !name) return { error: 'Missing key id or name' };

		try {
			await apiPatch(ctx, `/me/api-keys/${id}`, { name });
			return { renamed: id };
		} catch (err) {
			return failFrom(err, 'Failed to rename API key');
		}
	},

	/** Rotate an API key — POST /me/api-keys/{id}/rotate (P5.6). The new
	 *  secret is returned once; the old key expires after a 24h grace. */
	rotate: async (event) => {
		if (!event.locals.user) {
			throw redirect(307, `${mainOrigin}/login?redirect=${encodeURIComponent(event.url.href)}`);
		}
		const ctx = apiContext(event);
		const form = await event.request.formData();
		const id = form.get('id') as string | null;
		if (!id) return { error: 'Missing key id' };

		try {
			const rotated = await apiPost<ApiKeyCreated>(ctx, `/me/api-keys/${id}/rotate`);
			return { rotated };
		} catch (err) {
			return failFrom(err, 'Failed to rotate API key');
		}
	},

	/** Add a CIDR entry to a key's IP allowlist (§47). */
	addIp: async (event) => {
		if (!event.locals.user) {
			throw redirect(307, `${mainOrigin}/login?redirect=${encodeURIComponent(event.url.href)}`);
		}
		const ctx = apiContext(event);
		const form = await event.request.formData();
		const id = form.get('id') as string | null;
		const cidr = (form.get('cidr') as string | null)?.trim();
		const description = (form.get('description') as string | null)?.trim() ?? '';
		if (!id || !cidr) return { error: 'Missing key id or CIDR' };

		try {
			await apiPost(ctx, `/me/api-keys/${id}/ip-allowlist`, { cidr, description });
			return { ipAdded: id };
		} catch (err) {
			return failFrom(err, 'Failed to add allowlist entry');
		}
	},

	/** Remove an IP allowlist entry (§47). */
	removeIp: async (event) => {
		if (!event.locals.user) {
			throw redirect(307, `${mainOrigin}/login?redirect=${encodeURIComponent(event.url.href)}`);
		}
		const ctx = apiContext(event);
		const form = await event.request.formData();
		const id = form.get('id') as string | null;
		const ipId = form.get('ip_id') as string | null;
		if (!id || !ipId) return { error: 'Missing key id or entry id' };

		try {
			await apiDelete(ctx, `/me/api-keys/${id}/ip-allowlist/${ipId}`);
			return { ipRemoved: id };
		} catch (err) {
			return failFrom(err, 'Failed to remove allowlist entry');
		}
	},

	/** Revoke an API key (Ref: §47 ROADMAP 06.03). */
	revoke: async (event) => {
		if (!event.locals.user) {
			throw redirect(307, `${mainOrigin}/login?redirect=${encodeURIComponent(event.url.href)}`);
		}
		const ctx = apiContext(event);
		const form = await event.request.formData();
		const id = form.get('id') as string | null;
		if (!id) return { error: 'Missing key id' };

		try {
			await apiDelete(ctx, `/me/api-keys/${id}`);
			return { revoked: id };
		} catch (err) {
			return failFrom(err, 'Failed to revoke API key');
		}
	}
};
