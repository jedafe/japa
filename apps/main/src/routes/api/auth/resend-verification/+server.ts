
import type { RequestHandler } from './$types';
import { apiContext, resendVerification } from '#lib/server/auth.js';

export const POST: RequestHandler = async (event) => {
	const body = await event.request.json().catch(() => ({}));
	const email = body?.email;

	if (!email) {
		return Response.json({ error: { code: 'missing_fields', message: 'Email is required.', status: 400 } }, { status: 400 });
	}

	const ctx = apiContext(event);
	try {
		await resendVerification(ctx, email);
		return Response.json({ ok: true });
	} catch (err) {
		const status = err instanceof Error && 'status' in err ? (err as { status: number }).status : 500;
		const code = err instanceof Error && 'code' in err ? (err as { code: string }).code : 'api_error';
		const message = err instanceof Error ? err.message : 'Resend failed';
		return Response.json({ error: { code: code || 'api_error', message: message || 'Request failed', status } }, { status });
	}
};
