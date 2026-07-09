import type { LayoutServerLoad } from './$types';
import { API_BASE } from '$lib/server/api';

export const load: LayoutServerLoad = async ({ locals, cookies }) => {
	const token = cookies.get('control_token');
	const localeCode = cookies.get('jc_locale') ?? 'en';

	let translations: Record<string, string> = {};
	try {
		const res = await fetch(`${API_BASE}/i18n/packs/${localeCode}`);
		if (res.ok) {
			const body = await res.json();
			translations = body?.data ?? body ?? {};
		}
	} catch {
		// Graceful degradation — translations load fails silently
	}

	return {
		token: token ?? null,
		user: locals.user ?? null,
		locale: localeCode,
		translations
	};
};
