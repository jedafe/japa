import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';
import type { Track } from '$lib/api';

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('learn_token') ?? cookies.get('jc_token');

	try {
		const result = await apiFetch<Track[]>('/learning/tracks', {}, token);
		return { tracks: result.ok ? result.data ?? [] : [] };
	} catch {
		return { tracks: [] };
	}
};
