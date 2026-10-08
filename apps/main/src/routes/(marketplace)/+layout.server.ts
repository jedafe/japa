import type { LayoutServerLoad } from './$types';
import { redirect } from '@sveltejs/kit';
import { academyUrl } from '$lib/appOrigins';

// Pass-through: the root layout already loads everything the marketplace
// chrome needs (user, siteSettings, menu, footerMenu, features,
// unreadNotifications, currencies). This file documents the ADR-058
// exception and reserves the layout for marketplace-scoped data later;
// returning {} keeps child pages receiving parent() data.
//
// Host-surface guards replicate the (app) group exactly — moving /services
// out of (app) must not drop them. Deliberately scoped to this group
// rather than hoisted to the root layout so (auth) behavior is unchanged.
export const load: LayoutServerLoad = async (event) => {
	if (event.locals.isAcademyHost) {
		throw redirect(302, academyUrl(`${event.url.pathname}${event.url.search}`));
	}

	// ADR-127: the roadmap host serves only the public roadmap surface.
	// Allowlist /roadmap/* plus /login (vote/submit auth flow) and /api/* —
	// every other path redirects to the board. Locale-prefixed paths
	// (/fr/roadmap) are checked against their unprefixed form; the locale
	// cookie keeps the session language across the redirect.
	if (event.locals.isRoadmapHost) {
		const pathname = event.url.pathname;
		const unprefixed =
			event.locals.locale && pathname.startsWith(`/${event.locals.locale}/`)
				? pathname.slice(event.locals.locale.length + 1)
				: pathname;
		if (
			!unprefixed.startsWith('/roadmap') &&
			!unprefixed.startsWith('/login') &&
			!unprefixed.startsWith('/api')
		) {
			throw redirect(302, '/roadmap');
		}
	}

	return {};
};
