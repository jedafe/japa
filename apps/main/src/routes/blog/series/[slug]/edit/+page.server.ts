import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';
import { error } from '@sveltejs/kit';

interface BlogSeries {
	id: string;
	title: string;
	slug: string;
	description: string;
	status: string;
}

export const load: PageServerLoad = async ({ params, cookies }) => {
	const token = cookies.get('jc_token');
	if (!token) throw error(401, { code: 'unauthorized', message: 'Authentication required' });

	const headers = { Authorization: `Bearer ${token}` };

	const result = await apiFetch<BlogSeries>(`/blog/series/${params.slug}`, { headers });

	if (!result.ok || !result.data) {
		throw error(404, { code: 'not_found', message: 'Series not found' });
	}

	return { series: result.data };
};
