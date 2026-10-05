// Ref: japaconnect-engineering/sections/120-mcp-agent-distribution.md
import { goto } from '$app/navigation';

/**
 * Coarse client-side check used only as a fallback when the server did not
 * return a sanitized redirect. Keeps relative site paths; drops absolute
 * and protocol-relative URLs (the allowlist decision for absolute targets
 * belongs to the server-side sanitizePostLoginTarget).
 */
export function safeRelativePath(target: string | null | undefined): string {
	// "/\evil.com" normalizes to "//evil.com" on navigation — reject backslashes.
	if (target && target.startsWith('/') && !target.startsWith('//') && !target.includes('\\')) {
		return target;
	}
	return '/';
}

/**
 * Post-login navigation helper. The server-side resolvePostLoginRedirect
 * already sanitizes the target (relative site path, or an absolute URL on
 * the API issuer origin — §120 MCP OAuth consent round-trip). This helper
 * picks the right navigation primitive: SvelteKit goto for same-origin
 * paths, window.location.assign for the allowlisted absolute API-host URL
 * (goto only performs SPA navigation).
 *
 * Contract: absolute http(s) targets passed here must already be vetted by
 * the server. Callers that fall back to a raw ?redirect= param must wrap it
 * in safeRelativePath first.
 */
export async function navigatePostLogin(target: string | null | undefined): Promise<void> {
	const href = target && target.length > 0 ? target : '/';
	try {
		const u = new URL(href);
		if (u.protocol === 'https:' || u.protocol === 'http:') {
			window.location.assign(u.toString());
			return;
		}
		// javascript:, data:, etc. — refuse.
		await goto('/');
		return;
	} catch {
		// Not an absolute URL — internal path (or garbage that goto rejects).
	}
	await goto(safeRelativePath(href));
}
