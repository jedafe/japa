import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { apiContext, resendVerification } from '$lib/server/auth';

export const POST: RequestHandler = async (event) => {
	const body = await event.request.json().catch(() => ({}));
	const email = body?.email;

	if (!email) {
		throw error(400, { code: 'missing_fields', message: 'missing_fields' });
	}

	const ctx = apiContext(event);
	try {
		await resendVerification(ctx, email);
		return json({ ok: true });
	} catch (err) {
		const status = err instanceof Error && 'status' in err ? (err as { status: number }).status : 500;
		const code = err instanceof Error && 'code' in err ? (err as { code: string }).code : 'api_error';
		const message = err instanceof Error ? err.message : 'Resend failed';
		throw error(status, { code: code || 'api_error', message: message || 'Request failed' });
	}
};
