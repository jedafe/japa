import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList, ApiError } from '$lib/server/api';
import type {
	GeoSpaceOption,
	CredentialProfession,
	CredentialPathwayWithAi
} from '$lib/types';

/**
 * Professional Credential Checker — §94.
 * SSR loads the destination country list and the professions list. Pathways
 * require a destination + profession selection, so they are fetched
 * client-side (with optional SSR pre-fetch when ?dest= is present).
 *
 * API: GET /spaces?level=country — destination selector options.
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
	// A 404 or credential_checker_disabled code means the tool is turned off.
	let destinations: GeoSpaceOption[] = [];
	let disabled = false;
	try {
		destinations = (await apiList<GeoSpaceOption>(ctx, '/spaces?level=country')).items;
	} catch (e) {
		if (e instanceof ApiError && (e.status === 404 || e.code === 'credential_checker_disabled')) {
			disabled = true;
		}
		destinations = [];
	}

	// Professions list — the second selector axis. Public endpoint.
	let professions: CredentialProfession[] = [];
	if (!disabled) {
		try {
			professions = (await apiList<CredentialProfession>(ctx, '/credentials/professions')).items;
		} catch {
			professions = [];
		}
	}

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
		disabled,
		authed
	};
};
