import type { LayoutLoad } from './$types';
import { browser } from '$app/environment';
import { locale, loadTranslations } from '$lib/stores/i18n.svelte';

export const ssr = false;
export const prerender = false;

export const load: LayoutLoad = async ({ url }) => {
	let localeCode = 'en';

	if (browser) {
		const localeMatch = document.cookie.match(/jc_locale=([^;]+)/);
		if (localeMatch) localeCode = localeMatch[1];
	}

	// Built-in locales ship bundled dictionaries; anything else is served by
	// the DB pack (/i18n/packs/{locale}). §18 — every locale is layered over
	// English so a missing key resolves to English text, never a blank string.
	const enDict = localeCode === 'en' ? {} : await import('$lib/locales/en.json').then((m) => m.default);
	const translations =
		(await import(`$lib/locales/${localeCode}.json`)
			.then((m) => m.default)
			.catch(() => null)) ?? (await loadTranslations(localeCode));
	locale.init(localeCode, { ...enDict, ...translations });

	// In SPA mode, auth is handled client-side via hooks.server.ts
	// and the API client. No server-side data needed.
	return {};
};
