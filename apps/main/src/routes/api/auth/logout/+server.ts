// Ref: japaconnect-engineering/sections/02-authentication-sessions.md
import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { apiContext, backendLogout, clearSessionCookie } from '$lib/server/auth';

export const POST: RequestHandler = async (event) => {
	const ctx = apiContext(event);
	await backendLogout(ctx);
	clearSessionCookie(event);
	return json({ ok: true });
};
