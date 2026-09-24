// Quick-save target for the browser extension's "Save" card action.
// Extension tokens are read-only (Phase 3 spec), so the save deep-links
// here and bookmarks via the caller's session cookie instead of an
// API-key write.
// Ref: docs/superpowers/specs/2026-09-24-agent-data-api-phase3-design.md
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiPost, ApiError } from '$lib/server/api';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const contentID = event.url.searchParams.get('content_id') ?? '';
	if (!/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(contentID)) {
		throw error(400, { code: 'missing_content_id', message: 'A valid content_id is required' });
	}

	// `return` is only ever a same-origin path — never an open redirect.
	const ret = event.url.searchParams.get('return') ?? '';
	const returnHref = ret.startsWith('/') && !ret.startsWith('//') ? ret : '';

	try {
		await apiPost(apiContext(event), `/content/${contentID}/bookmark`, {});
	} catch (err) {
		// Invisible/absent content surfaces as 404/403 — same privacy
		// semantics as the API-key bookmark endpoint.
		if (err instanceof ApiError && (err.status === 404 || err.status === 403)) {
			return { saved: false, code: 'not_found' as const, returnHref };
		}
		throw error(500, { code: 'save_failed', message: 'Could not save this item right now' });
	}
	return { saved: true, code: 'saved' as const, returnHref };
};
