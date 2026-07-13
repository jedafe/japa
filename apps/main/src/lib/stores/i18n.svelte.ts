import { browser } from '$app/environment';
import { getToken, API_BASE } from '$lib/api';

type Locale = string;

// Client-only state - safe for SSR because browser check guards all mutations
let currentLocale: Locale = $state('en');
let translations: Record<string, string> = $state({});
let loaded: Record<string, Record<string, string>> = {};

export function getLocale() {
	return {
		get current() {
			// During SSR, return 'en' as default to avoid hydration mismatch
			return browser ? currentLocale : 'en';
		},
		set: (locale: Locale) => {
			currentLocale = locale;
			if (browser) {
				document.documentElement.lang = locale;
				document.cookie = `jc_locale=${locale};path=/;max-age=31536000;samesite=lax`;
				const token = getToken();
				fetch(`${API_BASE}/me/preferences`, {
					method: 'PUT',
					headers: {
						'Content-Type': 'application/json',
						...(token ? { Authorization: `Bearer ${token}` } : {})
					},
					body: JSON.stringify({ language: locale })
				}).catch(() => {});
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
	// During SSR, translations are populated from server load function
	// On client, they're available immediately after init()
	return translations[key] ?? fallback ?? key;
}

export async function loadTranslations(localeCode: string): Promise<Record<string, string>> {
	if (loaded[localeCode]) {
		translations = loaded[localeCode];
		return loaded[localeCode];
	}
	try {
		const base = API_BASE;
		const res = await fetch(`${base}/i18n/packs/${localeCode}`);
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
