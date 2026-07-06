import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

interface BlogPost {
	id: string;
	title: string;
	slug: string;
	body: string;
	body_html: string;
	content_type: string;
	status: string;
	read_time_minutes: number;
	view_count: number;
	comment_count: number;
	share_count: number;
	created_at: string;
	published_at: string;
	author_id: string;
	field_values: string;
}

export const load: PageServerLoad = async ({ params, cookies }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	const result = await apiFetch<BlogPost>(`/content/${params.slug}`, { headers });

	if (!result.ok || !result.data) {
		throw { status: 404, message: 'Post not found' };
	}

	return { post: result.data };
};
