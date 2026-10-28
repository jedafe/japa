import { PUBLIC_JC_API_URL } from '$env/static/public';
import { buildApiUrl } from '@japaconnect/api-url';
import { DEFAULT_SITE_NAME } from '$lib/brand';

/** Default brand name — re-exported from $lib/brand so server modules can
 * use it without importing this runes-based store. */
export { DEFAULT_SITE_NAME };

// Public site settings (GET /api/v1/site/settings/public). The control SPA has
// no SSR layout data, so the root layout fetches this once on mount.
let settings = $state<Record<string, unknown>>({});
let initStarted = false;

/** Configured brand name (general.site_name). */
export function siteName(): string {
	return (settings['general.site_name'] as string | undefined) || DEFAULT_SITE_NAME;
}

/**
 * Substitute the configured site name into a string: replaces the explicit
 * `{site}` placeholder and any literal brand spelling that slipped into a
 * translation pack/fallback, so a renamed site never leaks the default name.
 */
export function substituteSiteName(value: string): string {
	const name = siteName();
	return value
		.replaceAll('{site}', name)
		.replaceAll('Japa Connect', name)
		.replaceAll('JapaConnect', name);
}

/** Configured favicon URL (appearance.favicon_url) — empty string when unset. */
export function faviconUrl(): string {
	return (settings['appearance.favicon_url'] as string | undefined) || '';
}

/** Theme-specific favicon URL — appearance.favicon_url_dark overrides the
 * base favicon_url in dark mode; light always uses the base key. */
export function faviconForTheme(theme: 'light' | 'dark'): string {
	if (theme === 'dark') {
		return (
			(settings['appearance.favicon_url_dark'] as string | undefined) ||
			(settings['appearance.favicon_url'] as string | undefined) ||
			''
		);
	}
	return (settings['appearance.favicon_url'] as string | undefined) || '';
}

/** Fetch public site settings once. Safe to call from any layout onMount. */
export async function initSiteSettings(): Promise<void> {
	if (initStarted) return;
	initStarted = true;
	try {
		const res = await fetch(buildApiUrl('/site/settings/public', PUBLIC_JC_API_URL), {
			headers: { 'X-JC-Surface': 'control' }
		});
		if (!res.ok) return;
		const body = await res.json();
		settings = body?.data ?? body ?? {};
	} catch {
		// Keep the default name on network failure.
	}
}
