/**
 * Client-side image conversion to WebP.
 *
 * Uses OffscreenCanvas + createImageBitmap for zero-main-thread conversion.
 * Downscales to MAX_IMAGE_DIMENSION and re-encodes at WEBP_QUALITY_DEFAULT.
 * Falls back to the original file on any error or unsupported browser.
 */

const WEBP_QUALITY_DEFAULT = 0.85;

/** Cap stored image dimensions — multi-megapixel uploads get downscaled. */
const MAX_IMAGE_DIMENSION = 2048;

const IMAGE_MIME_PREFIX = 'image/';

/**
 * MIME types we never convert: already WebP, or formats where canvas
 * rasterization would silently destroy data (GIF animation, SVG vectors).
 * AVIF inputs ARE converted — the backend does not accept image/avif
 * uploads, and AVIF decode is universally supported in browsers.
 * HEIC/HEIF inputs ARE converted — via canvas on Safari, or libheif
 * WASM elsewhere — since iPhone photos commonly arrive in that format.
 */
const SKIP_MIMES = new Set(['image/webp', 'image/gif', 'image/svg+xml']);

const HEIC_MIMES = new Set([
	'image/heic',
	'image/heif',
	'image/heic-sequence',
	'image/heif-sequence'
]);
const HEIC_EXTENSIONS = /\.(heic|heif)$/i;

/**
 * HEIC/HEIF detected by MIME type or, when the platform reports an empty
 * or generic type, by file extension.
 */
function isHeicLike(file: File): boolean {
	return HEIC_MIMES.has(file.type) || HEIC_EXTENSIONS.test(file.name);
}

/** Replace the file extension to match the WebP output. */
function renameExtension(file: File): string {
	const name = file.name;
	const lastDot = name.lastIndexOf('.');
	const base = lastDot > 0 ? name.slice(0, lastDot) : name;
	return `${base}.webp`;
}

/**
 * Rasterize a file and re-encode it as WebP at the capped dimension.
 * Returns null when the browser cannot decode the source or the canvas
 * has no WebP encoder, so callers can apply their own fallback.
 */
async function canvasToWebP(file: File, quality?: number): Promise<File | null> {
	try {
		const bitmap = await createImageBitmap(file);

		// Downscale oversized images: a 24MP photo at MAX_IMAGE_DIMENSION
		// keeps full display quality while cutting WebP size dramatically.
		const scale = Math.min(1, MAX_IMAGE_DIMENSION / Math.max(bitmap.width, bitmap.height));
		const width = Math.max(1, Math.round(bitmap.width * scale));
		const height = Math.max(1, Math.round(bitmap.height * scale));

		const canvas = new OffscreenCanvas(width, height);
		const ctx = canvas.getContext('2d');
		if (!ctx) {
			bitmap.close();
			return null;
		}

		ctx.drawImage(bitmap, 0, 0, width, height);
		bitmap.close();

		const blob: Blob | null = await canvas.convertToBlob({
			type: 'image/webp',
			quality: quality ?? WEBP_QUALITY_DEFAULT
		});

		// Unsupported encoders silently fall back to PNG — uploading a PNG
		// mislabeled as image/webp would fail backend MIME verification.
		if (!blob || blob.type !== 'image/webp') return null;

		return new File([blob], renameExtension(file), { type: 'image/webp' });
	} catch {
		return null;
	}
}

/**
 * Decode a HEIC/HEIF file to PNG via libheif (heic2any). The WASM payload
 * is loaded lazily — only fetched when a HEIC file actually appears.
 * PNG preserves alpha for the subsequent WebP encode. Returns null when
 * decoding fails.
 */
async function decodeHeicToPng(file: File): Promise<File | null> {
	try {
		const { default: heic2any } = await import('heic2any');
		const converted = await heic2any({ blob: file, toType: 'image/png' });
		const blob = Array.isArray(converted) ? converted[0] : converted;
		return new File([blob], file.name.replace(HEIC_EXTENSIONS, '.png'), {
			type: 'image/png'
		});
	} catch {
		return null;
	}
}

/**
 * Convert an image File to WebP.
 *
 * - Non-image files are returned as-is.
 * - Files already in WebP, or in formats that lose data through canvas
 *   rasterization (animated GIF, SVG), are returned as-is.
 * - HEIC/HEIF files are decoded natively where supported (Safari) or via
 *   libheif WASM, then encoded like any other raster.
 * - Any conversion error returns the original file (graceful degradation).
 */
export async function convertImageToModernFormat(
	file: File,
	quality?: number
): Promise<File> {
	// Early exit: already WebP, or a format canvas would degrade.
	if (SKIP_MIMES.has(file.type)) return file;

	// HEIC/HEIF: try native canvas decode first (Safari handles it); when
	// that fails — Chrome, Firefox, most non-Apple platforms — decode via
	// libheif WASM, then run the normal WebP encode.
	if (isHeicLike(file)) {
		const viaCanvas = await canvasToWebP(file, quality);
		if (viaCanvas) return viaCanvas;

		const png = await decodeHeicToPng(file);
		if (!png) return file;

		return (await canvasToWebP(png, quality)) ?? file;
	}

	// Early exit: skip non-image files.
	if (!file.type.startsWith(IMAGE_MIME_PREFIX)) return file;

	return (await canvasToWebP(file, quality)) ?? file;
}
