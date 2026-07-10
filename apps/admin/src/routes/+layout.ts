import type { LayoutLoad } from './$types';
import { browser } from '$app/environment';
import { locale } from '$lib/stores/i18n.svelte';
import { fetchPermissions } from '$lib/stores/permissions';
import { apiFetch } from '$lib/api/client';
import type { User } from '$lib/api/types';

export const ssr = false;

export const load: LayoutLoad = async () => {
	let localeCode = 'en';
	let token: string | null = null;

	if (browser) {
		const tokenMatch = document.cookie.split('; ').find((c) => c.startsWith('jc_token='));
		token = tokenMatch?.split('=')[1] ?? null;

		const localeMatch = document.cookie.match(/jc_locale=([^;]+)/);
		if (localeMatch) localeCode = localeMatch[1];
	}

	const translations = await import(`$lib/locales/${localeCode}.json`)
		.then((m) => m.default)
		.catch(() => ({}));
	locale.init(localeCode, translations);

	if (token) {
		await fetchPermissions(token);
	}

	let user: User | null = null;
	if (token) {
		const result = await apiFetch<User>('/me');
		user = result.data;
	}

	return { token, user };
};
