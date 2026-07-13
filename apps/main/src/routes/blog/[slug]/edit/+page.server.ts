import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';
import { error } from '@sveltejs/kit';

interface BlogPost {
	id: string;
	title: string;
	slug: string;
	body: string;
	body_html: string;
	content_type: string;
	status: string;
	field_values: string;
}

export const load: PageServerLoad = async ({ params, cookies }) => {
	const token = cookies.get('jc_token');
	if (!token) throw error(401, { code: 'unauthorized', message: 'Authentication required' });

	const headers = { Authorization: `Bearer ${token}` };

	const result = await apiFetch<BlogPost>(`/content/${params.slug}`, { headers });

	if (!result.ok || !result.data) {
		throw error(404, { code: 'not_found', message: 'Post not found' });
	}

	const post = result.data;
	if (post.content_type !== 'blog_post') {
		throw error(404, { code: 'not_found', message: 'Not a blog post' });
	}

	return { post };
};
