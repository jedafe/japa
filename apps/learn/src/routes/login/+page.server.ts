import { fail, redirect } from '@sveltejs/kit';
import type { Actions } from './$types';

export const actions: Actions = {
	default: async ({ request, cookies, fetch }) => {
		const formData = await request.formData();
		const email = formData.get('email') as string;
		const password = formData.get('password') as string;

		if (!email || !password) {
			return fail(400, { error: 'Email and password are required.' });
		}

		const apiBase = import.meta.env.PRIVATE_API_BASE_URL ?? 'http://localhost:8080';
		const apiUrl = `${apiBase}/api/v1/auth/login`;

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

			const token = body.data?.token;
			if (!token) {
				return fail(500, { error: 'Login succeeded but no token was returned.' });
			}

			cookies.set('learn_token', token, {
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
