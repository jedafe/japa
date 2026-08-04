import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import { redirect } from '@sveltejs/kit';
import { apiPatch } from '$lib/server/api';
import type { NewsletterSubscriber } from '$lib/types';

/**
 * Newsletter preferences at /me/notifications (Section 62, ADR-082).
 * Auth-gated page where the user toggles digest and campaign emails.
 *
 * Ref: japaconnect-engineering/sections/62-newsletter.md
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, '/login?redirect=/me/notifications');
	}

	const ctx = apiContext(event);

	let subscriber: NewsletterSubscriber | null = null;
	try {
		subscriber = await apiGet<NewsletterSubscriber>(ctx, '/newsletter/preferences');
	} catch (e) {
		if (e instanceof ApiError && e.status === 404) {
			// Not subscribed — page shows a "subscribe" prompt.
		}
	}

	return { subscriber };
};

export const actions: Actions = {
	update: async (event) => {
		if (!event.locals.user) {
			return { error: 'auth_required' };
		}

		const ctx = apiContext(event);
		const form = await event.request.formData();
		const digestEnabled = form.get('digest_enabled') === 'on';
		const campaignEnabled = form.get('campaign_enabled') === 'on';

		try {
			const result = await apiPatch<NewsletterSubscriber>(
				ctx,
				'/newsletter/preferences',
				{ digest_enabled: digestEnabled, campaign_enabled: campaignEnabled }
			);
			return { ok: true, subscriber: result };
		} catch (err) {
			if (err instanceof ApiError) {
				return { error: err.code, message: err.message };
			}
			return { error: 'update_failed' };
		}
	}
};
