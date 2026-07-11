import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

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
	const auth: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	const orgResult = await apiFetch<Organization>(`/organizations/${params.slug}`, {
		headers: auth
	});

	if (!orgResult.ok || !orgResult.data) {
		throw new Response('Organisation not found', { status: 404 });
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
