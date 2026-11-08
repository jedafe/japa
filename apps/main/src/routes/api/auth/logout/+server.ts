// Ref: japaconnect-engineering/sections/02-authentication-sessions.md

import type { RequestHandler } from './$types';
import { apiContext, backendLogout, clearSessionCookie } from '#lib/server/auth.js';

export const POST: RequestHandler = async (event) => {
	const ctx = apiContext(event);
	await backendLogout(ctx);
	clearSessionCookie(event);
	return Response.json({ ok: true });
};
