import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { getTool, isToolEnabled } from '$lib/tools/registry';
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
 * Ref: §94 (Credential Checker), ROADMAP-TOOLS 02.06, ADR-088.
 */
export const load: PageServerLoad = async (event) => {
	// Gate: skip all backend calls when credential_checker.enabled is false.
	// Ref: conventions/system-settings-catalog.md, Rule 3.
	const tool = getTool('credential-checker')!;
	const settings = event.locals.siteSettings;
	if (!isToolEnabled(tool, settings)) {
		return {
			enabled: false,
			tool,
			destinations: [],
			professions: [],
			pathways: [],
			user: event.locals.user
		};
	}

	const ctx = apiContext(event);

	// Destination country list — the primary data axis (ADR-088). Rule 16.
	let destinations: GeoSpaceOption[] = [];
	try {
		destinations = (await apiList<GeoSpaceOption>(ctx, '/spaces?level=country')).items;
	} catch {
		destinations = [];
	}

	// Professions list — the second selector axis. Public endpoint.
	let professions: CredentialProfession[] = [];
	try {
		professions = (await apiList<CredentialProfession>(ctx, '/credentials/professions')).items;
	} catch {
		professions = [];
	}

	// Optional SSR pre-fetch of pathways when ?dest= and ?profession= are
	// present in the URL — improves first paint for deep links.
	let pathways: CredentialPathwayWithAi[] = [];
	const destParam = event.url.searchParams.get('dest');
	const professionParam = event.url.searchParams.get('profession');
	if (destParam && professionParam) {
		try {
			pathways = (
				await apiList<CredentialPathwayWithAi>(
					ctx,
					`/credentials?dest=${destParam}&profession=${professionParam}`
				)
			).items;
		} catch {
			pathways = [];
		}
	}

	return {
		enabled: true,
		tool,
		destinations,
		professions,
		pathways,
		user: event.locals.user
	};
};
