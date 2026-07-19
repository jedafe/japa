import { redirect, type Handle, type HandleServerError } from '@sveltejs/kit';
import { building, dev } from '$app/environment';
import * as Sentry from '@sentry/sveltekit';
import { env } from '$env/dynamic/private';

const API_BASE = env.PRIVATE_API_BASE_URL || 'http://localhost:8080';

const PUBLIC_ROUTES = ['/login', '/_app'];

function isPublicRoute(pathname: string): boolean {
	return PUBLIC_ROUTES.some((route) => pathname === route || pathname.startsWith(route + '/'));
}

// Dev-only auto-login is strictly gated behind SvelteKit's compile-time `dev` flag
// (which is stripped from production builds) AND an explicit opt-in env flag.
// It NEVER runs in production. The credentials are obviously-dev-only placeholders.
const DEV_LOGIN_ENABLED = dev && env.JC_DEV_LOGIN === 'true';
const DEV_LOGIN_EMAIL = 'dev-admin@local.dev';
const DEV_LOGIN_PASSWORD = 'dev-only-not-a-real-password';

// A limited, clearly-fake local dev user used only when the backend is
// unreachable during local development. It is NOT a super_admin — it carries
// the minimal `dev_viewer` group so the UI can render without implying real
// privileges. This never impersonates a real-looking super admin.
function createLocalDevUser(): App.Locals['user'] {
	return {
		id: 'dev-local-001',
		username: 'dev_local',
		email: DEV_LOGIN_EMAIL,
		displayName: 'Local Dev (backend offline)',
		groups: ['dev_viewer'],
		avatar_url: null,
	};
}

export const handle: Handle = async ({ event, resolve }) => {
	if (building) {
		return resolve(event);
	}

	if (isPublicRoute(event.url.pathname) || event.url.pathname.includes('.')) {
		return resolve(event);
	}

	const token = event.cookies.get('jc_token');

	// No token: dev auto-login is an explicit, dev-only convenience.
	if (!token) {
		if (DEV_LOGIN_ENABLED) {
			// Early exit: attempt real dev login against the backend.
			try {
				const loginRes = await fetch(`${API_BASE}/api/v1/auth/login`, {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({
						email: DEV_LOGIN_EMAIL,
						password: DEV_LOGIN_PASSWORD,
					}),
				});

				if (loginRes.ok) {
					const { data } = await loginRes.json();
					const realToken = data?.access_token;

					if (realToken) {
						event.cookies.set('jc_token', realToken, {
							path: '/',
							httpOnly: true,
							secure: false,
							sameSite: 'lax',
							maxAge: 3600 * 24 * 7,
						});

						const meRes = await fetch(`${API_BASE}/api/v1/me`, {
							headers: { Authorization: `Bearer ${realToken}` },
						});

						if (meRes.ok) {
							const { data: userData } = await meRes.json();
							event.locals.user = userData;
							event.locals.token = realToken;
							return await resolve(event);
						}
					}
				}
			} catch {
				// Backend unreachable in dev: fall back to a limited local dev user.
				event.locals.user = createLocalDevUser();
				event.locals.token = 'dev-mock-token';
				return await resolve(event);
			}

			// Dev login failed (wrong creds / backend rejected): show login page.
			throw redirect(303, '/login');
		}

		// Production (or dev without opt-in): require a real session.
		throw redirect(303, '/login');
	}

	// Valid token present: always delegate to the real backend session.
	try {
		const meRes = await fetch(`${API_BASE}/api/v1/me`, {
			headers: { Authorization: `Bearer ${token}` },
		});

		if (meRes.ok) {
			const { data } = await meRes.json();
			event.locals.user = data;
			event.locals.token = token;
		} else {
			event.cookies.delete('jc_token', { path: '/' });
			throw redirect(303, '/login');
		}
	} catch (e) {
		if (e instanceof Response) throw e;
		event.cookies.delete('jc_token', { path: '/' });
		throw redirect(303, '/login');
	}

	const response = await resolve(event);

	// Set cache headers based on path
	const path = event.url.pathname;

	if (path.startsWith('/_app/')) {
		// Static assets - cache aggressively (1 year, immutable)
		response.headers.set('Cache-Control', 'public, max-age=31536000, immutable');
	} else if (path.startsWith('/api/') || path.startsWith('/imgproxy/')) {
		// API and proxied responses - short cache with revalidation
		response.headers.set('Cache-Control', 'private, max-age=0, must-revalidate');
	} else if (path.match(/\.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2|ttf|eot)$/)) {
		// Static files - cache for 1 week
		response.headers.set('Cache-Control', 'public, max-age=604800, stale-while-revalidate=86400');
	} else {
		// HTML pages - no cache, must revalidate
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
