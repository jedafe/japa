import { writable, get } from 'svelte/store';
import { browser } from '$app/environment';
import { apiFetch } from '$lib/api/client';

type Locale = string;

export const currentLocale = writable<Locale>('en');
export const translations = writable<Record<string, string>>({});

const loaded: Record<string, Record<string, string>> = {};

export function initLocale(serverLocale: Locale, serverTranslations?: Record<string, string>) {
	currentLocale.set(serverLocale);
	if (serverTranslations) {
		translations.set(serverTranslations);
		loaded[serverLocale] = serverTranslations;
	}
	if (browser) document.documentElement.lang = serverLocale;
}

export function setLocale(locale: Locale) {
	currentLocale.set(locale);
	if (browser) {
		document.documentElement.lang = locale;
		document.cookie = `jc_locale=${locale};path=/;max-age=31536000;samesite=lax`;
	}
}

export function t(key: string, fallback?: string, tr?: Record<string, string>): string {
	const store = tr ?? get(translations);
	return store[key] ?? fallback ?? key;
}

export async function loadTranslations(localeCode: string): Promise<Record<string, string>> {
	if (loaded[localeCode]) {
		translations.set(loaded[localeCode]);
		return loaded[localeCode];
	}
	try {
		const result = await apiFetch<Record<string, string>>(`/api/v1/i18n/packs/${localeCode}`);
		const data = result.data ?? {};
		loaded[localeCode] = data;
		translations.set(data);
		return data;
	} catch {
		return {};
	}
}
