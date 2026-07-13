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

	const result = await apiFetch<Organization[]>(
		'/organizations?page_size=50',
		{},
		token ?? undefined
	);

	return {
		organisations: result.ok && Array.isArray(result.data) ? result.data : []
	};
};
