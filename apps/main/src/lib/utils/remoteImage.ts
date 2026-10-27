import { apiBaseUrl } from '$lib/api/client';

// First-party image hosts — the API host itself plus every sibling host on
// its registrable domain (api.japa.to → img.japa.to, cdn.japa.to, …). Our own
// media hosts never block cross-origin embeds, and routing them through
// /media/proxy breaks inside the backend network: img.* resolves to a
// non-public IP there, so the proxy's SSRF guard rejects the fetch and the
// placeholder SVG renders instead of the image.
const apiHost = (() => {
	try {
		return apiBaseUrl.startsWith('http') ? new URL(apiBaseUrl).hostname : '';
	} catch {
		return '';
	}
})();
const firstPartySuffix = apiHost.includes('.') ? apiHost.slice(apiHost.indexOf('.')) : '';

function isFirstParty(url: string): boolean {
	if (!firstPartySuffix) return false;
	try {
		const host = new URL(url).hostname;
		return host === apiHost || host.endsWith(firstPartySuffix);
	} catch {
		return false;
	}
}

/**
 * Routes an external http(s) image through the backend's SSRF-guarded,
 * Valkey-cached same-origin proxy (GET /api/v1/media/proxy?url=…) so remote
 * hosts that block cross-origin embeds via CORP/ORB still render, and fetch
 * failures return the backend placeholder instead of a broken image.
 * Same-origin/relative URLs and first-party media hosts pass through
 * unchanged.
 *
 * Ref: conventions/api-routes-catalog.md (GET /media/proxy)
 */
export function remoteImageUrl(url: string | null | undefined): string {
	if (!url) return '';
	if (/^\/api\/v1\//.test(url)) return `${apiBaseUrl}${url.slice('/api/v1'.length)}`;
	if (!/^https?:\/\//i.test(url)) return url;
	if (isFirstParty(url)) return url;
	return `${apiBaseUrl}/media/proxy?url=${encodeURIComponent(url)}`;
}
