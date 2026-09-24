// Ref: japaconnect-engineering/sections/87-japa-stage-profiles.md
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import { redirect } from '@sveltejs/kit';
import type { MyJapaStage, JapaStage, JapaIntent, GeoCountry } from '$lib/types';

/**
 * My Japa Stage at /japa-stage/me. Auth-gated: redirects to login when not
 * authed. Preloads the caller's own stage from GET /me/japa-stage so the form
 * can render server-side; degrades to null when the backend is unavailable.
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, '/login?redirect=/japa-stage/me');
	}

	const ctx = apiContext(event);
	let myStage: MyJapaStage | null = null;
	try {
		myStage = await apiGet<MyJapaStage>(ctx, '/me/japa-stage');
	} catch {
		myStage = null;
	}

	let countries: GeoCountry[] = [];
	try {
		const { items } = await apiList<GeoCountry>(ctx, '/geo/countries');
		countries = items;
	} catch {
		countries = [];
	}

	let stages: JapaStage[] = [];
	try {
		const list = await apiGet<{ value: string; label: string }[]>(ctx, '/japa-stage/stages');
		stages = (list ?? []).map((o) => o.value as JapaStage);
	} catch {
		stages = [];
	}

	let intents: JapaIntent[] = [];
	try {
		const list = await apiGet<{ value: string; label: string }[]>(ctx, '/japa-stage/intents');
		intents = (list ?? []).map((o) => o.value as JapaIntent);
	} catch {
		intents = [];
	}

	return { myStage, countries, stages, intents };
};
