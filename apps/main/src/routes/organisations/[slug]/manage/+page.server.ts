import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';
import { error, redirect } from '@sveltejs/kit';

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

interface Invitation {
	id: string;
	email: string;
	role: string;
	created_at?: string;
	expires_at?: string;
}

export const load: PageServerLoad = async ({ params, cookies }) => {
	const token = cookies.get('jc_token');
	if (!token) throw redirect(302, '/login');

	const auth = { Authorization: `Bearer ${token}` };

	const orgResult = await apiFetch<Organization>(`/organizations/${params.slug}`, {
		headers: auth
	});

	if (!orgResult.ok || !orgResult.data) {
		throw error(404, { code: 'not_found', message: 'Organisation not found' });
	}

	const org = orgResult.data;

	const [membersResult, invitationsResult] = await Promise.all([
		apiFetch<OrganizationMember[]>(`/organizations/${org.id}/members`, { headers: auth }),
		apiFetch<Invitation[]>(`/organizations/${org.id}/invitations`, { headers: auth })
	]);

	return {
		org,
		members: membersResult.ok && Array.isArray(membersResult.data) ? membersResult.data : [],
		invitations:
			invitationsResult.ok && Array.isArray(invitationsResult.data) ? invitationsResult.data : []
	};
};
