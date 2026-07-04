import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

interface Consent {
	id: string;
	consent_type: string;
	granted: boolean;
	created_at: string;
	updated_at: string;
}

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	const consentsResult = await apiFetch<Consent[]>('/consent', { headers });

	return {
		consents: consentsResult.ok && consentsResult.data ? consentsResult.data : [],
		isAuthenticated: !!token
	};
};
