import { page } from '$app/state';
import { tServer, type Locale } from './dict';

export type { Locale };

/**
 * Translator. Flat dotted keys resolved against the active locale's dictionary,
 * falling back to the provided fallback and then the key itself.
 *
 * The active locale is derived from the SvelteKit `page` store
 * (`page.data.locale`), which is request-scoped during SSR and reactive on the
 * client. This eliminates the cross-request leakage risk of a mutable module
 * singleton — each SSR request sees its own locale from its own page context,
 * and client-side locale switches (via the `jc_locale` cookie + page reload)
 * propagate reactively without any manual `setLocale()` call.
 */
class I18nStore {
	/** Active locale, derived from the request-scoped page data. */
	locale = $derived<Locale>(
		typeof page.data?.locale === 'string' && page.data.locale ? page.data.locale : 'en'
	);

	/** Interpolate {name} placeholders in the fallback string. */
	t = (key: string, fallback?: string, vars?: Record<string, string>): string =>
		tServer(this.locale, key, fallback, vars);
}

export const i18n = new I18nStore();

/** Convenience binding: `t('nav.search_placeholder')`. */
export const t = i18n.t;
