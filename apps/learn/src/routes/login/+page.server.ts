import { fail, redirect } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import { PUBLIC_JC_API_URL, PUBLIC_JC_ENV } from '$env/static/public';
import type { Actions } from './$types';

export const actions: Actions = {
	default: async ({ request, cookies, fetch, url }) => {
		const formData = await request.formData();
		const email = formData.get('email') as string;
		const password = formData.get('password') as string;

		if (!email || !password) {
			return fail(400, { error: 'Email and password are required.' });
		}

		const apiBase = PUBLIC_JC_API_URL ?? 'http://localhost:8080';
		const apiUrl = `${apiBase}/api/v1/auth/login`;

		let token: string | undefined;
		try {
			const res = await fetch(apiUrl, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ email, password })
			});

			const body = await res.json();

			if (!res.ok || body.error) {
				return fail(res.status, {
					error: body.error?.message || 'Login failed. Please check your credentials.'
				});
			}

			token = body.data?.access_token;
		} catch {
			return fail(502, { error: 'Could not reach authentication service.' });
		}

		if (!token) {
			return fail(500, { error: 'Login succeeded but no token was returned.' });
		}

		const cookieDomain = env.JC_COOKIE_DOMAIN || undefined;

		const isSecure = PUBLIC_JC_ENV !== 'development';

		cookies.set('jc_token', token, {
			path: '/',
			httpOnly: true,
			secure: isSecure,
			sameSite: 'lax',
			...(cookieDomain ? { domain: cookieDomain } : {}),
			maxAge: 60 * 60 * 24 * 7
		});

		// Honour ?redirectTo= from the auth guard, but only same-origin
		// paths — following arbitrary values would be an open redirect.
		const redirectTo = url.searchParams.get('redirectTo');
		let target = '/';
		if (redirectTo) {
			try {
				const dest = new URL(redirectTo, url.origin);
				if (dest.origin === url.origin) {
					target = dest.pathname + dest.search;
				}
			} catch {
				// Malformed value — fall back to home.
			}
		}

		redirect(302, target);
	}
};
