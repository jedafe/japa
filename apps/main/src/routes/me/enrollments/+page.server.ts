import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

type Enrollment = {
	id: string;
	track_title: string;
	progress_percent: number;
	enrolled_at: string;
};

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('jc_token');

	try {
		const result = await apiFetch<Enrollment[]>('/me/enrollments', {}, token);
		if (result.ok) {
			return { data: result.data };
		}
		return { data: null, error: result.error };
	} catch {
		return { data: null, error: 'Failed to load' };
	}
};
