// Ref: japaconnect-engineering/sections/33-professional-profiles.md (profile creation)
// Create-professional-profile form at /dashboard/profiles/new — auth-gated.
// POST /profiles creates the profile; on success we redirect to the dashboard.
import { fail, redirect } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiGet, apiPost, ApiError } from '$lib/server/api';
import type { ProfessionalProfile, ProfessionalProfileType } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	// Fetch profile type options from the API (Rule 16: no hardcoded option lists).
	const ctx = apiContext(event);
	let profileTypes: ProfessionalProfileType[] = [];
	try {
		profileTypes = await apiGet<ProfessionalProfileType[]>(ctx, '/profiles/types?page_size=100&page_offset=0');
	} catch {
		// Non-fatal — the form will show an empty select.
	}

	return { profileTypes };
};

export const actions: Actions = {
	default: async (event) => {
		if (!event.locals.user) return fail(403, { message: 'Auth required' });

		const ctx = apiContext(event);
		const form = await event.request.formData();

		const name = String(form.get('name') ?? '').trim();
		const slug = String(form.get('slug') ?? '').trim();
		const typeId = String(form.get('type_id') ?? '').trim();
		const tagline = String(form.get('tagline') ?? '').trim();
		const description = String(form.get('description') ?? '').trim();
		const websiteUrl = String(form.get('website_url') ?? '').trim();
		const verificationType = String(form.get('verification_type') ?? '').trim();

		if (!name) return fail(400, { message: 'Name is required', values: { name, slug, typeId, tagline, description, website_url: websiteUrl } });
		if (!slug) return fail(400, { message: 'Slug is required', values: { name, slug, typeId, tagline, description, website_url: websiteUrl } });

		const payload: Record<string, unknown> = { name, slug };
		if (typeId) payload.type_id = typeId;
		if (tagline) payload.tagline = tagline;
		if (description) payload.description = description;
		if (websiteUrl) payload.website_url = websiteUrl;
		if (verificationType) payload.verification_type = verificationType;

		try {
			const created = await apiPost<ProfessionalProfile>(ctx, '/profiles', payload);
			throw redirect(303, `/dashboard/profiles/${created.id}`);
		} catch (e) {
			if (e instanceof ApiError) {
				const code = e.code ?? 'create_failed';
				return fail(e.status >= 400 && e.status < 500 ? e.status : 500, {
					message: code === 'upgrade_to_business_required' ? 'upgrade_required' : e.message,
					values: { name, slug, typeId, tagline, description, website_url: websiteUrl }
				});
			}
			throw e;
		}
	}
};
