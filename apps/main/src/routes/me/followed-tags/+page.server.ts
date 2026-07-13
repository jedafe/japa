import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

type FollowedTag = {
	tag: string;
	followed_at: string;
};

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('jc_token');

	try {
		const result = await apiFetch<FollowedTag[]>('/me/followed-tags', {}, token);
		if (result.ok) {
			return { data: result.data };
		}
		return { data: null, error: result.error };
	} catch {
		return { data: null, error: 'Failed to load' };
	}
};
