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

interface UserProfile {
	id: string;
	username: string;
	display_name: string;
	bio?: string;
	avatar_url?: string;
}

export const load: PageServerLoad = async ({ params, cookies }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	const [profileResult, feedResult, cardStyleResult] = await Promise.all([
		apiFetch<UserProfile>(`/users/${params.userId}`, { headers }),
		apiFetch<{ data: FeedItem[] }>(`/feed/profile/${params.userId}?limit=20`, { headers }),
		apiFetch<{ data: { card_style: string } }>('/feed/card-style', { headers })
	]);

	if (!profileResult.ok || !profileResult.data) {
		throw error(404, { code: 'NOT_FOUND', message: 'User not found' });
	}

	return {
		profile: profileResult.data,
		feed: feedResult.ok && feedResult.data ? feedResult.data.data : [],
		cardStyle: cardStyleResult.ok && cardStyleResult.data ? cardStyleResult.data.data.card_style : 'reddit'
	};
};
