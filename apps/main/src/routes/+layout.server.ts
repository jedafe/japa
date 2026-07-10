import type { LayoutServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

export const load: LayoutServerLoad = async ({ locals, cookies }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	let flags: Record<string, boolean> = {};
	try {
		const flagsResult = await apiFetch<Record<string, boolean>>('/feature-flags');
		if (flagsResult.ok && flagsResult.data) {
			flags = flagsResult.data;
		}
	} catch {
		// Feature flags are non-critical; degrade gracefully
	}

	let notificationCount = 0;
	if (locals.user) {
		try {
			const countResult = await apiFetch<{ count: number }>('/notifications/unread-count', {
				headers
			});
			if (countResult.ok && countResult.data) {
				notificationCount = countResult.data.count;
			}
		} catch {
			// Notification count is non-critical; default to 0
		}
	}

	const locale = cookies.get('jc_locale')
		?? (locals.user as any)?.language
		?? 'en';

	let translations: Record<string, string> = {};
	try {
		const translationsResult = await apiFetch<{ data: Record<string, string> }>(`/i18n/packs/${locale}`);
		if (translationsResult.ok && translationsResult.data?.data) {
			translations = translationsResult.data.data;
		}
	} catch {
		// Translations are non-critical; degrade gracefully
	}

	return {
		user: locals.user,
		flags,
		notificationCount,
		token: token ?? null,
		locale,
		translations
	};
};
