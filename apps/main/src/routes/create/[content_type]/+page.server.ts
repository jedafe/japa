import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

const validTypes = ['blog_post', 'discussion', 'guide', 'event', 'question', 'listing', 'comparison'] as const;

export const load: PageServerLoad = async ({ params }) => {
	if (!(validTypes as readonly string[]).includes(params.content_type)) {
		error(404, { code: 'NOT_FOUND', message: `Content type "${params.content_type}" not found` });
	}

	return {
		contentType: params.content_type
	};
};
