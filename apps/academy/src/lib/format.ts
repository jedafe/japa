import { t } from '$lib/i18n';
import { currencyOf, type CurrencyInfo } from '$lib/utils/currency';

/**
 * Academy-specific formatting helpers.
 * Ref: ADR-063 (price_kobo marketplace), ADR-060 (duration display).
 */

/**
 * Format a kobo (minor-unit) amount as a localized price string.
 * Currency and locale are driven by the track/caller, not hardcoded NGN/en-NG.
 * 0 or null → "Free". Ref: ADR-063, Rule 16.
 * `currencies` is the managed currency table (§58) — when provided, its
 * symbol replaces the ISO symbol in Intl's localized output.
 */
export function formatPriceKobo(
	kobo: number | null | undefined,
	currency?: string | null,
	locale?: string | null,
	currencies?: CurrencyInfo[] | null
): string {
	if (!kobo || kobo <= 0) return t('academy.free', 'Free');
	const units = kobo / 100;
	const code = (currency ?? '').trim() || undefined;
	const loc = locale ?? undefined;
	if (!code) {
		// No currency code known — show the raw major-unit amount without guessing.
		return units.toLocaleString(loc);
	}
	// Only substitute the MANAGED symbol — falling back to Intl's narrowSymbol
	// here would degrade disambiguating defaults (e.g. "CA$" → "$").
	const managedSymbol = currencyOf(code, currencies)?.symbol;
	try {
		const parts = new Intl.NumberFormat(loc, {
			style: 'currency',
			currency: code,
			maximumFractionDigits: 0
		}).formatToParts(units);
		if (managedSymbol) {
			return parts.map((p) => (p.type === 'currency' ? managedSymbol : p.value)).join('');
		}
		return parts.map((p) => p.value).join('');
	} catch {
		// Code isn't a valid ISO 4217 code (e.g. a platform-specific currency).
		if (managedSymbol) return `${managedSymbol}${units.toLocaleString(loc)}`;
		return `${units.toLocaleString(loc)} ${code}`;
	}
}

/** True when the course is free (kobo is 0/null) or the user is Pro. */
export function isFreeForUser(
	kobo: number | null | undefined,
	isPro: boolean | undefined
): boolean {
	return isPro || !kobo || kobo <= 0;
}

/** Format an ISO 8601 duration ("PT4H30M") as "4h 30m". */
export function formatDuration(iso: string | null | undefined): string {
	if (!iso) return '';
	const m = /^PT(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?$/.exec(iso);
	if (!m) return '';
	const [, h, min, s] = m;
	const hours = h ? parseInt(h, 10) : 0;
	const minutes = min ? parseInt(min, 10) : 0;
	const seconds = s ? parseInt(s, 10) : 0;
	if (hours && minutes) return `${hours}h ${minutes}m`;
	if (hours) return `${hours}h`;
	if (minutes) return `${minutes}m`;
	if (seconds) return `${seconds}s`;
	return '';
}

/** Map a track level to its CSS custom property (never hardcode hex). */
export function levelColorVar(level: string): string {
	switch (level) {
		case 'beginner':
			return 'var(--level-beginner)';
		case 'intermediate':
			return 'var(--level-intermediate)';
		case 'advanced':
			return 'var(--level-advanced)';
		default:
			return 'var(--level-beginner)';
	}
}

/** Format an enrolled count compactly: 1200 → "1.2k". */
export function formatCount(n: number | null | undefined): string {
	if (!n || n <= 0) return '0';
	if (n < 1000) return String(n);
	if (n < 1_000_000) return `${(n / 1000).toFixed(1).replace(/\.0$/, '')}k`;
	return `${(n / 1_000_000).toFixed(1).replace(/\.0$/, '')}M`;
}
