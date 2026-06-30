import type { PageLoad } from './$types';
import { error } from '@sveltejs/kit';
import { api } from '$lib/api';

export const load: PageLoad = async ({ params }) => {
	try {
		const track = await api.getTrack(params.slug);
		return { track };
	} catch (e) {
		if (e instanceof Error && e.message.includes('404')) {
			throw error(404, { code: 'NOT_FOUND', message: 'Track not found' });
		}
		throw error(500, { code: 'LOAD_ERROR', message: 'Failed to load track' });
	}
};
