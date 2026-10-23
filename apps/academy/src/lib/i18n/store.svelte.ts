import { page } from '$app/state';
import { DEFAULT_SITE_NAME } from '$lib/brand';
import en from './en.json';
import fr from './fr.json';

export type Locale = string;
type Dict = Record<string, string>;

// Static dictionaries for built-in locales. Additional locales resolved at
// runtime fall back to English per Section 18. ADR-013 — runtime i18n.
const dictionaries: Record<string, Dict> = { en, fr };

// Non-bundled locales are served by the DB pack: the root layout load fetches
// /i18n/packs/{locale} and returns it as `i18nPack` in page data.
function runtimePack(): Dict | undefined {
	const p = (page.data as { i18nPack?: Dict } | undefined)?.i18nPack;
	return p && typeof p === 'object' ? p : undefined;
}

// §18 — every locale is layered over English so a missing key in the active
// dictionary resolves to real English text instead of the raw key. Merged
// dicts are memoized: bundled per locale, runtime pack per pack reference.
const mergedBundled: Record<string, Dict> = {};
let mergedPack: { src: Dict; dict: Dict } | null = null;

function dictFor(locale: string): Dict {
	if (locale === 'en') return dictionaries.en;
	const bundled = dictionaries[locale];
	if (bundled) {
		return (mergedBundled[locale] ??= { ...dictionaries.en, ...bundled });
	}
	const pack = runtimePack();
	if (!pack) return dictionaries.en;
	if (mergedPack?.src !== pack) mergedPack = { src: pack, dict: { ...dictionaries.en, ...pack } };
	return mergedPack.dict;
}

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

	t = (key: string, fallback?: string): string => {
		const dict = dictFor(this.locale);
		return substituteSiteName(dict[key] ?? fallback ?? key, this.siteName);
	};
}

export const i18n = new I18nStore();

/** Convenience binding: `t('nav.compose')`. */
export const t = i18n.t;

/** Convenience binding for non-i18n markup: `{siteName()}`. */
export function siteName(): string {
	return i18n.siteName;
}
