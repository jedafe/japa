// Ref: japaconnect-engineering/sections/33-professional-profiles.md (invitation acceptance)
// Accept-invitation page at /profiles/invitations/{token} — auth-gated.
// POST /profiles/invitations/{token}/accept accepts the invitation.
import { fail, redirect } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiPost, ApiError } from '$lib/server/api';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}
	return { token: event.params.token };
};

export const actions: Actions = {
	default: async (event) => {
		if (!event.locals.user) return fail(403, { message: 'login_required' });

		const ctx = apiContext(event);
		const token = event.params.token;

		try {
			await apiPost(ctx, `/profiles/invitations/${token}/accept`, {});
			return { success: true };
		} catch (e) {
			if (e instanceof ApiError) {
				const code = e.code ?? 'failed';
				const messageMap: Record<string, string> = {
					'invitation_not_found': 'invalid',
					'invitation_expired': 'invalid',
					'email_mismatch': 'email_mismatch',
					'already_member': 'failed'
				};
				return fail(e.status >= 400 && e.status < 500 ? e.status : 500, {
					message: messageMap[code] ?? 'failed'
				});
			}
			return fail(500, { message: 'failed' });
		}
	}
};
