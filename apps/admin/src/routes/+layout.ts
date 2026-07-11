import type { LayoutLoad } from './$types';
import { browser } from '$app/environment';
import { locale } from '$lib/stores/i18n.svelte';
import { fetchPermissions } from '$lib/stores/permissions';
import { apiFetch } from '$lib/api/client';
import type { User } from '$lib/api/types';

export const ssr = false;

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

	// jc_token is httpOnly (AUDIT-FE-ADM-001) — it can never be read via
	// document.cookie, only sent automatically by apiFetch's
	// `credentials: 'include'`. Fetch permissions/user unconditionally
	// instead of gating on a client-readable token. Skip on /login: apiFetch
	// redirects to /login on a 401, which would otherwise refetch and 401
	// again in a loop.
	let user: User | null = null;
	if (!url.pathname.startsWith('/login')) {
		await fetchPermissions();
		const result = await apiFetch<User>('/me');
		user = result.data;
	}

	// `token` is always null now — kept only so the legacy `$lib/api.ts`
	// bearer-token client (still used by users/+page.svelte and
	// users/[userId]/+page.svelte, see AUDIT-FE-ADM-007) keeps compiling.
	// That client's Authorization-header auth model is a separate, larger
	// issue and out of scope here.
	return { token: null as string | null, user };
};
