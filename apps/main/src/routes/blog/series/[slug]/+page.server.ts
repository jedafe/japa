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

interface SeriesPost {
	series_id: string;
	content_id: string;
	position: number;
	content_id_2: string;
	title: string;
	slug: string;
	status: string;
	created_at: string;
}

export const load: PageServerLoad = async ({ params, cookies }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	const seriesResult = await apiFetch<BlogSeries>(`/blog/series/${params.slug}`, { headers });

	if (!seriesResult.ok || !seriesResult.data) {
		throw { status: 404, message: 'Series not found' };
	}

	const postsResult = await apiFetch<SeriesPost[]>(`/blog/series/${seriesResult.data.id}/posts`, { headers });

	return {
		series: seriesResult.data,
		posts: postsResult.ok && postsResult.data ? postsResult.data : []
	};
};
