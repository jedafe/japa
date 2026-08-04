import { PUBLIC_MAIN_ORIGIN } from '$env/static/public';

/**
 * SEO metadata helper for the Tools surface.
 * Builds <svelte:head> content with Open Graph and Twitter Card tags.
 *
 * Ref: §78 (Tools Web Surface), ROADMAP-TOOLS Phase 04.02.
 */

export interface SeoInput {
	title: string;
	description: string;
	/** Canonical path on the tools surface (e.g. "/visa-checklist"). */
	path: string;
	/** Optional OG image URL (absolute). */
	ogImage?: string;
	/** Optional JSON-LD structured data object. */
	jsonLd?: Record<string, unknown>;
	/** Override the site name (defaults to "JapaConnect Tools"). */
	siteName?: string;
	/** Mark as noindex for tools that should not be indexed. */
	noindex?: boolean;
}

const DEFAULT_SITE_NAME = 'JapaConnect Tools';

/**
 * Returns the canonical tools origin. In production this is
 * https://tools.japaconnect.com; in dev it falls back to the request origin.
 * The origin is not known at module scope on the server, so callers pass the
 * absolute URL via the `origin` parameter when needed. For SSR, use
 * `event.url.origin` from the load function.
 */
export function buildSeoMeta(input: SeoInput & { origin?: string }): {
	title: string;
	description: string;
	canonical: string;
	ogImage: string;
	siteName: string;
	noindex: boolean;
	jsonLdString: string | null;
} {
	const origin = input.origin ?? `https://tools.japaconnect.com`;
	const canonical = `${origin}${input.path}`;
	const ogImage = input.ogImage ?? `${origin}/og-default.png`;
	const siteName = input.siteName ?? DEFAULT_SITE_NAME;
	const jsonLdString = input.jsonLd ? JSON.stringify(input.jsonLd) : null;

	return {
		title: input.title,
		description: input.description,
		canonical,
		ogImage,
		siteName,
		noindex: input.noindex ?? false,
		jsonLdString
	};
}

/** Cross-app link to the Main app (login, signup, profile). */
export function mainAppUrl(path: string): string {
	return `${PUBLIC_MAIN_ORIGIN}${path}`;
}
