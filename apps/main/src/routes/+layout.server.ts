import type { LayoutServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

export const load: LayoutServerLoad = async ({ locals, cookies }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	const flagsResult = await apiFetch<Record<string, boolean>>('/features');
	const flags = flagsResult.ok && flagsResult.data ? flagsResult.data : {};

	let notificationCount = 0;
	if (locals.user) {
		const countResult = await apiFetch<{ count: number }>('/notifications/unread-count', {
			headers
		});
		if (countResult.ok && countResult.data) {
			notificationCount = countResult.data.count;
		}
	}

	const locale = cookies.get('jc_locale')
		?? (locals.user as any)?.language
		?? 'en';

	const translationsResult = await apiFetch<Record<string, string>>(`/translations/${locale}`);
	const translations = translationsResult.ok && translationsResult.data ? translationsResult.data : {};

	return {
		user: locals.user,
		flags,
		notificationCount,
		token: token ?? null,
		locale,
		translations
	};
};
