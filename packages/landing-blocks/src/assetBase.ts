// Root-relative asset paths (e.g. /brand/…, /logos/…) baked into block
// defaults resolve against the host serving the page. Surfaces that render
// a preview of another app's pages — the admin page-builder previewing the
// main site, for example — call setAssetBaseUrl() once so those assets
// resolve on the site that owns them. On the owning host itself nothing is
// set and every URL passes through unchanged.

let assetBaseUrl = '';

// Root-relative paths that look like static assets (image/video extensions),
// excluding API and imgproxy paths which the caller's origin already proxies.
const REL_ASSET =
  /^\/(?!api\/|imgproxy\/)[^\s"']+\.(?:jpe?g|png|webp|gif|svg|avif|mp4|webm)$/i;

/** Set the origin used to resolve root-relative asset URLs. */
export function setAssetBaseUrl(url: string | undefined | null): void {
  assetBaseUrl = (url ?? '').replace(/\/$/, '');
}

/** Resolve a possibly root-relative asset URL against the configured base. */
export function resolveAssetUrl(url: string | undefined | null): string {
  if (!url || !assetBaseUrl) return url ?? '';
  return REL_ASSET.test(url) ? assetBaseUrl + url : url;
}
