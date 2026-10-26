import { page } from '$app/state';
import { DEFAULT_SITE_NAME } from '$lib/brand';
import en from './en.json';
import jce_en from './jce_en.json';

export type Locale = string;
type Dict = Record<string, string>;

// Static dictionaries for built-in locales. Additional locales resolved at
// runtime fall back to English per Section 18 ("Missing keys always fall back
// to English, never a blank string"). ADR-013 — runtime i18n.
// JCE keys are merged under the `jce.*` prefix so JCE public pages can reuse
// the same translator.
const dictionaries: Record<string, Dict> = {
	en: { ...en, ...jce_en }
};

// Bundled-but-lazy locales (ADR-013): shipped as separate chunks and
// registered by +layout.ts's ensureLocale() before first render — keeps
// ~370KB of non-English strings out of the eager bundle for the
// English-majority traffic while SSR and hydration see identical dicts.
const localeLoaders: Record<string, () => Promise<Dict>> = {
	fr: async () => {
		const [fr, jce] = await Promise.all([import('./fr.json'), import('./jce_fr.json')]);
		return { ...fr.default, ...jce.default };
	}
};

const pendingLocales = new Map<string, Promise<void>>();

/** Ensure the dictionary for `locale` is registered. Awaited by the root
 * layout's universal load so SSR and hydration always render with the same
 * strings. No-op for `en` and runtime-pack locales (es, de, …). */
export function ensureLocale(locale: string): Promise<void> {
	const loader = localeLoaders[locale];
	if (!loader || dictionaries[locale]) return Promise.resolve();
	let p = pendingLocales.get(locale);
	if (!p) {
		p = loader().then(
			(dict) => {
				dictionaries[locale] = dict;
				delete mergedBundled[locale];
				pendingLocales.delete(locale);
			},
			(err) => {
				// Evict on failure so the next navigation retries — caching a
				// rejected promise would permanently break the locale.
				pendingLocales.delete(locale);
				throw err;
			}
		);
		pendingLocales.set(locale, p);
	}
	return p;
}

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
		const dict = dictFor(this.locale);
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
