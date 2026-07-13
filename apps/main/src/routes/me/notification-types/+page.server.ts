import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

type NotificationType = {
	key: string;
	label: string;
	description?: string;
	enabled: boolean;
};

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('jc_token');

	try {
		const result = await apiFetch<NotificationType[]>('/me/notification-types', {}, token);
		if (result.ok) {
			return { data: result.data };
		}
		return { data: null, error: result.error };
	} catch {
		return { data: null, error: 'Failed to load' };
	}
};
