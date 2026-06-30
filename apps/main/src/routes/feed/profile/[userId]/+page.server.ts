import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';
import { error } from '@sveltejs/kit';

interface FeedItem {
	id: string;
	title: string;
	content_type: string;
	author_id: string;
	author_name: string;
	author_avatar?: string;
	excerpt?: string;
	reaction_count: number;
	comment_count: number;
	created_at: string;
	slug?: string;
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

	const [profileResult, feedResult] = await Promise.all([
		apiFetch<UserProfile>(`/users/${params.userId}`, { headers }),
		apiFetch<{ items: FeedItem[]; has_more: boolean }>(`/feed/profile/${params.userId}?limit=20`, { headers })
	]);

	if (!profileResult.ok || !profileResult.data) {
		throw error(404, { code: 'NOT_FOUND', message: 'User not found' });
	}

	return {
		profile: profileResult.data,
		feed: feedResult.ok && feedResult.data ? feedResult.data.items : [],
		hasMore: feedResult.ok && feedResult.data ? feedResult.data.has_more : false
	};
};
