import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

export const load: PageServerLoad = async ({ parent }) => {
	const { token } = await parent();

	const result = await apiFetch<Array<{
		id: string;
		name: string;
		slug: string;
		thread_count: number;
	}>>('/forum/tags', {}, token ?? undefined);

	return {
		tags: result.ok && result.data ? result.data : []
	};
};
