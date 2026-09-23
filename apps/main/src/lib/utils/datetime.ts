/**
 * Convert a `datetime-local` input value ("YYYY-MM-DDTHH:mm") to a UTC
 * ISO 8601 instant. The bare input string is browser-local wall time —
 * it must be resolved to an instant here, where the user's timezone is
 * known, because the API stores publish_at as a timestamptz.
 * Values already carrying a zone/seconds pass through unchanged.
 */
export function localDateTimeToIso(value: string | null | undefined): string | null {
	if (!value) return null;
	if (/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(value)) {
		const d = new Date(value);
		return isNaN(d.getTime()) ? value : d.toISOString();
	}
	return value;
}
