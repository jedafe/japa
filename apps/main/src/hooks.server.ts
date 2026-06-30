import { redirect, type Handle, type HandleServerError } from '@sveltejs/kit';
import { API_BASE_URL, apiFetch } from '$lib/server/api';
import type { FeatureFlags } from '$lib/types';

/** Routes that require an authenticated user. */
const PROTECTED_ROUTES = [
	'/notifications',
	'/settings',
	'/settings/security',
	'/create',
	'/onboarding'
];

/** Check if a pathname requires authentication. */
function isProtectedRoute(pathname: string): boolean {
	return PROTECTED_ROUTES.some(
		(route) => pathname === route || pathname.startsWith(`${route}/`)
	);
}

export const handle: Handle = async ({ event, resolve }) => {
	const token = event.cookies.get('jc_token');

	if (token) {
		try {
			const res = await fetch(`${API_BASE_URL}/users/me`, {
				headers: { Authorization: `Bearer ${token}` }
			});
			if (res.ok) {
				const { data } = await res.json();
				event.locals.user = {
					id: data.id,
					email: data.email,
					username: data.username,
					displayName: data.display_name,
					avatarUrl: data.avatar_url,
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

	// Fetch feature flags from the backend.
	let featureFlags: FeatureFlags = {};
	try {
		const flagsResult = await apiFetch<FeatureFlags>('/features');
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
	return {
		code: 'INTERNAL_ERROR',
		message: 'Internal error'
	};
};
