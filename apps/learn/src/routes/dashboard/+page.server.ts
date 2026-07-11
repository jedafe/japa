import type { PageServerLoad } from './$types';
import { PUBLIC_JC_API_URL } from '$env/static/public';

export const load: PageServerLoad = async ({ locals }) => {
	const user = locals.user;
	const token = locals.token;

	let enrollments: unknown[] = [];
	if (user && token) {
		try {
			const res = await fetch(`${PUBLIC_JC_API_URL}/api/v1/me/enrollments`, {
				headers: { Authorization: `Bearer ${token}` }
			});
			if (res.ok) {
				const { data } = await res.json();
				enrollments = data ?? [];
			}
		} catch {
			enrollments = [];
		}
	}

	return { enrollments };
};
