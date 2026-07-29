// Helpers for the auth brand-panel carousel slides stored in system settings.
// The setting key is appearance.auth_brand_slides (JSONB array, public).

export interface BrandSlide {
	image: string;
	headline: string;
	subheading: string;
}

/**
 * Parse the appearance.auth_brand_slides value from GET /site/settings/public.
 * Accepts either a pre-parsed array (JSONB) or a raw JSON string. Returns an
 * empty array for any malformed input so the AuthShell can fall back to its
 * default static brand panel.
 */
export function parseBrandSlides(
	siteSettings: Record<string, unknown> | undefined | null
): BrandSlide[] {
	const raw = siteSettings?.['appearance.auth_brand_slides'];
	if (!raw) return [];
	try {
		const arr = typeof raw === 'string' ? JSON.parse(raw) : raw;
		if (!Array.isArray(arr)) return [];
		return arr
			.filter(
				(s): s is Record<string, string> =>
					typeof s === 'object' && s !== null && typeof s.image === 'string'
			)
			.map((s) => ({
				image: s.image,
				headline: typeof s.headline === 'string' ? s.headline : '',
				subheading: typeof s.subheading === 'string' ? s.subheading : ''
			}));
	} catch {
		return [];
	}
}
