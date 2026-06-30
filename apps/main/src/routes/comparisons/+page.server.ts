import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

export const load: PageServerLoad = async ({ parent }) => {
	const { token } = await parent();

	const result = await apiFetch<Array<{
		id: string;
		name: string;
		slug: string;
		description: string;
		comparison_count: number;
	}>>('/comparison/types?limit=50', {}, token ?? undefined);

	return {
		types: result.ok && result.data ? result.data : []
	};
};
