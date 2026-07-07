/**
 * Frontend imgproxy URL builder.
 *
 * The Go backend already signs imgproxy URLs server-side for API responses.
 * This helper lets the client generate transformation-only URLs for images
 * served from the same base (e.g. presigned SeaweedFS or CDN).
 *
 * Usage:
 *   import { imgproxy } from '$lib/utils/imgproxy';
 *   <img src={imgproxy(url, { width: 400, format: 'webp' })} />
 */

export interface ImgproxyOptions {
	/** Resize to this width (px). */
	width?: number;
	/** Resize to this height (px). */
	height?: number;
	/** Output format: webp, avif, png, jpg, etc. */
	format?: 'webp' | 'avif' | 'png' | 'jpg' | 'jpeg' | 'gif' | 'svg';
	/** Resize algorithm: 'fit' (default), 'fill', 'auto'. */
	resize?: 'fit' | 'fill' | 'auto';
	/** JPEG/WebP quality 1-100. */
	quality?: number;
	/** Add loading="lazy" attribute. */
	lazy?: boolean;
}

/**
 * Build an imgproxy transformation URL from a source URL.
 *
 * For server-side signed URLs (from the API), the backend already includes
 * the signature. This function appends plain transformation parameters
 * that imgproxy can process without a signature (for public images served
 * through a proxy that handles signing).
 *
 * If the source URL is already an imgproxy URL (contains /insecure/ or a
 * signature path), this function returns it unchanged.
 */
export function imgproxy(url: string | null | undefined, opts: ImgproxyOptions = {}): string {
	if (!url) return '';

	// Don't double-proxy imgproxy URLs
	if (url.includes('/insecure/') || /\/[a-f0-9]{32}\//.test(url)) return url;

	const { width, height, format, resize = 'fit', quality } = opts;

	const parts: string[] = [];
	if (resize) parts.push(`rs:${resize}`);
	if (width) parts.push(`w:${width}`);
	if (height) parts.push(`h:${height}`);
	if (format) parts.push(`f:${format}`);
	if (quality) parts.push(`q:${quality}`);

	if (parts.length === 0) return url;

	const transformation = parts.join('/');
	return `/imgproxy/${transformation}/plain/${encodeURIComponent(url)}`;
}

/**
 * Generate srcset string for responsive images.
 */
export function imgproxySrcset(
	url: string | null | undefined,
	sizes: number[] = [320, 640, 960, 1280],
	format: ImgproxyOptions['format'] = 'webp',
): string {
	if (!url) return '';
	return sizes
		.map((w) => `${imgproxy(url, { width: w, format })} ${w}w`)
		.join(', ');
}
