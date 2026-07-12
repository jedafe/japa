import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';
import { error } from '@sveltejs/kit';

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

interface Space {
	id: string;
	name: string;
	slug: string;
	description: string;
}

export const load: PageServerLoad = async ({ params, cookies }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	const [spaceResult, feedResult, cardStyleResult] = await Promise.all([
		apiFetch<Space>(`/spaces/${params.spaceId}`, { headers }),
		apiFetch<{ data: FeedItem[] }>(`/feed/space/${params.spaceId}?limit=20`, { headers }),
		apiFetch<{ data: { card_style: string } }>('/feed/card-style', { headers })
	]);

	if (!spaceResult.ok || !spaceResult.data) {
		throw error(404, { code: 'NOT_FOUND', message: 'Space not found' });
	}

	return {
		space: spaceResult.data,
		feed: feedResult.ok && feedResult.data ? feedResult.data.data : [],
		cardStyle: cardStyleResult.ok && cardStyleResult.data ? cardStyleResult.data.data.card_style : 'reddit'
	};
};
