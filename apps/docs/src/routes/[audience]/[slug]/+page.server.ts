import type { PageServerLoad } from './$types';
import { api } from '$lib/api';
import { error } from '@sveltejs/kit';

export const load: PageServerLoad = async ({ params }) => {
	const { audience, slug } = params;
	try {
		const doc = await api.getDocPage(audience, slug);
		return { doc };
	} catch {
		return { doc: null };
	}
};
