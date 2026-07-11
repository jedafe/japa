import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';
import { redirect } from '@sveltejs/kit';

interface ProfessionalProfile {
	id: string;
	slug: string;
	name: string;
	headline: string;
	skills: string[];
	is_public: boolean;
	author_user_id: string;
	created_at: string;
}

interface ProfileListResponse {
	profiles: ProfessionalProfile[];
	total: number;
	page: number;
	total_pages: number;
}

export const load: PageServerLoad = async ({ url, cookies, locals }) => {
	if (!locals.user) throw redirect(303, '/login');
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	const params = new URLSearchParams();
	const search = url.searchParams.get('search');
	if (search) params.set('search', search);

	const limit = url.searchParams.get('limit') || '20';
	const offset = url.searchParams.get('offset') || '0';
	params.set('limit', limit);
	params.set('offset', offset);

	const result = await apiFetch<ProfileListResponse>(`/profiles?${params.toString()}`, { headers });

	return {
		profiles: result.ok && result.data ? result.data.profiles : [],
		total: result.ok && result.data ? result.data.total : 0,
		totalPages: result.ok && result.data ? result.data.total_pages : 0,
		filters: { search }
	};
};
