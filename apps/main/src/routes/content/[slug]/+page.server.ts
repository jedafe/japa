import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

export const load: PageServerLoad = async ({ params, parent }) => {
	const { token } = await parent();

	const result = await apiFetch<{
		id: string;
		title: string;
		slug: string;
		excerpt?: string;
		body?: string;
		author_id: string;
		author_name: string;
		author_avatar_url?: string;
		space_name?: string;
		content_type: string;
		vote_score: number;
		comment_count: number;
		bookmark_count?: number;
		view_count?: number;
		created_at: string;
		reading_time_minutes?: number;
		cover_image_url?: string;
		tags?: string[];
	}>('/content/' + params.slug, {}, token ?? undefined);

	if (!result.ok || !result.data) {
		error(404, { code: 'NOT_FOUND', message: 'Content not found' });
	}

	return { content: result.data };
};
