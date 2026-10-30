// Ref: japaconnect-engineering/sections/06-custom-field-builder-form-builder.md
// Generic public form renderer — resolves the {SITE_URL}/forms/{slug} URL the
// admin panel advertises as the share/embed target. Loads the published form
// schema from GET /forms/{slug}/embed (public, no auth).
import type { PageServerLoad } from './$types';
import { redirect } from '@sveltejs/kit';
import { apiContext, apiGet } from '$lib/server/api';
import type { FormEmbed } from '$lib/types';

// System forms consumed by bespoke pages (same special-casing the backend
// applies for link intents): the generic renderer would be a degraded clone
// — no assessment scoring, no link intent, no upload UX — so the share URL
// redirects to the canonical page instead. Forms without a bespoke page
// (admin-created or contextual like job_application_form) render here.
const BESPOKE_FORM_ROUTES: Record<string, string> = {
	'verification-request': '/verification-request',
	'claim-ownership': '/claim-ownership',
	feedback: '/feedback',
	'japa-readiness-assessment': '/tools/readiness'
};

export const load: PageServerLoad = async (event) => {
	const bespoke = BESPOKE_FORM_ROUTES[event.params.slug];
	if (bespoke) redirect(307, bespoke);

	const ctx = apiContext(event);
	let form: FormEmbed | null = null;
	try {
		form = await apiGet<FormEmbed>(ctx, `/forms/${event.params.slug}/embed`);
	} catch {
		form = null;
	}
	return { form };
};
