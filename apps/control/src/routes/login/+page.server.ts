import { fail, redirect } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import { PUBLIC_JC_ENV } from '$env/static/public';
import type { Actions } from './$types';
import { API_V1_BASE } from '$lib/server/api';

export const actions: Actions = {
	default: async ({ request, cookies, fetch }) => {
		const formData = await request.formData();
		const email = formData.get('email') as string;
		const password = formData.get('password') as string;

		if (!email || !password) {
			return fail(400, { error: 'Email and password are required.' });
		}

		let token: string | undefined;
		// Keep the network call in its own try/catch. In SvelteKit 2
		// redirect() throws, so it must live OUTSIDE this block — otherwise a
		// successful login's redirect is caught here and turned into a 502.
		try {
			const res = await fetch(`${API_V1_BASE}/auth/login`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ email, password })
			});

			const body = await res.json();

			if (!res.ok || body.error) {
				return fail(res.status, {
					error: body.error?.message || 'Invalid credentials.'
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

		redirect(302, '/');
	}
};
