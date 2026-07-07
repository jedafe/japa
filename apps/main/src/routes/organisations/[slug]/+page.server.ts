import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';
import { error } from '@sveltejs/kit';

interface Organization {
	id: string;
	slug: string;
	name: string;
	headline?: string;
	bio?: string;
	logo_url?: string;
	is_public: boolean;
	member_count?: number;
	created_at?: string;
}

interface OrganizationMember {
	user_id: string;
	username: string;
	display_name?: string;
	avatar_url?: string;
	role: string;
}

export const load: PageServerLoad = async ({ params, cookies }) => {
	const token = cookies.get('jc_token');
	const auth = token ? { Authorization: `Bearer ${token}` } : {};

	const orgResult = await apiFetch<Organization>(`/organizations/${params.slug}`, {
		headers: auth
	});

	if (!orgResult.ok || !orgResult.data) {
		throw error(404, 'Organisation not found');
	}

	const org = orgResult.data;

	const membersResult = await apiFetch<{ members: OrganizationMember[] }>(
		`/organizations/${org.id}/members`,
		{ headers: auth }
	);

	return {
		org,
		members: membersResult.ok && membersResult.data ? membersResult.data.members : []
	};
};
