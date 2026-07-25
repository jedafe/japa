/**
 * Convert a `<input type="date">` value (`YYYY-MM-DD`) into an RFC 3339
 * timestamp at the start of the day in UTC.
 */
export function dateInputToStartOfDayISO(date: string): string {
	if (!date) return '';
	return `${date}T00:00:00Z`;
}

/**
 * Convert a `<input type="date">` value into an RFC 3339 timestamp at the
 * end of the day in UTC (inclusive).
 */
export function dateInputToEndOfDayISO(date: string): string {
	if (!date) return '';
	return `${date}T23:59:59Z`;
}

/**
 * Convert an ISO / RFC 3339 timestamp into a value suitable for
 * `<input type="date">` (`YYYY-MM-DD`). Returns an empty string for missing
 * or invalid input.
 */
export function isoTimestampToDateInput(iso: string | null | undefined): string {
	if (!iso) return '';
	if (/^\d{4}-\d{2}-\d{2}$/.test(iso)) return iso;
	const d = new Date(iso);
	if (isNaN(d.getTime())) return '';
	// Shift to UTC so the displayed date matches the ISO instant.
	return new Date(d.getTime() + d.getTimezoneOffset() * 60_000)
		.toISOString()
		.slice(0, 10);
}

/**
 * Convert a `<input type="datetime-local">` value into an RFC 3339
 * UTC timestamp. Interprets the input as local browser time.
 */
export function datetimeLocalToISOString(local: string): string {
	if (!local) return '';
	const d = new Date(local);
	if (isNaN(d.getTime())) return '';
	return d.toISOString();
}

function pad(n: number): string {
	return String(n).padStart(2, '0');
}

/**
 * Convert a `<input type="datetime-local">` value into a PostgreSQL
 * `timestamptz` text string (`YYYY-MM-DD HH:MM:SSZ`) that pgx can scan.
 * Interprets the input as local browser time.
 */
export function datetimeLocalToPostgresTimestamptz(local: string): string {
	if (!local) return '';
	const [datePart, timePart] = local.split('T');
	if (!datePart || !timePart) return '';
	const [year, month, day] = datePart.split('-').map(Number);
	const [hours, minutes] = timePart.split(':').map(Number);
	if ([year, month, day, hours, minutes].some((n) => isNaN(n))) return '';

	const d = new Date(year, month - 1, day, hours, minutes, 0);
	if (isNaN(d.getTime())) return '';

	return `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())} ${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())}:${pad(d.getUTCSeconds())}Z`;
}
