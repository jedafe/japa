import type { PageServerLoad } from './$types';
import { audienceStats } from '$lib/docs/catalog';
import { buildMeta } from '$lib/seo/meta';
import { tServer, DEFAULT_SITE_NAME } from '$lib/i18n/dict';

/**
 * User-groups index: every audience the visitor can read, derived from the
 * layout's gated nav tree (no extra API calls).
 */
export const load: PageServerLoad = async (event) => {
	const data = await event.parent();
	const site = (data.siteSettings?.['general.site_name'] as string | undefined) ?? DEFAULT_SITE_NAME;

	const groups = data.audienceOrder
		.filter((a) => data.accessibleAudiences.includes(a))
		.map((audience) => ({
			audience,
			stats: audienceStats(data.nav.find((n) => n.audience === audience))
		}));

	const locale = data.locale ?? event.locals.locale;
	const appName = tServer(locale, 'app.name', '{site} Docs', undefined, site);
	const title = `${tServer(locale, 'groups.title', 'Documentation by user group', undefined, site)} · ${appName}`;
	const description = tServer(
		locale,
		'groups.sub',
		'Choose your role to explore relevant guides and resources.',
		undefined,
		site
	);
	const meta = buildMeta({ title, description, path: '/docs/groups', ogType: 'website' });

	return { groups, meta };
};
