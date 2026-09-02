import { redirect } from '@sveltejs/kit';
import type { LayoutServerLoad } from './$types';

/**
 * JCE redirect layout (Section 30 / JCE Sub-spec 1).
 *
 * When the `feature.jce` flag is enabled, all legacy `/compare/*` routes in
 * the Main app are redirected to the standalone `apps/jce` surface so the
 * reverse-proxy fallback also points users to the right app.
 *
 * Mapping:
 * - /compare                 -> {jceOrigin}/compare
 * - /compare/new             -> {jceOrigin}/compare
 * - /compare/[slug]          -> {jceOrigin}/compare/[slug]
 * - /compare/mentors?ids=... -> {jceOrigin}/compare?type=mentors&preselect=<ids>
 * - /compare/universities?item1_slug=... -> {jceOrigin}/compare?type=universities&preselect=<slug>
 *
 * The JCE origin is read from `siteSettings['jce.origin']` and falls back to
 * `http://localhost:3006` for local development.
 */
export const load: LayoutServerLoad = async (event) => {
	const parent = await event.parent();
	const features = (parent?.features as Record<string, boolean> | undefined) ?? {};
	const siteSettings = (parent?.siteSettings as Record<string, unknown> | undefined) ?? {};

	if (!features?.['feature.comparison'] || !features?.['feature.jce']) {
		// Keep legacy compare routes when either comparison feature flag is off.
		// The catch-all +page.svelte will render a "not available" fallback.
		return {};
	}

	const jceOrigin = String(siteSettings?.['jce.origin'] ?? 'http://localhost:3006').replace(/\/$/, '');
	if (!jceOrigin) {
		return {};
	}

	const path = event.url.pathname;
	const params = new URLSearchParams(event.url.search);

	let targetPath = '/compare';

	if (path === '/compare' || path === '/compare/') {
		targetPath = '/compare';
	} else if (path === '/compare/new') {
		targetPath = '/compare';
	} else if (path.startsWith('/compare/')) {
		const rest = path.slice('/compare/'.length);
		if (rest === 'mentors') {
			// Mentor compare type; the JCE picker is keyed by `type=mentors`.
			const ids = params.get('ids');
			params.delete('ids');
			if (ids) {
				params.set('preselect', ids);
			}
			params.set('type', 'mentors');
			targetPath = '/compare';
		} else if (rest === 'universities') {
			// Pre-select the first university by slug if provided.
			const item1 = params.get('item1_slug');
			params.delete('item1_slug');
			if (item1) {
				params.set('preselect', item1);
			}
			params.set('type', 'universities');
			targetPath = '/compare';
		} else {
			// Any other /compare/<slug> is treated as a quick comparison view.
			targetPath = `/compare/${rest}`;
		}
	}

	const search = params.toString();
	const redirectUrl = `${jceOrigin}${targetPath}${search ? '?' + search : ''}`;
	throw redirect(307, redirectUrl);
};
