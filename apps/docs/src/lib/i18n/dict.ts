import en from './en.json';
import fr from './fr.json';

export type Locale = string;

// Static dictionaries for built-in locales. Additional locales resolved at
// runtime fall back to English per Section 18. ADR-013 — runtime i18n.
export const dictionaries: Record<Locale, Record<string, string>> = { en, fr };

// Runtime packs for locales without a bundled dictionary, layered over
// English at registration so lookups stay O(1). The root layout load fetches
// /i18n/packs/{locale} and registers it here; pack contents are public
// per-locale data, so a process-level map is safe to share across requests
// (unlike the request-scoped locale itself).
const runtimePacks: Record<Locale, Record<string, string>> = {};

export function setRuntimePack(locale: Locale, pack: Record<string, string>): void {
	runtimePacks[locale] = { ...en, ...pack };
}

// §18 — every non-en locale is layered over English so a missing key resolves
// to real English text instead of the raw key.
const mergedBundled: Record<Locale, Record<string, string>> = {};

function dictFor(locale: Locale): Record<string, string> {
	if (locale === 'en') return dictionaries.en;
	const bundled = dictionaries[locale];
	if (bundled) {
		return (mergedBundled[locale] ??= { ...dictionaries.en, ...bundled });
	}
	return runtimePacks[locale] ?? dictionaries.en;
}

/** Default brand name when general.site_name is unset — the only place the
 * literal may appear; all UI copy must flow through t()/tServer {site}. */
export const DEFAULT_SITE_NAME = 'Japa.to';

/**
 * Substitute the configured site name into a translated string: replaces the
 * explicit `{site}` placeholder and any literal brand spelling that slipped
 * into a dictionary/fallback, so a renamed site never leaks the default name.
 */
export function substituteSiteName(value: string, name: string): string {
	return value
		.replaceAll('{site}', name)
		.replaceAll('Japa Connect', name)
		.replaceAll('Japa.to', name);
}

/**
 * Translate a key for a specific locale. Used server-side where the reactive
 * `page` store is not available. `siteName` should come from
 * `event.locals.siteSettings['general.site_name']`.
 */
export function tServer(
	locale: string,
	key: string,
	fallback?: string,
	vars?: Record<string, string>,
	siteName: string = DEFAULT_SITE_NAME
): string {
	const dict = dictFor(locale);
	let raw = dict[key] ?? fallback ?? key;
	if (vars) {
		raw = raw.replace(/\{(\w+)\}/g, (_, k: string) => vars[k] ?? `{${k}}`);
	}
	return substituteSiteName(raw, siteName);
}
