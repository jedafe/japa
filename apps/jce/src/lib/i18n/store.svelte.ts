import { page } from '$app/state';
import en from './en.json';
import fr from './fr.json';
import type { Locale } from '$lib/types';

type Dict = Record<string, string>;

// Static dictionaries for built-in locales. Additional locales resolved at
// runtime fall back to English for missing keys.
const dictionaries: Record<string, Dict> = {
	en: en as Record<string, string>,
	fr: fr as Record<string, string>
};

/**
 * Runes-based translator. Flat dotted keys are resolved against the active
 * locale's dictionary, falling back to English so missing keys are never blank.
 *
 * The active locale is derived from the SvelteKit `page` state
 * (`page.data.locale`), which is request-scoped during SSR and reactive on the
 * client. This matches the pattern in apps/main/src/lib/i18n/store.svelte.ts.
 */
class I18nStore {
	/** Active locale, derived from the request-scoped page data. */
	locale = $derived<Locale>(
		typeof page.data?.locale === 'string' && page.data.locale ? page.data.locale : 'en'
	);

	t = (key: string, fallback?: string): string => {
		const active = dictionaries[this.locale] ?? {};
		// Active locale overrides English; missing keys always fall back to English.
		return active[key] ?? dictionaries.en[key] ?? fallback ?? key;
	};
}

export const i18n = new I18nStore();

/** Convenience binding: `t('jce.compare.heading')`. */
export const t = i18n.t;
