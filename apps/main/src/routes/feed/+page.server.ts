import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

interface FeedItem {
	id: string;
	title: string;
	content_type: string;
	author_id: string;
	author_name: string;
	author_avatar_url?: string;
	excerpt?: string;
	vote_score: number;
	comment_count: number;
	bookmark_count?: number;
	view_count?: number;
	created_at: string;
	slug?: string;
	reading_time_minutes?: number;
	cover_image_url?: string;
	tags?: string[];
}

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	const [feedResult, cardStyleResult] = await Promise.all([
		apiFetch<{ data: FeedItem[] }>('/feed?limit=20', { headers }),
		apiFetch<{ data: { card_style: string } }>('/feed/card-style', { headers })
	]);

	return {
		feed: feedResult.ok && feedResult.data ? feedResult.data.data : [],
		cardStyle: cardStyleResult.ok && cardStyleResult.data ? cardStyleResult.data.data.card_style : 'reddit'
	};
};
