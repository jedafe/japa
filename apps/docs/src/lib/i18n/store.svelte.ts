import { page } from '$app/state';
import { tServer, DEFAULT_SITE_NAME, type Locale } from './dict';

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

	/** Configured brand name (general.site_name) from the public site settings
	 * returned by the root layout load. Single fallback literal for the whole
	 * app — every other consumer must read this, never hardcode the name. */
	siteName = $derived<string>(
		(page.data?.siteSettings?.['general.site_name'] as string | undefined) ?? DEFAULT_SITE_NAME
	);

	/** Interpolate {name} placeholders in the fallback string. */
	t = (key: string, fallback?: string, vars?: Record<string, string>): string =>
		tServer(this.locale, key, fallback, vars, this.siteName);
}

export const i18n = new I18nStore();

/** Convenience binding: `t('nav.search_placeholder')`. */
export const t = i18n.t;

/** Convenience binding for non-i18n markup: `{siteName()}`. */
export function siteName(): string {
	return i18n.siteName;
}
