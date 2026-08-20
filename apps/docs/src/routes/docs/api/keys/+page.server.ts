import { redirect, error } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiGet, apiPost, apiDelete, ApiError } from '$lib/server/api';
import { buildMeta } from '$lib/seo/meta';
import type { ApiKeySummary, ApiKeyCreated } from '$lib/types';
import { env } from '$env/dynamic/public';
import { tServer } from '$lib/i18n/dict';

// Login lives on the Main app, not the Docs app. Any /login redirect relative
// to the docs origin 404s, so always build the full URL against the main origin.
const mainOrigin = (env.PUBLIC_MAIN_ORIGIN ?? 'http://localhost:3000').replace(/\/$/, '');

/**
 * Developer Self-Service API Keys page (Ref: §47 ROADMAP Phase 06).
 * Auth required: guests are redirected to login. Lists, creates, and revokes
 * the user's own API keys via /me/api-keys. The full key is shown once on
 * creation, then only the prefix is visible.
 */
export const load: PageServerLoad = async (event) => {
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

	// Key limit from system_settings (Ref: §47 ROADMAP 06.04). The backend
	// exposes this via the public settings endpoint; degrade to a high
	// default if unavailable.
	let maxKeys = 5;
	try {
		const settings = await apiGet<Record<string, unknown>>(ctx, '/site/settings/public');
		const v = settings['api.keys.max_per_user'];
		if (typeof v === 'number') maxKeys = v;
	} catch {
		// keep default
	}

	const locale = event.locals.locale;
	const appName = tServer(locale, 'app.name', 'JapaConnect Docs');
	const title = `${tServer(locale, 'api.keys_title', 'API Keys')} · ${appName}`;
	const description = tServer(
		locale,
		'api.keys_desc',
		'Manage your developer API keys for the JapaConnect platform.'
	);

	const meta = buildMeta({
		title,
		description,
		path: '/docs/api/keys'
	});

	return { keys, maxKeys, meta };
};

export const actions: Actions = {
	/** Create a new API key (Ref: §47 ROADMAP 06.02). */
	create: async (event) => {
		if (!event.locals.user) {
			throw redirect(307, `${mainOrigin}/login?redirect=${encodeURIComponent(event.url.href)}`);
		}
		const ctx = apiContext(event);
		const form = await event.request.formData();
		const app_name = (form.get('app_name') as string | null)?.trim();
		const environment = (form.get('environment') as string | null) ?? 'live';
		const scopesRaw = form.get('scopes') as string | null;
		const scopes = scopesRaw
			? scopesRaw.split(',').map((s) => s.trim()).filter(Boolean)
			: [];

		if (!app_name) {
			return { error: 'app_name is required' };
		}

		try {
			const created = await apiPost<ApiKeyCreated>(ctx, '/me/api-keys', {
				app_name,
				environment,
				scopes
			});
			return { created };
		} catch (err) {
			if (err instanceof ApiError) {
				return { error: err.message };
			}
			return { error: 'Failed to create API key' };
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
			if (err instanceof ApiError) {
				return { error: err.message };
			}
			return { error: 'Failed to revoke API key' };
		}
	}
};
