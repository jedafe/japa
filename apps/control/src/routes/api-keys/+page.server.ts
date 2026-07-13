import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { API_BASE } from '$lib/server/api';

export const load: PageServerLoad = async ({ locals, cookies }) => {
	if (!locals.user) throw redirect(302, '/login');

	const token = cookies.get('jc_token');

	try {
		const res = await fetch(`${API_BASE}/control/api-keys`, {
			headers: {
				Authorization: `Bearer ${token}`,
				'Content-Type': 'application/json'
			}
		});

		if (!res.ok) {
			return { keys: null, error: `API ${res.status}`, user: locals.user };
		}

		const body = await res.json();
		const data = body?.data ?? body ?? {};

		return {
			keys: data ?? null,
			error: null,
			user: locals.user
		};
	} catch (e) {
		return {
			keys: null,
			error: e instanceof Error ? e.message : 'Network error',
			user: locals.user
		};
	}
};
