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

export const load: PageServerLoad = async ({ url, cookies, locals }) => {
	if (!locals.user) throw redirect(303, '/login');
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	const search = url.searchParams.get('search');

	const params = new URLSearchParams();
	const pageSize = url.searchParams.get('limit') || '20';
	const offset = url.searchParams.get('offset') || '0';
	params.set('page_size', pageSize);
	params.set('page_offset', offset);

	const result = await apiFetch<ProfessionalProfile[]>(`/profiles?${params.toString()}`, {
		headers
	});

	let profiles = result.ok && Array.isArray(result.data) ? result.data : [];
	// The backend has no search param on /profiles; filter here.
	if (search) {
		const q = search.toLowerCase();
		profiles = profiles.filter(
			(p) =>
				p.name.toLowerCase().includes(q) ||
				p.headline?.toLowerCase().includes(q) ||
				p.skills?.some((s) => s.toLowerCase().includes(q))
		);
	}

	return {
		profiles,
		total: profiles.length,
		totalPages: 1,
		filters: { search }
	};
};
