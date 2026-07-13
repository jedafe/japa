import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

export const load: PageServerLoad = async ({ params, parent }) => {
	const { token } = await parent();

	const result = await apiFetch<{
		id: string;
		track_title: string;
		issued_at: string;
		download_url: string;
	}>('/certificates/' + params.id, {}, token ?? undefined);

	if (!result.ok || !result.data) {
		error(404, { code: 'NOT_FOUND', message: 'Certificate not found' });
	}

	return { certificate: result.data };
};
