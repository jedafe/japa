import type { LayoutLoad } from './$types';
import { ensureLocale } from '$lib/i18n';

/**
 * Root universal load — runs after +layout.server.ts on both SSR and client
 * navigations. Awaiting the (lazily-chunked) locale dictionary here keeps
 * non-English dictionaries out of the eager bundle while guaranteeing SSR
 * and hydration render with identical strings. The server data is returned
 * verbatim so SvelteKit's merged-data typing is preserved.
 */
export const load: LayoutLoad = async ({ data }) => {
	if (typeof data?.locale === 'string' && data.locale) {
		await ensureLocale(data.locale);
	}
	return data;
};
