import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { apiContext, requestPasswordReset } from '$lib/server/auth';

export const POST: RequestHandler = async (event) => {
	const body = await event.request.json().catch(() => ({}));
	const email = body?.email;
	const website = body?.website; // §41 Layer 2 honeypot

	if (!email) {
		throw error(400, { code: 'missing_fields', message: 'missing_fields' });
	}

	const ctx = apiContext(event);
	try {
		await requestPasswordReset(ctx, email, website);
		return json({ ok: true });
	} catch {
		// Don't reveal whether the email exists.
		return json({ ok: true });
	}
};
