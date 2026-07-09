import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

interface Relocation {
	id: string;
	profile_id: string;
	country: string;
	city?: string;
	status: string;
	year?: number;
	created_at: string;
}

interface Document {
	id: string;
	profile_id: string;
	document_type: string;
	title: string;
	description?: string;
	file_url?: string;
	verified: boolean;
	created_at: string;
}

interface Verifier {
	id: string;
	profile_id: string;
	user_id: string;
	user_name: string;
	role: string;
	status: string;
	created_at: string;
}

interface ProfessionalProfile {
	id: string;
	slug: string;
	name: string;
	headline: string;
	skills: string[];
	is_public: boolean;
	author_user_id: string;
	original_author_id?: string;
	created_at: string;
	updated_at: string;
	relocations: Relocation[];
	documents: Document[];
	verifiers: Verifier[];
}

export const load: PageServerLoad = async ({ params, cookies }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	const result = await apiFetch<ProfessionalProfile>(`/profiles/${params.slug}`, { headers });

	if (!result.ok || !result.data) {
		throw { status: 404, message: 'Profile not found' };
	}

	return { profile: result.data };
};
