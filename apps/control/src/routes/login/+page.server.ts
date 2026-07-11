import { fail, redirect } from '@sveltejs/kit';
import type { Actions } from './$types';
import { PUBLIC_JC_API_URL } from '$env/static/public';

export const actions: Actions = {
	default: async ({ request, cookies, fetch }) => {
		const formData = await request.formData();
		const email = formData.get('email') as string;
		const password = formData.get('password') as string;

		if (!email || !password) {
			return fail(400, { error: 'Email and password are required.' });
		}

		const apiBase = PUBLIC_JC_API_URL ?? 'http://localhost:8080/api/v1';

		try {
			const res = await fetch(`${apiBase}/auth/login`, {
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

			const token = body.data?.access_token;
			if (!token) {
				return fail(500, { error: 'Login succeeded but no token was returned.' });
			}

			cookies.set('jc_token', token, {
				path: '/',
				httpOnly: true,
				secure: true,
				sameSite: 'lax',
				domain: '.japaconnect.com',
				maxAge: 60 * 60 * 24 * 7
			});

			return redirect(302, '/');
		} catch {
			return fail(502, { error: 'Could not reach authentication service.' });
		}
	}
};
