import type { PageServerLoad, Actions } from './$types';
import { apiContext, API_V1 } from '$lib/server/api';

/**
 * Newsletter unsubscribe page at /unsubscribe (Section 62, ADR-082).
 * Public page accessed via the unsubscribe link in email headers (List-Unsubscribe).
 * Accepts ?token= or ?email= query params and performs a one-click unsubscribe.
 *
 * Ref: japaconnect-engineering/sections/62-newsletter.md
 */
export const load: PageServerLoad = async (event) => {
	const token = event.url.searchParams.get('token') ?? '';
	const email = event.url.searchParams.get('email') ?? '';
	return { token, email, authed: !!event.locals.user };
};

export const actions: Actions = {
	unsubscribe: async (event) => {
		const ctx = apiContext(event);
		const form = await event.request.formData();
		const token = (form.get('token') as string | null)?.trim() ?? '';
		const email = (form.get('email') as string | null)?.trim() ?? '';

		const params = new URLSearchParams();
		if (token) params.set('token', token);
		else if (email) params.set('email', email);
		else return { error: 'missing_param' };

		const headers: Record<string, string> = {
			'content-type': 'application/json',
			host: ctx.host,
			'x-forwarded-host': ctx.host
		};
		if (ctx.token) headers.authorization = `Bearer ${ctx.token}`;

		try {
			const res = await ctx.fetch(`${API_V1}/newsletter/unsubscribe?${params.toString()}`, {
				method: 'DELETE',
				headers
			});
			if (!res.ok) {
				const body = await res.json().catch(() => ({}));
				return { error: body?.error?.code ?? 'unsubscribe_failed' };
			}
			return { ok: true };
		} catch {
			return { error: 'network_error' };
		}
	}
};
