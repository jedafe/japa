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

/**
 * Inverse of localDateTimeToIso — convert an ISO 8601 instant (e.g. an
 * API timestamptz like "2026-10-15T11:00:00+01:00") to the
 * `datetime-local` input format "YYYY-MM-DDTHH:mm" in the browser's
 * local timezone. Unparseable values return null so the input clears
 * instead of tripping the browser's format validation.
 */
export function isoToLocalDateTime(value: string | null | undefined): string | null {
	if (!value) return null;
	const d = new Date(value);
	if (isNaN(d.getTime())) return null;
	const pad = (n: number) => String(n).padStart(2, '0');
	return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}
