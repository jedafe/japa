/**
 * Parse a timestamp that may be ISO or the backend's non-ISO form,
 * e.g. "2026-07-12 08:03:55.888616 +0100 WAT". Returns null when unparseable.
 */
function parseTimestamp(raw: string): Date | null {
	if (!raw) return null;

	const iso = new Date(raw);
	if (!Number.isNaN(iso.getTime())) return iso;

	const m = raw.match(/^(\d{4}-\d{2}-\d{2})[ T](\d{2}:\d{2}:\d{2})(\.\d+)?\s*([+-]\d{2})(\d{2})/);
	if (!m) return null;

	const [, date, time, frac = '', offH, offM] = m;
	const built = new Date(`${date}T${time}${frac}${offH}:${offM}`);
	return Number.isNaN(built.getTime()) ? null : built;
}

/** Compact relative time ("now", "5m", "2h", "3d", "2w") or a short date; '' if unparseable. */
export function relativeTime(raw: string, now: Date = new Date()): string {
	const d = parseTimestamp(raw);
	if (!d) return '';

	const secs = Math.floor((now.getTime() - d.getTime()) / 1000);
	if (secs < 45) return 'now';
	const mins = Math.floor(secs / 60);
	if (mins < 60) return `${mins}m`;
	const hrs = Math.floor(mins / 60);
	if (hrs < 24) return `${hrs}h`;
	const days = Math.floor(hrs / 24);
	if (days < 7) return `${days}d`;
	const weeks = Math.floor(days / 7);
	if (weeks < 5) return `${weeks}w`;
	return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}
