import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

export const load: PageServerLoad = async ({ parent }) => {
	const { token, user } = await parent();

	if (!user) {
		return { webhooks: [] };
	}

	const result = await apiFetch<Array<{
		id: string;
		url: string;
		events: string[];
		secret?: string;
		status: string;
		created_at: string;
	}>>('/webhooks', {}, token ?? undefined);

	return {
		webhooks: result.ok && result.data ? result.data : []
	};
};
