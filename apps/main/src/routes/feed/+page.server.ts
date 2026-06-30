import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

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

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	const feedResult = await apiFetch<{ items: FeedItem[]; has_more: boolean }>('/feed/primary?limit=20', { headers });

	return {
		feed: feedResult.ok && feedResult.data ? feedResult.data.items : [],
		hasMore: feedResult.ok && feedResult.data ? feedResult.data.has_more : false
	};
};
