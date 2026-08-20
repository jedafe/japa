import { json, error } from '@sveltejs/kit';
import { apiContext, apiPatch, ApiError } from '$lib/server/api';
import type { RequestHandler } from './$types';

/**
 * Persist the left-nav collapsed state to the user's ui.nav_collapsed preference
 * (ADR-058 — server-persistence, never localStorage). Guests get a no-op 200
 * since they have no /me/account; the toggle is purely visual for them.
 *
 * POST /api/nav-collapse  body: { collapsed: boolean }
 */
export const POST: RequestHandler = async (event) => {
	if (!event.locals.token || !event.locals.user) {
		// Guest: acknowledge but persist nothing.
		return json({ ok: true, persisted: false });
	}

	const body = (await event.request.json().catch(() => null)) as { collapsed?: boolean } | null;
	if (!body || typeof body.collapsed !== 'boolean') {
		throw error(400, { code: 'invalid_body', message: 'collapsed (boolean) is required' });
	}

	try {
		await apiPatch(apiContext(event), '/me/account', {
			ui: { nav_collapsed: body.collapsed }
		});
	} catch (err) {
		if (err instanceof ApiError) {
			throw error(err.status, { code: err.code, message: err.message });
		}
		throw error(500, { code: 'internal_error', message: 'Failed to persist preference' });
	}

	return json({ ok: true, persisted: true });
};
