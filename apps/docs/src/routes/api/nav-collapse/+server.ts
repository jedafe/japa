import { json, error } from '@sveltejs/kit';
import type { RequestHandler } from './$types';

/**
 * Persist the left-nav collapsed state to a durable `jc_nav_collapsed` cookie
 * (ADR-058 — server-persistence, never localStorage).
 *
 * The backend PATCH /me/account does not currently accept `ui` preferences,
 * so a cookie is the source of truth. The SvelteKit hooks read this cookie on
 * every request to set event.locals.navCollapsed.
 *
 * POST /api/nav-collapse  body: { collapsed: boolean }
 */
export const POST: RequestHandler = async (event) => {
	const body = (await event.request.json().catch(() => null)) as { collapsed?: boolean } | null;
	if (!body || typeof body.collapsed !== 'boolean') {
		throw error(400, { code: 'invalid_body', message: 'collapsed (boolean) is required' });
	}

	const isHttps = event.url.protocol === 'https:';
	event.cookies.set('jc_nav_collapsed', body.collapsed ? 'true' : 'false', {
		path: '/',
		maxAge: 60 * 60 * 24 * 365,
		httpOnly: false,
		secure: isHttps,
		sameSite: 'lax'
	});

	return json({ ok: true, persisted: true });
};
