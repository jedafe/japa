import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

interface Organization {
	id: string;
	slug: string;
	name: string;
	headline?: string;
	logo_url?: string;
	is_public: boolean;
	member_count?: number;
}

export const load: PageServerLoad = async ({ parent }) => {
	const { token } = await parent();

	const result = await apiFetch<{
		organizations: Organization[];
		total: number;
		page: number;
		total_pages: number;
	}>('/organizations?limit=50', {}, token ?? undefined);

	return {
		organisations: result.ok && result.data ? result.data.organizations : []
	};
};
