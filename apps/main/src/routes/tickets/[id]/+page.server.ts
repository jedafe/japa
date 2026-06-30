import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

export const load: PageServerLoad = async ({ params, parent }) => {
	const { token } = await parent();
	const { id } = params;

	const result = await apiFetch<{
		ticket: {
			id: string;
			subject: string;
			description: string;
			status: string;
			category: string;
			author_id: string;
			author_name: string;
			created_at: string;
			updated_at: string;
		};
		comments: Array<{
			id: string;
			ticket_id: string;
			body: string;
			author_id: string;
			author_name: string;
			created_at: string;
		}>;
		attachments: unknown[];
	}>('/support/my/' + id, {}, token ?? undefined);

	if (!result.ok || !result.data) {
		return { ticket: null, comments: [] };
	}

	return {
		ticket: result.data.ticket,
		comments: result.data.comments
	};
};
