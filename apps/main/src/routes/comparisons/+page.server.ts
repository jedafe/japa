import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';
import { redirect } from '@sveltejs/kit';

export const load: PageServerLoad = async ({ parent }) => {
	const { token, user } = await parent();
	if (!user) throw redirect(303, '/login');

	const result = await apiFetch<Array<{
		id: string;
		name: string;
		slug: string;
		description: string;
		comparison_count: number;
	}>>('/comparisons/types?limit=50', {}, token ?? undefined);

	return {
		types: result.ok && result.data ? result.data : []
	};
};
