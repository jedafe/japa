/**
 * Resolve the default scholarship reminder window from public site settings.
 * Falls back to 14 days when the setting is missing or invalid.
 * Ref: conventions/system-settings-catalog.md (scholarship_finder.default_reminder_days).
 */
export function defaultReminderDays(siteSettings: Record<string, unknown> | undefined): number {
	const raw = siteSettings?.['scholarship_finder.default_reminder_days'];
	if (typeof raw === 'number') return Number.isFinite(raw) && raw >= 0 ? raw : 14;
	if (typeof raw === 'string') {
		const n = parseInt(raw, 10);
		if (!Number.isNaN(n) && n >= 0) return n;
	}
	return 14;
}
