/**
 * Timezone-aware date formatting utilities.
 * Uses the browser's resolved timezone by default.
 */

export function formatTimestamp(date: string | Date, timezone?: string): string {
	const d = typeof date === 'string' ? new Date(date) : date;
	const tz = timezone || Intl.DateTimeFormat().resolvedOptions().timeZone;
	return d.toLocaleDateString('en-US', {
		month: 'short',
		day: 'numeric',
		year: 'numeric',
		timeZone: tz
	});
}

export function formatTimestampLong(date: string | Date, timezone?: string): string {
	const d = typeof date === 'string' ? new Date(date) : date;
	const tz = timezone || Intl.DateTimeFormat().resolvedOptions().timeZone;
	return d.toLocaleDateString('en-US', {
		month: 'long',
		day: 'numeric',
		year: 'numeric',
		timeZone: tz
	});
}

export function formatTimestampWithTime(date: string | Date, timezone?: string): string {
	const d = typeof date === 'string' ? new Date(date) : date;
	const tz = timezone || Intl.DateTimeFormat().resolvedOptions().timeZone;
	return d.toLocaleDateString('en-US', {
		year: 'numeric',
		month: 'short',
		day: 'numeric',
		hour: '2-digit',
		minute: '2-digit',
		timeZone: tz
	});
}

export function formatRelativeTime(date: string | Date, timezone?: string): string {
	const d = typeof date === 'string' ? new Date(date) : date;
	const now = new Date();
	const diff = now.getTime() - d.getTime();
	const minutes = Math.floor(diff / 60000);
	if (minutes < 1) return 'just now';
	if (minutes < 60) return `${minutes}m ago`;
	const hours = Math.floor(minutes / 60);
	if (hours < 24) return `${hours}h ago`;
	const days = Math.floor(hours / 24);
	if (days < 7) return `${days}d ago`;
	return formatTimestamp(date, timezone);
}
