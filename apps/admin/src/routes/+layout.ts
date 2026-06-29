import type { LayoutLoad } from './$types';
import { browser } from '$app/environment';
import { locale } from '$lib/stores/i18n.svelte';

// SPA mode — adapter-static emits a fallback index.html shell, so no
// server-side rendering is needed. Every route is handled client-side.
export const ssr = false;

// Universal load replaces the former +layout.server.ts. In a static SPA
// there is no server runtime to populate `locals`, so the auth token is
// read directly from the cookie on the client.
export const load: LayoutLoad = async () => {
	let localeCode = 'en';
	let token: string | null = null;

	if (browser) {
		const tokenMatch = document.cookie.split('; ').find((c) => c.startsWith('admin_token='));
		token = tokenMatch?.split('=')[1] ?? null;

		const localeMatch = document.cookie.match(/jc_locale=([^;]+)/);
		if (localeMatch) localeCode = localeMatch[1];
	}

	const translations = await import(`$lib/locales/${localeCode}.json`)
		.then((m) => m.default)
		.catch(() => ({}));
	locale.init(localeCode, translations);

	return { token, user: null };
};
