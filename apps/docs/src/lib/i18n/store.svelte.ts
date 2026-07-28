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
 * wired to mutate it during SSR (the layout syncs it after hydration via
 * i18n.setLocale(data.locale)) to avoid cross-request leakage.
 */
class I18nStore {
	locale = $state<Locale>('en');

	setLocale(locale: Locale) {
		if (locale in dictionaries) this.locale = locale;
	}

	/** Interpolate {name} placeholders in the fallback string. */
	t = (key: string, fallback?: string, vars?: Record<string, string>): string => {
		const raw = dictionaries[this.locale][key] ?? fallback ?? key;
		if (!vars) return raw;
		return raw.replace(/\{(\w+)\}/g, (_, k: string) => vars[k] ?? `{${k}}`);
	};
}

export const i18n = new I18nStore();

/** Convenience binding: `t('nav.search_placeholder')`. */
export const t = i18n.t;
