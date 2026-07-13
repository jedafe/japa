import type { LayoutServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';
import { getCached } from '$lib/server/cache';

export const load: LayoutServerLoad = async ({ locals, cookies, url }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	const flags = await getCached<Record<string, boolean>>('feature-flags', 5 * 60 * 1000, async () => {
		try {
			const flagsResult = await apiFetch<Record<string, boolean>>('/feature-flags');
			if (flagsResult.ok && flagsResult.data) {
				return flagsResult.data;
			}
		} catch {
			// Feature flags are non-critical; degrade gracefully
		}
		return {};
	});

	let notificationCount = 0;
	if (locals.user) {
		notificationCount = await getCached<number>(`notifications:${locals.user.id}`, 60 * 1000, async () => {
			try {
				const countResult = await apiFetch<{ count: number }>('/me/notifications/unread-count', {
					headers
				});
				if (countResult.ok && countResult.data) {
					return countResult.data.count;
				}
			} catch {
				// Notification count is non-critical; default to 0
			}
			return 0;
		});
	}

	// Detect locale from URL path prefix (/fr/...) or fall back to cookie/user preference
	const pathLocale = url.pathname.startsWith('/fr') ? 'fr' : null;
	const locale = pathLocale
		?? cookies.get('jc_locale')
		?? (locals.user as any)?.language
		?? 'en';

	const translations = await getCached<Record<string, string>>(`translations:${locale}`, 5 * 60 * 1000, async () => {
		try {
			const translationsResult = await apiFetch<Record<string, string>>(`/i18n/packs/${locale}`);
			if (translationsResult.ok && translationsResult.data) {
				return translationsResult.data;
			}
		} catch {
			// Translations are non-critical; degrade gracefully
		}
		return {};
	});

	return {
		user: locals.user,
		flags,
		notificationCount,
		token: token ?? null,
		locale,
		translations
	};
};
