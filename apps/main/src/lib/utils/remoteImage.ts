import { apiBaseUrl } from '$lib/api/client';

/**
 * Routes an external http(s) image through the backend's SSRF-guarded,
 * Valkey-cached same-origin proxy (GET /api/v1/media/proxy?url=…) so remote
 * hosts that block cross-origin embeds via CORP/ORB still render, and fetch
 * failures return the backend placeholder instead of a broken image.
 * Same-origin/relative URLs pass through unchanged.
 *
 * Ref: conventions/api-routes-catalog.md (GET /media/proxy)
 */
export function remoteImageUrl(url: string | null | undefined): string {
	if (!url) return '';
	if (/^\/api\/v1\//.test(url)) return `${apiBaseUrl}${url.slice('/api/v1'.length)}`;
	if (!/^https?:\/\//i.test(url)) return url;
	return `${apiBaseUrl}/media/proxy?url=${encodeURIComponent(url)}`;
}
