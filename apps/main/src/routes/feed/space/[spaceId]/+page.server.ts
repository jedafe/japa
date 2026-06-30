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

interface Space {
	id: string;
	name: string;
	slug: string;
	description: string;
}

export const load: PageServerLoad = async ({ params, cookies }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	const [spaceResult, feedResult] = await Promise.all([
		apiFetch<Space>(`/spaces/${params.spaceId}`, { headers }),
		apiFetch<{ items: FeedItem[]; has_more: boolean }>(`/feed/space/${params.spaceId}?limit=20`, { headers })
	]);

	if (!spaceResult.ok || !spaceResult.data) {
		throw error(404, { code: 'NOT_FOUND', message: 'Space not found' });
	}

	return {
		space: spaceResult.data,
		feed: feedResult.ok && feedResult.data ? feedResult.data.items : [],
		hasMore: feedResult.ok && feedResult.data ? feedResult.data.has_more : false
	};
};
