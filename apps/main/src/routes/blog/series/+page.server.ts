import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

interface BlogSeries {
	id: string;
	title: string;
	slug: string;
	description: string;
	status: string;
	created_at: string;
}

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	const result = await apiFetch<BlogSeries[]>('/blog/series?page_size=50', { headers });

	return {
		series: result.ok && result.data ? result.data : []
	};
};
