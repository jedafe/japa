import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { apiContext, requestPasswordReset } from '$lib/server/auth';

export const POST: RequestHandler = async (event) => {
	const body = await event.request.json().catch(() => ({}));
	const email = body?.email;
	const website = body?.website; // §41 Layer 2 honeypot

	if (!email) {
		return json({ error: { code: 'missing_fields', message: 'Email is required.', status: 400 } }, { status: 400 });
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
