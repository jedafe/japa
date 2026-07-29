// Ref: japaconnect-engineering/sections/28-i18n.md (French routing)
// GET /fr — sets the jc_locale=fr cookie and redirects to the homepage.
// The root layout reads the cookie and sets the i18n store locale, so every
// page renders in French without duplicating the route tree.
import { redirect } from '@sveltejs/kit';
import type { RequestHandler } from './$types';

export const GET: RequestHandler = ({ cookies }) => {
	cookies.set('jc_locale', 'fr', { path: '/', maxAge: 60 * 60 * 24 * 365, sameSite: 'lax' });
	throw redirect(302, '/');
};
