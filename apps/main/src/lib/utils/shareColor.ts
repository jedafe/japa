/** Compute a readable text color for a platform brand color. */
export function shareContrastColor(bg: string): string {
	const hex = bg.replace('#', '');
	if (!/^[0-9a-f]{3,8}$/i.test(hex)) return '#fff';
	let r = 0, g = 0, b = 0;
	if (hex.length === 3 || hex.length === 4) {
		r = parseInt(hex[0] + hex[0], 16);
		g = parseInt(hex[1] + hex[1], 16);
		b = parseInt(hex[2] + hex[2], 16);
	} else if (hex.length === 6 || hex.length === 8) {
		r = parseInt(hex.slice(0, 2), 16);
		g = parseInt(hex.slice(2, 4), 16);
		b = parseInt(hex.slice(4, 6), 16);
	} else {
		return '#fff';
	}
	const luminance = (0.299 * r + 0.587 * g + 0.114 * b) / 255;
	return luminance > 0.6 ? '#1f2937' : '#fff';
}
