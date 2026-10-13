import { goto } from '$app/navigation';
import { page } from '$app/state';

/**
 * Locale helpers shared by the header language menu and the sidebar
 * language_toggle widget. Reads are non-reactive helpers — call them inside
 * $derived/$effect or event handlers.
 * Ref: §18 (i18n language packs), ADR-013 (runtime i18n).
 */

/** Enabled UI languages from site settings; falls back to the active locale. */
export function enabledLanguages(): string[] {
	return (
		((page.data?.siteSettings?.['i18n.enabled_languages'] as string | undefined) ??
			(page.data?.locale as string) ??
			'en')
			.split(',')
			.map((l) => l.trim())
			.filter(Boolean)
	);
}

export function defaultLanguage(): string {
	return (page.data?.siteSettings?.['i18n.default_language'] as string | undefined) ?? 'en';
}

let displayNames: Intl.DisplayNames | null | undefined;
export function localeLabel(code: string): string {
	if (displayNames === undefined) {
		try {
			displayNames = new Intl.DisplayNames(undefined, { type: 'language' });
		} catch {
			displayNames = null;
		}
	}
	return displayNames?.of(code) ?? code;
}

/**
 * Locale-prefixed href for a target language. Strips an existing locale
 * prefix first so switching from /fr/docs to English yields /docs, not /fr/fr.
 */
export function localeHref(lang: string): string {
	let path = page.url.pathname.replace(/^\//, '');
	const firstSeg = path.split('/')[0];
	if (enabledLanguages().includes(firstSeg)) {
		path = path.slice(firstSeg.length).replace(/^\//, '');
	}
	return lang === defaultLanguage() ? `/${path}` : `/${lang}${path ? `/${path}` : ''}`;
}

/**
 * Switch locale: persist jc_locale AND navigate to the locale-prefixed URL.
 * The cookie write is required — hooks fall back to it when the URL has no
 * prefix. invalidateAll forces a load re-run for same-URL navigations
 * (switching back to the default language strips the prefix).
 */
export function switchLanguage(lang: string) {
	document.cookie = `jc_locale=${lang}; path=/; max-age=${60 * 60 * 24 * 365}; samesite=lax`;
	goto(localeHref(lang), { invalidateAll: true });
}
