import { page } from '$app/state';
import en from './en.json';
import fr from './fr.json';

export type Locale = string;
type Dict = Record<string, string>;

// Static dictionaries for built-in locales. Additional locales resolved at
// runtime fall back to English per Section 18. ADR-013 — runtime i18n.
const dictionaries: Record<string, Dict> = { en, fr };

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
	t = (key: string, fallback?: string, vars?: Record<string, string>): string => {
		const dict = dictionaries[this.locale] ?? dictionaries.en;
		const raw = dict[key] ?? fallback ?? key;
		if (!vars) return raw;
		return raw.replace(/\{(\w+)\}/g, (_, k: string) => vars[k] ?? `{${k}}`);
	};
}

export const i18n = new I18nStore();

/** Convenience binding: `t('nav.search_placeholder')`. */
export const t = i18n.t;
