import { browser } from '$app/environment';
import { apiFetch } from '$lib/api/client';
import { substituteSiteName } from './site.svelte';

type Locale = string;

let currentLocale = $state<Locale>('en');
let translationsState = $state<Record<string, string>>({});

const loaded: Record<string, Record<string, string>> = {};

/** Reactive wrapper — use inside $derived.by blocks. Returns current translations map. */
export function translations(): Record<string, string> {
	return translationsState;
}

export function initLocale(serverLocale: Locale, serverTranslations?: Record<string, string>) {
	currentLocale = serverLocale;
	if (serverTranslations) {
		translationsState = serverTranslations;
		loaded[serverLocale] = serverTranslations;
	}
	if (browser) document.documentElement.lang = serverLocale;
}

export async function setLocale(locale: Locale) {
	currentLocale = locale;
	if (browser) {
		document.documentElement.lang = locale;
		document.cookie = `jc_locale=${locale};path=/;max-age=31536000;samesite=lax`;
	}
	await loadTranslations(locale);
}

export function t(key: string, fallback?: string): string {
	// substituteSiteName expands {site} and rewrites any literal brand spelling
	// in packs/fallbacks to the configured general.site_name.
	return substituteSiteName(translationsState[key] ?? fallback ?? key);
}

export async function loadTranslations(localeCode: string): Promise<Record<string, string>> {
	if (loaded[localeCode]) {
		translationsState = loaded[localeCode];
		return loaded[localeCode];
	}
	try {
		const result = await apiFetch<Record<string, string>>(`/api/v1/i18n/packs/${localeCode}`);
		const data = result.data ?? {};
		loaded[localeCode] = data;
		translationsState = data;
		return data;
	} catch {
		return {};
	}
}
