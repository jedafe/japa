import type { LayoutServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

export const load: LayoutServerLoad = async ({ locals, cookies, url }) => {
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

	// Detect locale from URL path prefix (/fr/...) or fall back to cookie/user preference
	const pathLocale = url.pathname.startsWith('/fr') ? 'fr' : null;
	const locale = pathLocale
		?? cookies.get('jc_locale')
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
