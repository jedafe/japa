/**
 * Exact-match confirmation for the delete-account dialog. Trimmed,
 * case-sensitive — the user must type their exact username to confirm.
 */
export function matchesConfirmation(typed: string, expected: string): boolean {
	return typed.trim() === expected && expected.length > 0;
}
