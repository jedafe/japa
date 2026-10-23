import { i18n } from '$lib/i18n';

/**
 * Number formatting for analytics chart labels. Rounds to the nearest
 * integer and adds locale-aware thousands separators.
 */
export function formatMetric(n: number): string {
	return Math.round(n).toLocaleString(i18n.locale);
}
