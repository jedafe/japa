import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

export const load: PageServerLoad = async ({ parent }) => {
	const { token } = await parent();

	const result = await apiFetch<{
		profiles: Array<{
			id: string;
			slug: string;
			name: string;
			headline: string;
			skills: string[];
			is_public: boolean;
		}>;
		total: number;
		page: number;
		total_pages: number;
	}>('/professional-profiles?limit=50', {}, token ?? undefined);

	return {
		organisations: result.ok && result.data ? result.data.profiles : []
	};
};
