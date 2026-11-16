import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiGet, ApiError } from '#lib/server/api.js';

/**
 * Newsletter double opt-in confirmation page at /newsletter/confirm
 * (Section 62, migration 001170). Public page reached via the confirm link
 * in the double opt-in email; submits the single-use purpose='confirm'
 * token to GET /newsletter/confirm.
 *
 * Ref: japaconnect-engineering/sections/62-newsletter-digest.md
 */
export const load: PageServerLoad = async (event) => {
	const token = event.url.searchParams.get('token') ?? '';
	return { token };
};

export const actions: Actions = {
	confirm: async (event) => {
		const ctx = apiContext(event);
		const form = await event.request.formData();
		const token = (form.get('token') as string | null)?.trim() ?? '';
		if (!token) return { error: 'missing_param' };

		try {
			await apiGet(ctx, `/newsletter/confirm?token=${encodeURIComponent(token)}`);
			return { ok: true };
		} catch (err) {
			if (err instanceof ApiError) {
				return { error: err.code };
			}
			return { error: 'network_error' };
		}
	}
};
