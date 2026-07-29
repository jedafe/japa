// Ref: japaconnect-engineering/sections/06-custom-fields-forms.md (public form embed)
// Public feedback page. Loads the form definition from GET /forms/feedback/embed
// (public, no auth) so the page can render the fields without hardcoding them.
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { FormEmbed } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	let form: FormEmbed | null = null;
	try {
		form = await apiGet<FormEmbed>(ctx, '/forms/feedback/embed');
	} catch {
		form = null;
	}
	return { form };
};
