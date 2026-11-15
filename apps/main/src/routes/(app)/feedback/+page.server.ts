// Ref: japaconnect-engineering/sections/06-custom-fields-forms.md (public form embed)
// Public feedback page. Loads the form definition from GET /forms/feedback/embed
// (public, no auth) so the page can render the fields without hardcoding them.
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '#lib/server/api.js';
import type { FormEmbed, GeoCountry } from '#lib/types/index.js';

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	let form: FormEmbed | null = null;
	try {
		form = await apiGet<FormEmbed>(ctx, '/forms/feedback/embed');
	} catch {
		form = null;
	}

	// Same geo_countries resolution as the generic renderer — suggest fields
	// with {"source":"geo_countries"} need the catalog for their options.
	let countries: GeoCountry[] = [];
	if (form?.fields.some((f) => f.options_source === 'geo_countries')) {
		try {
			countries = (await apiList<GeoCountry>(ctx, '/geo/countries')).items;
		} catch {
			countries = [];
		}
	}

	return { form, countries };
};
