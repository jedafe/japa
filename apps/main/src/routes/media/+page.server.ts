import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

export const load: PageServerLoad = async ({ parent }) => {
	const { token } = await parent();

	const result = await apiFetch<Array<{
		id: string;
		filename: string;
		url: string;
		mime_type: string;
		file_size: number;
		alt_text: string;
		created_at: string;
	}>>('/media?limit=50', {}, token ?? undefined);

	return {
		media: result.ok && result.data ? result.data : []
	};
};
