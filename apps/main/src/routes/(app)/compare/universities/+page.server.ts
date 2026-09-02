import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async (event) => {
	const slug = event.url.searchParams.get('item1_slug') ?? '';
	const target = slug
		? `/compare?type=universities&source=university&preselect=${encodeURIComponent(slug)}`
		: '/compare?type=universities&source=university';
	throw redirect(307, target);
};
