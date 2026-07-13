import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

export const load: PageServerLoad = async ({ parent }) => {
	const { token } = await parent();

	const result = await apiFetch<Array<{
		id: string;
		name: string;
		description?: string;
		item_count: number;
		created_at: string;
	}>>('/me/bookmark-folders', {}, token ?? undefined);

	return {
		folders: result.ok && result.data ? result.data : []
	};
};
