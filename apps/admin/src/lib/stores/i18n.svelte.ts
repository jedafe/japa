import { browser } from '$app/environment';
import { PUBLIC_JC_API_URL } from '$env/static/public';
import { buildApiUrl } from '@japaconnect/api-url';
import { substituteSiteName } from './site.svelte';

type Locale = string;

const RTL_LOCALES = new Set(['ar', 'he', 'fa', 'ur']);
const isRtl = (locale: Locale): boolean => RTL_LOCALES.has(locale);

let currentLocale: Locale = $state('en');
let translations: Record<string, string> = $state({});
let loaded: Record<string, Record<string, string>> = {};

export function getLocale() {
	return {
		get current() {
			return currentLocale;
		},
		set: (locale: Locale) => {
			currentLocale = locale;
			if (browser) {
				document.documentElement.lang = locale;
				document.documentElement.dir = isRtl(locale) ? 'rtl' : 'ltr';
				document.cookie = `jc_locale=${locale};path=/;max-age=31536000;samesite=lax`;
			}
		},
		init: (serverLocale: Locale, serverTranslations?: Record<string, string>) => {
			currentLocale = serverLocale;
			if (serverTranslations) {
				translations = serverTranslations;
				loaded[serverLocale] = serverTranslations;
			}
			if (browser) {
				document.documentElement.lang = serverLocale;
				document.documentElement.dir = isRtl(serverLocale) ? 'rtl' : 'ltr';
			}
		}
	};
}

export const locale = getLocale();

export function t(key: string, fallback?: string): string {
	// substituteSiteName expands {site} and rewrites any literal brand spelling
	// in packs/fallbacks to the configured general.site_name.
	return substituteSiteName(translations[key] ?? fallback ?? key);
}

export async function loadTranslations(localeCode: string): Promise<Record<string, string>> {
	if (loaded[localeCode]) {
		translations = loaded[localeCode];
		return loaded[localeCode];
	}
	try {
		const res = await fetch(buildApiUrl(`/i18n/packs/${localeCode}`, PUBLIC_JC_API_URL), {
			headers: { 'X-JC-Surface': 'admin' }
		});
		if (!res.ok) throw new Error(`HTTP ${res.status}`);
		const body = await res.json();
		const data = body?.data ?? body ?? {};
		loaded[localeCode] = data;
		translations = data;
		return data;
	} catch {
		return {};
	}
}
