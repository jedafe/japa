import { browser } from '$app/environment';
import en from './en.json';
import fr from './fr.json';

export type Locale = 'en' | 'fr';
type Dict = Record<string, string>;

const dictionaries: Record<Locale, Dict> = { en, fr };

/**
 * Runes-based i18n store for the Tools surface. Mirrors the Main app's
 * pattern (ADR-013 runtime i18n). The active locale is seeded from the
 * SSR-resolved `event.locals.locale` (jc_locale cookie → i18n.default_language
 * setting) via `i18n.setLocale()` in +layout.svelte.
 *
 * Ref: §7 (i18n), §18 (language packs), ADR-013 (runtime i18n).
 */
class I18nStore {
	locale = $state<Locale>('en');

	setLocale(locale: Locale) {
		if (locale in dictionaries) this.locale = locale;
	}

	t = (key: string, fallback?: string): string => {
		return dictionaries[this.locale][key] ?? fallback ?? key;
	};
}

export const i18n = new I18nStore();

/** Convenience binding: `t('tools.hub.title')`. */
export const t = i18n.t;
