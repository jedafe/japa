import { redirect, type Handle, type HandleServerError } from '@sveltejs/kit';
import * as Sentry from '@sentry/sveltekit';
import { API_BASE_URL, apiFetch } from '$lib/server/api';
import type { FeatureFlags } from '$lib/types';

const SUPPORTED_LOCALES = ['en', 'fr', 'yo', 'ha'];

/** Routes that require an authenticated user. */
const PROTECTED_ROUTES = [
	'/notifications',
	'/settings',
	'/settings/security',
	'/create',
	'/onboarding',
	'/messages',
	'/payments',
	'/checkout',
	'/dashboard',
	'/wallet',
	'/tickets',
	'/professional',
	'/spaces/new',
	'/comparisons/new'
];

/** Check if a pathname requires authentication. */
function isProtectedRoute(pathname: string): boolean {
	return PROTECTED_ROUTES.some(
		(route) => pathname === route || pathname.startsWith(`${route}/`)
	);
}

/** Resolve path-based locale (/fr/blog → /blog, sets cookie). */
function resolveLocale(pathname: string): { lang: string; cleanPath: string } | null {
	const match = pathname.match(/^\/(fr|en|yo|ha)(\/|$)/);
	if (!match) return null;
	return { lang: match[1], cleanPath: pathname.slice(match[0].length) || '/' };
}

export const reroute = ({ url }: { url: URL }) => {
	const result = resolveLocale(url.pathname);
	if (!result) return;
	const { lang, cleanPath } = result;
	const newUrl = new URL(url);
	newUrl.pathname = cleanPath;
	newUrl.searchParams.set('_lang', lang);
	return newUrl;
};

export const handle: Handle = async ({ event, resolve }) => {
	const token = event.cookies.get('jc_token');

	if (token) {
		try {
			const res = await fetch(`${API_BASE_URL}/me`, {
				headers: { Authorization: `Bearer ${token}` }
			});
			if (res.ok) {
				const { data } = await res.json();
				event.locals.token = token;
				event.locals.user = {
					id: data.id,
					email: data.email,
					username: data.username,
					displayName: data.display_name,
					avatarUrl: data.url_avatar,
					profileType: data.profile_type,
					status: data.status
				};
			} else {
				event.locals.user = null;
			}
		} catch {
			event.locals.user = null;
		}
	} else {
		event.locals.user = null;
	}

	// Redirect unauthenticated users away from protected routes.
	if (isProtectedRoute(event.url.pathname) && !event.locals.user) {
		throw redirect(303, '/login');
	}

	// Handle locale from path-based routing.
	const urlLang = event.url.searchParams.get('_lang');
	if (urlLang && SUPPORTED_LOCALES.includes(urlLang)) {
		event.cookies.set('jc_locale', urlLang, {
			path: '/',
			maxAge: 60 * 60 * 24 * 365,
			sameSite: 'lax'
		});
	}

	// Fetch feature flags from the backend.
	let featureFlags: FeatureFlags = {};
	try {
		const flagsResult = await apiFetch<FeatureFlags>('/feature-flags');
		if (flagsResult.ok && flagsResult.data) {
			featureFlags = flagsResult.data;
		}
	} catch {
		// Fall back to empty flags on error.
	}
	event.locals.featureFlags = featureFlags;
	event.locals.notificationCount = 0;

	const response = await resolve(event);
	return response;
};

export const handleError: HandleServerError = ({ error }) => {
	Sentry.captureException(error);
	return {
		code: 'INTERNAL_ERROR',
		message: 'Internal error'
	};
};
