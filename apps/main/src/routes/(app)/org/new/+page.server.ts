import { fail, redirect } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiGet, apiPost, ApiError } from '$lib/server/api';
import type { OrgMeta, PublicOrganization } from '$lib/types';

/**
 * Create-organization form at /org/new — auth-gated. POST
 * /organizations creates the organization; on success we redirect to
 * /org/{slug}.
 *
 * Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	// BUG-022: Fetch org type options from the API (Rule 16).
	let orgMeta: OrgMeta | null = null;
	try {
		orgMeta = await apiGet<OrgMeta>(apiContext(event), '/organizations/meta');
	} catch {
		// Non-fatal — the form falls back to hardcoded options.
	}

	return { orgMeta };
};

export const actions: Actions = {
	default: async (event) => {
		if (!event.locals.user) return fail(403, { message: 'Auth required' });

		const ctx = apiContext(event);
		const form = await event.request.formData();

		const name = String(form.get('name') ?? '').trim();
		const slug = String(form.get('slug') ?? '').trim();
		const tagline = String(form.get('tagline') ?? '').trim();
		const description = String(form.get('description') ?? '').trim();
		const websiteUrl = String(form.get('website_url') ?? '').trim();
		const countryCode = String(form.get('country_code') ?? '').trim();
		const orgType = String(form.get('org_type') ?? 'business');

		if (!name) {
			return fail(400, {
				message: 'Name is required',
				values: { name, slug, tagline, description, website_url: websiteUrl, country_code: countryCode, org_type: orgType }
			});
		}
		if (!slug) {
			return fail(400, {
				message: 'Slug is required',
				values: { name, slug, tagline, description, website_url: websiteUrl, country_code: countryCode, org_type: orgType }
			});
		}

		const payload: Record<string, unknown> = { name, slug, org_type: orgType };
		if (tagline) payload.tagline = tagline;
		if (description) payload.description = description;
		if (websiteUrl) payload.website_url = websiteUrl;
		if (countryCode) payload.country_code = countryCode;

		try {
			const created = await apiPost<PublicOrganization>(ctx, '/organizations', payload);
			throw redirect(303, `/org/${created.slug}`);
		} catch (e) {
			if (e instanceof ApiError) {
				return fail(e.status >= 400 && e.status < 500 ? e.status : 500, {
					message: e.message,
					values: { name, slug, tagline, description, website_url: websiteUrl, country_code: countryCode, org_type: orgType }
				});
			}
			throw e;
		}
	}
};
