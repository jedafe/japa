import { fail, redirect } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
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

		cookies.set('jc_token', token, {
			path: '/',
			httpOnly: true,
			secure: true,
			sameSite: 'lax',
			...(cookieDomain ? { domain: cookieDomain } : {}),
			maxAge: 60 * 60 * 24 * 7
		});

		redirect(302, '/');
	}
};
