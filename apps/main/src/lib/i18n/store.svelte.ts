import { page } from '$app/state';
import en from './en.json';
import fr from './fr.json';

export type Locale = string;
type Dict = Record<string, string>;

// Static dictionaries for built-in locales. Additional locales resolved at
// runtime fall back to English per Section 18 ("Missing keys always fall back
// to English, never a blank string"). ADR-013 — runtime i18n.
const dictionaries: Record<string, Dict> = { en, fr };

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
 */
class I18nStore {
	/** Active locale, derived from the request-scoped page data. */
	locale = $derived<Locale>(
		typeof page.data?.locale === 'string' && page.data.locale ? page.data.locale : 'en'
	);

	t = (key: string, fallback?: string): string => {
		const dict = dictionaries[this.locale] ?? dictionaries.en;
		return dict[key] ?? fallback ?? key;
	};
}

export const i18n = new I18nStore();

/** Convenience binding: `t('nav.compose')`. */
export const t = i18n.t;
