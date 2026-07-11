import type { LayoutLoad } from './$types';
import { browser } from '$app/environment';
import { locale } from '$lib/stores/i18n.svelte';
import { fetchPermissions } from '$lib/stores/permissions';
import { apiFetch } from '$lib/api/client';
import { user, setPermissions } from '$lib/stores/auth';
import type { User } from '$lib/api/types';

export const ssr = false;

export const load: LayoutLoad = async () => {
	let localeCode = 'en';

	if (browser) {
		const localeMatch = document.cookie.match(/jc_locale=([^;]+)/);
		if (localeMatch) localeCode = localeMatch[1];
	}

	const translations = await import(`$lib/locales/${localeCode}.json`)
		.then((m) => m.default)
		.catch(() => ({}));
	locale.init(localeCode, translations);

	let userData: User | null = null;
	const result = await apiFetch<User>('/me');
	if (result.status !== 401) {
		userData = result.data;
	}

	if (browser) {
		user.set(userData);
	}

	await fetchPermissions(null);

	const permsResult = await apiFetch<{ permissions: string[] }>('/me/permissions');
	if (browser && permsResult.data) {
		setPermissions(permsResult.data.permissions ?? []);
	}

	return { user: userData };
};
