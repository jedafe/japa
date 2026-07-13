import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

export const load: PageServerLoad = async ({ parent, url }) => {
	const { token } = await parent();
	const spaceId = url.searchParams.get('spaceId') || '';

	if (!spaceId) {
		return { members: [] };
	}

	const result = await apiFetch<Array<{
		user_id: string;
		role: string;
		joined_at: string;
	}>>(`/profiles/${spaceId}/members`, {}, token ?? undefined);

	return {
		members: result.ok && result.data ? result.data : []
	};
};
