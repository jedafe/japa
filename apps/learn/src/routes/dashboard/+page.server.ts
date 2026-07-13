import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';
import type { Enrollment } from '$lib/api';

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('learn_token') ?? cookies.get('jc_token');

	try {
		const result = await apiFetch<Enrollment[]>('/me/enrollments', {}, token);
		return { enrollments: result.ok ? result.data ?? [] : [] };
	} catch {
		return { enrollments: [] };
	}
};
