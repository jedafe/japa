import type { LayoutLoad } from './$types';
import { locale } from '$lib/stores/i18n.svelte';

// Initialise the i18n store during load so t() resolves during SSR too —
// without this, server-rendered HTML (including error pages, which never
// get a chance to re-render after hydration fails) shows raw i18n keys.
export const load: LayoutLoad = async ({ data }) => {
	locale.init(data.locale, data.translations);

	// A universal load replaces the server load's data, so pass it through.
	return data;
};
