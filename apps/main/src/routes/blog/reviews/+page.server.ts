import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

interface BlogReview {
	id: string;
	content_id: string;
	rating: number;
	title?: string;
	body?: string;
	author_id: string;
	status: string;
	created_at: string;
}

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	const result = await apiFetch<BlogReview[]>('/blog/reviews?page_size=50', { headers });

	return {
		reviews: result.ok && result.data ? result.data : []
	};
};
