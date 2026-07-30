/**
 * Client-side image conversion to modern formats (AVIF / WebP).
 *
 * Uses OffscreenCanvas + createImageBitmap for zero-main-thread conversion.
 * Falls back to the original file on any error or unsupported browser.
 */

const AVIF_QUALITY_DEFAULT = 0.8;
const WEBP_QUALITY_DEFAULT = 0.85;

const IMAGE_MIME_PREFIX = 'image/';

/** MIME types we consider already "modern" enough to skip conversion. */
const MODERN_MIMES = new Set(['image/avif', 'image/webp']);

/** Test whether the browser can encode AVIF via OffscreenCanvas. */
let _avifSupported: boolean | null = null;
async function isAvifSupported(): Promise<boolean> {
	if (_avifSupported !== null) return _avifSupported;
	try {
		const canvas = new OffscreenCanvas(1, 1);
		const ctx = canvas.getContext('2d');
		if (!ctx) { _avifSupported = false; return false; }
		ctx.fillStyle = '#000';
		ctx.fillRect(0, 0, 1, 1);
		const blob: Blob | null = await canvas.convertToBlob({ type: 'image/avif', quality: 0.5 });
		_avifSupported = blob !== null && blob.type === 'image/avif';
	} catch {
		_avifSupported = false;
	}
	return _avifSupported;
}

/** Replace the file extension to match the new MIME type. */
function renameExtension(file: File, newMime: string): string {
	const name = file.name;
	const lastDot = name.lastIndexOf('.');
	const base = lastDot > 0 ? name.slice(0, lastDot) : name;
	const ext = newMime === 'image/avif' ? '.avif' : '.webp';
	return `${base}${ext}`;
}

/**
 * Convert an image File to AVIF (if supported) or WebP.
 *
 * - Non-image files are returned as-is.
 * - Files already in AVIF/WebP are returned as-is.
 * - Any conversion error returns the original file (graceful degradation).
 */
export async function convertImageToModernFormat(
	file: File,
	quality?: number
): Promise<File> {
	// Early exit: skip non-image files.
	if (!file.type.startsWith(IMAGE_MIME_PREFIX)) return file;

	// Early exit: already a modern format.
	if (MODERN_MIMES.has(file.type)) return file;

	try {
		const bitmap = await createImageBitmap(file);

		const useAvif = await isAvifSupported();
		const targetMime = useAvif ? 'image/avif' : 'image/webp';
		const targetQuality = quality ?? (useAvif ? AVIF_QUALITY_DEFAULT : WEBP_QUALITY_DEFAULT);

		const canvas = new OffscreenCanvas(bitmap.width, bitmap.height);
		const ctx = canvas.getContext('2d');
		if (!ctx) { bitmap.close(); return file; }

		ctx.drawImage(bitmap, 0, 0);
		bitmap.close();

		const blob: Blob | null = await canvas.convertToBlob({
			type: targetMime,
			quality: targetQuality
		});

		if (!blob) return file;

		const newName = renameExtension(file, targetMime);
		return new File([blob], newName, { type: targetMime });
	} catch {
		// Graceful fallback: return the original file unchanged.
		return file;
	}
}
