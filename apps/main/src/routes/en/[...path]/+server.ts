// Ref: japaconnect-engineering/sections/28-i18n.md (English routing)
// GET /en/[...path] — sets the jc_locale=en cookie and redirects to the
// canonical (non-prefixed) URL.
import { redirect } from '@sveltejs/kit';
import type { RequestHandler } from './$types';

export const GET: RequestHandler = ({ params, cookies }) => {
	cookies.set('jc_locale', 'en', { path: '/', maxAge: 60 * 60 * 24 * 365, sameSite: 'lax' });
	const target = '/' + (params.path ?? '');
	throw redirect(302, target);
};
