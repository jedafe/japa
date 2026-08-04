// Ref: japaconnect-engineering/sections/28-i18n.md (English routing)
// GET /en — sets the jc_locale=en cookie and redirects to the homepage.
import { redirect } from '@sveltejs/kit';
import type { RequestHandler } from './$types';

export const GET: RequestHandler = ({ cookies }) => {
	cookies.set('jc_locale', 'en', { path: '/', maxAge: 60 * 60 * 24 * 365, sameSite: 'lax' });
	throw redirect(302, '/');
};
