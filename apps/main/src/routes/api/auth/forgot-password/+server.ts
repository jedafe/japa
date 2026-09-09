import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { apiContext, requestPasswordReset } from '$lib/server/auth';
import { ApiError } from '$lib/server/api';

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
	} catch (e) {
		// The backend already returns 200 for missing email addresses, so any
		// non-2xx response here is a real infrastructure/rate-limit error.
		const err = e instanceof ApiError ? e : new ApiError(500, 'api_error', 'Something went wrong. Please try again.');
		return json(
			{ error: { code: err.code, message: err.message, status: err.status } },
			{ status: err.status }
		);
	}
};
