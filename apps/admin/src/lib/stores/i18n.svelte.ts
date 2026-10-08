import { browser } from '$app/environment';
import { PUBLIC_JC_API_URL } from '$env/static/public';
import { buildApiUrl } from '@japaconnect/api-url';

type Locale = string;

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
				document.cookie = `jc_locale=${locale};path=/;max-age=31536000;samesite=lax`;
			}
		},
		init: (serverLocale: Locale, serverTranslations?: Record<string, string>) => {
			currentLocale = serverLocale;
			if (serverTranslations) {
				translations = serverTranslations;
				loaded[serverLocale] = serverTranslations;
			}
			if (browser) document.documentElement.lang = serverLocale;
		}
	};
}

export const locale = getLocale();

export function t(key: string, fallback?: string): string {
	return translations[key] ?? fallback ?? key;
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
