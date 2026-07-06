import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

interface BlogPost {
	id: string;
	title: string;
	slug: string;
	body_html: string;
	content_type: string;
	status: string;
	read_time_minutes: number;
	view_count: number;
	comment_count: number;
	created_at: string;
	published_at: string;
	author_id: string;
}

interface FeaturedPost {
	id: string;
	content_id: string;
	featured_order: number;
	featured_until: string;
	created_at: string;
}

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

	const [featuredResult, postsResult, seriesResult] = await Promise.all([
		apiFetch<FeaturedPost[]>('/blog/featured?page_size=6', { headers }),
		apiFetch<BlogPost[]>('/content?type=blog_post&page_size=12', { headers }),
		apiFetch<BlogSeries[]>('/blog/series?page_size=5', { headers })
	]);

	return {
		featured: featuredResult.ok && featuredResult.data ? featuredResult.data : [],
		posts: postsResult.ok && postsResult.data ? postsResult.data : [],
		series: seriesResult.ok && seriesResult.data ? seriesResult.data : []
	};
};
