import { type Handle, type HandleServerError } from '@sveltejs/kit';
import { building } from '$app/environment';
import * as Sentry from '@sentry/sveltekit';

export const handle: Handle = async ({ event, resolve }) => {
	if (building) {
		return resolve(event);
	}

	const response = await resolve(event);

	// Baseline hardening headers (CSP itself is emitted by SvelteKit's
	// kit.csp nonce mode — see svelte.config.js).
	response.headers.set('X-Content-Type-Options', 'nosniff');
	response.headers.set('Referrer-Policy', 'strict-origin-when-cross-origin');
	response.headers.set('Permissions-Policy', 'camera=(), microphone=(), geolocation=(), payment=()');

	const path = event.url.pathname;

	if (path.startsWith('/_app/')) {
		response.headers.set('Cache-Control', 'public, max-age=31536000, immutable');
	} else if (path.startsWith('/api/') || path.startsWith('/imgproxy/')) {
		response.headers.set('Cache-Control', 'private, max-age=0, must-revalidate');
	} else if (path.match(/\.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2|ttf|eot)$/)) {
		response.headers.set('Cache-Control', 'public, max-age=604800, stale-while-revalidate=86400');
	} else {
		response.headers.set('Cache-Control', 'no-cache, must-revalidate');
		response.headers.set('Pragma', 'no-cache');
	}

	return response;
};

export const handleError: HandleServerError = ({ error }) => {
	Sentry.captureException(error);
	return {
		code: 'INTERNAL_ERROR',
		message: 'Internal error',
	};
};
