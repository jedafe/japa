import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/api/client';

interface LandingPage {
	id: string;
	title: string;
	slug: string;
	page_type: string;
	status: string;
	updated_at: string;
}

export const load: PageServerLoad = async ({ fetch }) => {
	const result = await apiFetch<LandingPage[]>('/admin/pages?page_type=landing', {}, fetch);
	return {
		pages: result.data ?? [],
		error: result.error ? result.error.message : null,
	};
};
