import { page } from '$app/state';
import { DEFAULT_SITE_NAME } from '$lib/brand';
import en from './en.json';
import fr from './fr.json';
import jce_en from './jce_en.json';
import jce_fr from './jce_fr.json';

export type Locale = string;
type Dict = Record<string, string>;

// Static dictionaries for built-in locales. Additional locales resolved at
// runtime fall back to English per Section 18 ("Missing keys always fall back
// to English, never a blank string"). ADR-013 — runtime i18n.
// JCE keys are merged under the `jce.*` prefix so JCE public pages can reuse
// the same translator.
const dictionaries: Record<string, Dict> = {
	en: { ...en, ...jce_en },
	fr: { ...fr, ...jce_fr }
};

/**
 * Translator. Flat dotted keys resolved against the active locale's dictionary,
 * falling back to the provided fallback and then the key itself.
 *
 * The active locale is derived from the SvelteKit `page` store
 * (`page.data.locale`), which is request-scoped during SSR and reactive on the
 * client. This eliminates the cross-request leakage risk of a mutable module
 * singleton — each SSR request sees its own locale from its own page context,
 * and client-side locale switches (via the `jc_locale` cookie + `invalidateAll`)
 * propagate reactively without any manual `setLocale()` call.
 *
 * Brand name is never hardcoded: strings resolve through substituteSiteName,
 * which expands `{site}` and rewrites stray literal brand spellings to the
 * configured general.site_name.
 */
/** Default brand name — re-exported from $lib/brand so server modules can
 * use it without importing this runes-based store. */
export { DEFAULT_SITE_NAME };

/**
 * Substitute the configured site name into a translated string: replaces the
 * explicit `{site}` placeholder and any literal brand spelling that slipped
 * into a dictionary/fallback, so a renamed site never leaks the default name.
 */
export function substituteSiteName(value: string, name: string): string {
	return value
		.replaceAll('{site}', name)
		.replaceAll('Japa Connect', name)
		.replaceAll('JapaConnect', name);
}

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

	t = (key: string, fallback?: string, params?: Record<string, string | number>): string => {
		const dict = dictionaries[this.locale] ?? dictionaries.en;
		let value = dict[key] ?? fallback ?? key;
		if (params) {
			value = value.replace(/\{(\w+)\}/g, (match, p) => {
				const v = params[p];
				return v !== undefined ? String(v) : match;
			});
		}
		return substituteSiteName(value, this.siteName);
	};
}

export const i18n = new I18nStore();

/** Convenience binding: `t('nav.compose')`. */
export const t = i18n.t;

/** Convenience binding for non-i18n markup: `{siteName()}`. */
export function siteName(): string {
	return i18n.siteName;
}
