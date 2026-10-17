import type { PageServerLoad } from './$types';
import { buildMeta } from '$lib/seo/meta';
import { env } from '$env/dynamic/public';
import { tServer, DEFAULT_SITE_NAME } from '$lib/i18n/dict';

/**
 * Access-denied page — the destination for 403s on gated audiences (ADR-060).
 * Public (no session needed); the page offers sign-in and a link to the public
 * API reference.
 */
export const load: PageServerLoad = async (event) => {
	const site = (((await event.parent())?.siteSettings ?? {}) as Record<string, string>)['general.site_name'] ?? DEFAULT_SITE_NAME;
	const locale = event.locals.locale;
	const appName = tServer(locale, 'app.name', '{site} Docs', undefined, site);
	const title = `${tServer(locale, 'audience.no_access_title', 'Access denied', undefined, site)} · ${appName}`;
	const description = tServer(locale, 'audience.no_access_body', 'You do not have access to this documentation section.', undefined, site);
	const meta = buildMeta({
		title,
		description,
		path: '/docs/access-denied'
	});
	// Login lives on the Main app — point the CTA at the main origin so it
	// does not 404 on the docs origin.
	const mainOrigin = (env.PUBLIC_MAIN_ORIGIN ?? 'http://localhost:3000').replace(/\/$/, '');
	return { meta, loginUrl: `${mainOrigin}/login?redirect=${encodeURIComponent(event.url.href)}` };
};
