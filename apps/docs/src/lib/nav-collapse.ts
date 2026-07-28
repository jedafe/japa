/**
 * Client helper to toggle + persist the left-nav collapsed state via
 * POST /api/nav-collapse (ADR-058 — server-persistence, never localStorage).
 * Returns the new collapsed value; callers update their local state.
 */
export async function persistNavCollapse(collapsed: boolean): Promise<boolean> {
	try {
		await fetch('/api/nav-collapse', {
			method: 'POST',
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({ collapsed })
		});
	} catch {
		// Persistence is best-effort; the local toggle still applies for this session.
	}
	return collapsed;
}
