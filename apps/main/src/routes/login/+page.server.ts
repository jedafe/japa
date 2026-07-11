import { fail, redirect } from '@sveltejs/kit';
import type { Actions, PageServerLoad } from './$types';
import { API_BASE_URL } from '$lib/server/api';

export const load: PageServerLoad = async ({ locals }) => {
	// If already logged in, redirect to home
	if (locals.user) {
		throw redirect(303, '/');
	}
	return {};
};

export const actions: Actions = {
	default: async ({ request, cookies }) => {
		const formData = await request.formData();
		const email = formData.get('email')?.toString() ?? '';
		const password = formData.get('password')?.toString() ?? '';

		if (!email || !password) {
			return fail(400, { error: 'Email and password are required' });
		}

		try {
			const res = await fetch(`${API_BASE_URL}/auth/login`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ email, password })
			});

			const body = await res.json();

			if (!res.ok) {
				const message = body?.error?.message ?? 'Login failed. Please try again.';
				return fail(400, { error: message });
			}

			const token = body?.data?.access_token;
			if (!token) {
				return fail(400, { error: 'No token received from server' });
			}

			// Set the session cookie server-side with proper security attributes
			cookies.set('jc_token', token, {
				path: '/',
				httpOnly: true,
				secure: true,
				sameSite: 'lax',
				maxAge: 60 * 60 * 24 * 30 // 30 days
			});
		} catch {
			return fail(500, { error: 'Network error. Please try again.' });
		}

		throw redirect(303, '/');
	}
};
