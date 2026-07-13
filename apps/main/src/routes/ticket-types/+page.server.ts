import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

export const load: PageServerLoad = async ({ parent }) => {
	const { token } = await parent();

	const result = await apiFetch<Array<{
		id: string;
		name: string;
		description?: string;
	}>>('/ticket-types', {}, token ?? undefined);

	return {
		types: result.ok && result.data ? result.data : []
	};
};
