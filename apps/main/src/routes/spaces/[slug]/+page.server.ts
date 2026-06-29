import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

interface SpaceRule {
	id: string;
	title: string;
	description: string;
}

interface Space {
	id: string;
	name: string;
	slug: string;
	description: string;
	rules: SpaceRule[];
	space_type: string;
	member_count: number;
	created_by: string;
}

export const load: PageServerLoad = async ({ params, cookies }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	const result = await apiFetch<Space>(`/spaces/${params.slug}`, { headers });

	if (!result.ok || !result.data) {
		throw { status: 404, message: 'Space not found' };
	}

	return { space: result.data };
};
