/**
 * Pure password validation for the change-password flow. `reason` is an i18n
 * key so the caller can surface a localized message.
 *
 * Ref: japaconnect-engineering/sections/02-authentication-sessions.md
 */
export function validatePassword(
	current: string,
	next: string,
	confirm: string
): { ok: boolean; reason: string | null } {
	if (current.length === 0) return { ok: false, reason: 'settings.security.err_current' };
	const strong = next.length >= 8 && /[a-zA-Z]/.test(next) && /[0-9]/.test(next);
	if (!strong) return { ok: false, reason: 'settings.security.err_weak' };
	if (next !== confirm) return { ok: false, reason: 'settings.security.err_mismatch' };
	return { ok: true, reason: null };
}
