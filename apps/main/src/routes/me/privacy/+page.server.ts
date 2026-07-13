import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

type PrivacySettings = {
	profile_visibility: string;
	show_email: boolean;
	show_online_status: boolean;
	allow_search_indexing: boolean;
};

type ConsentItem = {
	category: string;
	granted: boolean;
	granted_at?: string;
};

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('jc_token');

	try {
		const [privacyResult, consentResult] = await Promise.all([
			apiFetch<PrivacySettings>('/me/privacy', {}, token),
			apiFetch<ConsentItem[]>('/consent', {}, token)
		]);

		return {
			data: {
				privacy: privacyResult.ok ? privacyResult.data : null,
				consent: consentResult.ok ? consentResult.data : null
			}
		};
	} catch {
		return { data: null, error: 'Failed to load' };
	}
};
