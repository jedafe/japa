import type { LayoutLoad } from './$types';
import { browser } from '$app/environment';
import { locale } from '$lib/stores/i18n.svelte';

export const ssr = false;
export const prerender = false;

export const load: LayoutLoad = async ({ url }) => {
	let localeCode = 'en';

	if (browser) {
		const localeMatch = document.cookie.match(/jc_locale=([^;]+)/);
		if (localeMatch) localeCode = localeMatch[1];
	}

	const translations = await import(`$lib/locales/${localeCode}.json`)
		.then((m) => m.default)
		.catch(() => ({}));
	locale.init(localeCode, translations);

	// In SPA mode, auth is handled client-side via hooks.server.ts
	// and the API client. No server-side data needed.
	return {};
};
