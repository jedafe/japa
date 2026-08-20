import en from './en.json';
import fr from './fr.json';

export type Locale = string;

// Static dictionaries for built-in locales. Additional locales resolved at
// runtime fall back to English per Section 18. ADR-013 — runtime i18n.
export const dictionaries: Record<Locale, Record<string, string>> = { en, fr };

/**
 * Translate a key for a specific locale. Used server-side where the reactive
 * `page` store is not available.
 */
export function tServer(
	locale: string,
	key: string,
	fallback?: string,
	vars?: Record<string, string>
): string {
	const dict = dictionaries[locale] ?? dictionaries.en;
	let raw = dict[key] ?? fallback ?? key;
	if (!vars) return raw;
	return raw.replace(/\{(\w+)\}/g, (_, k: string) => vars[k] ?? `{${k}}`);
}
