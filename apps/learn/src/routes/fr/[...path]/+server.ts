// Ref: japaconnect-engineering/sections/28-i18n.md (French routing)
// GET /fr/[...path] — sets the jc_locale=fr cookie and redirects to the
// canonical (non-prefixed) URL. This lets users bookmark /fr/my-learning and
// have the app render that page in French on every visit.
import { redirect } from '@sveltejs/kit';
import type { RequestHandler } from './$types';

export const GET: RequestHandler = ({ params, cookies }) => {
	cookies.set('jc_locale', 'fr', { path: '/', maxAge: 60 * 60 * 24 * 365, sameSite: 'lax' });
	const target = '/' + (params.path ?? '');
	throw redirect(302, target);
};
