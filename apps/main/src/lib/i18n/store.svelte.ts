import en from './en.json';
import fr from './fr.json';

export type Locale = 'en' | 'fr';
type Dict = Record<string, string>;

const dictionaries: Record<Locale, Dict> = { en, fr };

/**
 * Translator. Flat dotted keys resolved against the active locale's dictionary,
 * falling back to the provided fallback and then the key itself.
 *
 * NOTE: this is a module singleton defaulting to `en`. Locale switching is not
 * wired yet (no LanguageToggle in this increment), so SSR never mutates it. When
 * per-user locale lands, move the active locale into request context to avoid
 * cross-request leakage during SSR.
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

/** Convenience binding: `t('nav.compose')`. */
export const t = i18n.t;
