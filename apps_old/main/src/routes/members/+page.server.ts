import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

export const load: PageServerLoad = async ({ parent }) => {
	const { token } = await parent();

	const result = await apiFetch<Array<{
		id: string;
		username: string;
		display_name: string;
		avatar_url: string | null;
		bio: string | null;
		location: string | null;
	}>>('/users?limit=50', {}, token ?? undefined);

	return {
		members: result.ok && result.data ? result.data : []
	};
};
