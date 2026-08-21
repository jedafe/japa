import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { apiContext, resetPassword } from '$lib/server/auth';

export const POST: RequestHandler = async (event) => {
	const body = await event.request.json().catch(() => ({}));
	const token = body?.token;
	const newPassword = body?.new_password ?? body?.newPassword;
	const website = body?.website; // §41 Layer 2 honeypot

	if (!token || !newPassword) {
		throw error(400, { code: 'missing_fields', message: 'missing_fields' });
	}

	const ctx = apiContext(event);
	try {
		await resetPassword(ctx, token, newPassword, website);
		return json({ ok: true });
	} catch (err) {
		const status = err instanceof Error && 'status' in err ? (err as { status: number }).status : 500;
		const code = err instanceof Error && 'code' in err ? (err as { code: string }).code : 'api_error';
		const message = err instanceof Error ? err.message : 'Reset failed';
		throw error(status, { code: code || 'api_error', message: message || 'Request failed' });
	}
};
