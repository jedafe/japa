import { fail, redirect } from '@sveltejs/kit';
import type { Actions } from './$types';
import { API_BASE } from '$lib/server/api';

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
			const res = await fetch(`${API_BASE}/auth/login`, {
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

			token = body.data?.token;
		} catch {
			return fail(502, { error: 'Could not reach authentication service.' });
		}

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

		redirect(302, '/');
	}
};
