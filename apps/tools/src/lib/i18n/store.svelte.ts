import { page } from '$app/state';
import en from './en.json';
import fr from './fr.json';

export type Locale = 'en' | 'fr';
type Dict = Record<string, string>;

const dictionaries: Record<Locale, Dict> = { en, fr };

/**
 * Runes-based i18n store for the Tools surface. Mirrors the Main app's
 * pattern (ADR-013 runtime i18n). The active locale is derived from the
 * SvelteKit `page` store (`page.data.locale`), which is request-scoped during
 * SSR and reactive on the client — eliminating the cross-request leakage risk
 * of a mutable module singleton. Client-side locale switches propagate
 * reactively via the `jc_locale` cookie + page reload.
 *
 * Ref: §7 (i18n), §18 (language packs), ADR-013 (runtime i18n).
 */
class I18nStore {
	/** Active locale, derived from the request-scoped page data. */
	locale = $derived<Locale>(
		(page.data?.locale === 'fr' || page.data?.locale === 'en') ? page.data.locale : 'en'
	);

	t = (key: string, fallback?: string): string => {
		return dictionaries[this.locale][key] ?? fallback ?? key;
	};
}

export const i18n = new I18nStore();

/** Convenience binding: `t('tools.hub.title')`. */
export const t = i18n.t;
