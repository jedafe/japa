import en from './en.json';
import fr from './fr.json';

export type Locale = string;

// Static dictionaries for built-in locales. Additional locales resolved at
// runtime fall back to English per Section 18. ADR-013 — runtime i18n.
export const dictionaries: Record<Locale, Record<string, string>> = { en, fr };

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
	const dict = dictionaries[locale] ?? dictionaries.en;
	let raw = dict[key] ?? fallback ?? key;
	if (vars) {
		raw = raw.replace(/\{(\w+)\}/g, (_, k: string) => vars[k] ?? `{${k}}`);
	}
	return substituteSiteName(raw, siteName);
}
