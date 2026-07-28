/**
 * Learn-specific formatting helpers.
 * Ref: ADR-063 (price_kobo marketplace), ADR-060 (duration display).
 */

/** Format a kobo amount as a NGN price string. 0 or null → "Free". */
export function formatPriceKobo(kobo: number | null | undefined): string {
	if (!kobo || kobo <= 0) return 'Free';
	// 100 kobo = 1 naira. Use Intl.NumberFormat for grouping.
	const naira = kobo / 100;
	return new Intl.NumberFormat('en-NG', {
		style: 'currency',
		currency: 'NGN',
		maximumFractionDigits: 0
	}).format(naira);
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

/** Map a TrackLevel to its CSS custom property (never hardcode hex). */
export function levelColorVar(level: 'beginner' | 'intermediate' | 'advanced'): string {
	switch (level) {
		case 'beginner':
			return 'var(--level-beginner)';
		case 'intermediate':
			return 'var(--level-intermediate)';
		case 'advanced':
			return 'var(--level-advanced)';
	}
}

/** Format an enrolled count compactly: 1200 → "1.2k". */
export function formatCount(n: number | null | undefined): string {
	if (!n || n <= 0) return '0';
	if (n < 1000) return String(n);
	if (n < 1_000_000) return `${(n / 1000).toFixed(1).replace(/\.0$/, '')}k`;
	return `${(n / 1_000_000).toFixed(1).replace(/\.0$/, '')}M`;
}
