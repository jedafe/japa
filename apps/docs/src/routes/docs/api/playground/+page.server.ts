import { redirect } from '@sveltejs/kit';
import { env } from '$env/dynamic/public';
import type { PageServerLoad } from './$types';
import { buildMeta } from '$lib/seo/meta';
import { API_BASE, JC_SURFACE, apiContext, apiGet } from '$lib/server/api';
import type { OpenAPISpec, ApiKeySummary } from '$lib/types';
import { tServer } from '$lib/i18n/dict';

/**
 * API Playground — the full-width tool exception (ADR-060). Breaks the shell
 * for room. The OpenAPI spec is fetched server-side and passed to the client;
 * the client makes real API calls with the user's own key (Section 47).
 *
 * Auth gate (Ref: §47 ROADMAP 04.01): the playground requires an authenticated
 * session. Guests are redirected to the main site login.
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		const mainOrigin = (env.PUBLIC_MAIN_ORIGIN ?? 'http://localhost:3000').replace(/\/$/, '');
		throw redirect(307, `${mainOrigin}/login?redirect=${encodeURIComponent(event.url.href)}`);
	}

	let spec: OpenAPISpec | null = null;
	try {
		const res = await event.fetch(`${API_BASE}/docs/openapi.json`, {
			headers: { 'X-JC-Surface': JC_SURFACE }
		});
		if (res.ok) spec = (await res.json()) as OpenAPISpec;
	} catch {
		spec = null;
	}

	// Fetch the user's API keys so the "Use my API Key" selector can populate
	// (Ref: §47 ROADMAP 04.05). Degrade to empty list on failure.
	let apiKeys: ApiKeySummary[] = [];
	try {
		const ctx = apiContext(event);
		apiKeys = await apiGet<ApiKeySummary[]>(ctx, '/me/api-keys');
	} catch {
		apiKeys = [];
	}

	const locale = event.locals.locale;
	const appName = tServer(locale, 'app.name', 'JapaConnect Docs');
	const title = `${tServer(locale, 'api.playground_title', 'API Playground')} · ${appName}`;
	const description = tServer(
		locale,
		'api.playground_subtitle',
		'Make real requests against the JapaConnect API with your own key.'
	);

	const meta = buildMeta({
		title,
		description,
		path: '/docs/api/playground'
	});

	// Prefill from ?method=&path= — used by the "Try it" links on the API
	// landing endpoint browser.
	const VALID_METHODS = new Set(['GET', 'POST', 'PATCH', 'PUT', 'DELETE']);
	const rawMethod = (event.url.searchParams.get('method') ?? '').toUpperCase();
	const rawPath = (event.url.searchParams.get('path') ?? '').trim();
	const prefill = {
		method: VALID_METHODS.has(rawMethod) ? rawMethod : '',
		path: rawPath.startsWith('/') ? rawPath : ''
	};

	return { spec, apiKeys, prefill, meta };
};
