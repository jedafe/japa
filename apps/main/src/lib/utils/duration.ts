/**
 * Format a duration in seconds as `m:ss` or `h:mm:ss` (§106 video cards).
 * Returns '' for missing/non-positive values so callers can gate on it.
 */
export function formatDuration(secs: number | null | undefined): string {
	if (!secs || secs <= 0) return '';
	const total = Math.floor(secs);
	const h = Math.floor(total / 3600);
	const m = Math.floor((total % 3600) / 60);
	const s = total % 60;
	return h > 0
		? `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
		: `${m}:${String(s).padStart(2, '0')}`;
}
