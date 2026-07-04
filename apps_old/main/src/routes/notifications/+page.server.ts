import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

export const load: PageServerLoad = async ({ parent }) => {
	const { token } = await parent();

	const result = await apiFetch<Array<{
		id: string;
		title: string;
		body: string;
		read: boolean;
		created_at: string;
	}>>('/notifications?limit=50', {}, token ?? undefined);

	return {
		notifications: result.ok && result.data ? result.data : []
	};
};
