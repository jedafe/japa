// Ref: japaconnect-engineering/sections/02-authentication-sessions.md (OAuth social login)
// Proxy for OAuth initiate. Returns { redirect_url } for the client to
// window.location.assign to. The backend callback handles the rest.
import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { apiContext, initiateOAuth } from '$lib/server/auth';

export const GET: RequestHandler = async (event) => {
	const provider = event.url.searchParams.get('provider');
	const redirect = event.url.searchParams.get('redirect') ?? '/';

	if (!provider) {
		return json({ error: { code: 'missing_provider', message: 'Provider is required.', status: 400 } }, { status: 400 });
	}

	const ctx = apiContext(event);
	try {
		const authUrl = await initiateOAuth(ctx, provider, redirect);
		return json({ redirect_url: authUrl });
	} catch (err) {
		const status = err instanceof Error && 'status' in err ? (err as { status: number }).status : 500;
		const code = err instanceof Error && 'code' in err ? (err as { code: string }).code : 'api_error';
		const message = err instanceof Error ? err.message : 'OAuth initiate failed';
		return json({ error: { code: code || 'api_error', message: message || 'Request failed', status } }, { status });
	}
};
