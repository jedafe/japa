import type { LayoutLoad } from './$types';
import { browser } from '$app/environment';
import { locale } from '$lib/stores/i18n.svelte';

export const load: LayoutLoad = async ({ data }) => {
	let localeCode = 'en';
	if (browser) {
		const match = document.cookie.match(/jc_locale=([^;]+)/);
		if (match) localeCode = match[1];
	}

	const translations = await import(`$lib/locales/${localeCode}.json`)
		.then((m) => m.default)
		.catch(() => ({}));
	locale.init(localeCode, translations);

	// A universal load REPLACES the server load's data rather than merging it,
	// so the server data (navTree, user, isAuth, token) must be passed through.
	return { ...data };
};
