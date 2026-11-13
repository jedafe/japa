import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '#lib/server/api.js';
import type {
	GeoCountry,
	CredentialProfession,
	CredentialPathwayWithAi
} from '#lib/types/index.js';

/**
 * Professional Credential Checker — §94.
 * SSR loads the full 250 primary geo space countries and the professions list. Pathways
 * require a destination + profession selection, so they are fetched
 * client-side (with optional SSR pre-fetch when ?dest= is present).
 *
 * API: GET /geo/countries — destination selector options (the full 250 primary geo space countries).
 *      GET /credentials/professions — list of professions.
 *      GET /credentials?dest=&profession=&issuing= — pathway list.
 *      GET /credentials/{id} — { pathway, notes } (non-standard envelope).
 *      POST /credentials/{id}/notes — community note (auth).
 *
 * Route: /tools/credential-checker
 * Ref: §94 (Credential Checker), ROADMAP-TOOLS 02.06, ADR-088.
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['credential_checker.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Credential Checker is not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	// Destination country list — the primary data axis (ADR-088). Rule 16.
	// The feature flag gate above throws 404 before this point when the tool
	// is off, so fetches only have to degrade to empty lists on API errors.
	const [destinations, professionsRes] = await Promise.all([
		apiList<GeoCountry>(ctx, '/geo/countries')
			.then((r) => r.items)
			.catch(() => [] as GeoCountry[]),
		apiList<CredentialProfession>(ctx, '/credentials/professions')
			.then((r) => r.items)
			.catch(() => [] as CredentialProfession[])
	]);
	const professions: CredentialProfession[] = professionsRes;

	// Optional SSR pre-fetch of pathways when ?dest= and ?profession= are
	// present in the URL — improves first paint for deep links.
	let pathways: CredentialPathwayWithAi[] = [];
	const destParam = event.url.searchParams.get('dest');
	const professionParam = event.url.searchParams.get('profession');
	const initialDestId =
		destParam && destinations.some((d) => d.id === destParam) ? destParam : null;
	const initialProfessionId =
		professionParam && professions.some((p) => p.id === professionParam)
			? professionParam
			: null;
	if (initialDestId && initialProfessionId) {
		try {
			pathways = (
				await apiList<CredentialPathwayWithAi>(
					ctx,
					`/credentials?dest=${initialDestId}&profession=${initialProfessionId}`
				)
			).items;
		} catch {
			pathways = [];
		}
	}

	return {
		destinations,
		professions,
		pathways,
		initialDestId,
		initialProfessionId,
		authed
	};
};
