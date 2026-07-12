import type { LayoutLoad } from './$types';
import { browser } from '$app/environment';
import { locale } from '$lib/stores/i18n.svelte';
import type { User } from '$lib/api/types';

export const ssr = false;

export const load: LayoutLoad = async ({ url, data }) => {
	let localeCode = 'en';

	if (browser) {
		const localeMatch = document.cookie.match(/jc_locale=([^;]+)/);
		if (localeMatch) localeCode = localeMatch[1];
	}

	const translations = await import(`$lib/locales/${localeCode}.json`)
		.then((m) => m.default)
		.catch(() => ({}));
	locale.init(localeCode, translations);

	// Use server-side data from +layout.server.ts (which validates the token
	// via the backend /me endpoint). The server hooks already authenticated
	// the user and set locals.user/locals.token. No need to re-fetch client-side.
	return { token: data.token, user: data.user, permissions: data.permissions };
};
