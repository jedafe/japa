import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';
import { redirect } from '@sveltejs/kit';

export const load: PageServerLoad = async ({ parent }) => {
	const { token, user } = await parent();
	if (!user) throw redirect(303, '/login');

	const result = await apiFetch<Array<{
		id: string;
		subject: string;
		description: string;
		status: string;
		category: string;
		author_id: string;
		author_name: string;
		created_at: string;
		updated_at: string;
	}>>('/me/tickets?page_size=20&page_offset=0', {}, token ?? undefined);

	return {
		tickets: result.ok && result.data ? result.data : []
	};
};
