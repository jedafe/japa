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
	/** Optional OG image URL (absolute). Defaults to the static fallback
	 *  ({canonicalOrigin}/og/og-default.png). */
	ogImage?: string;
	/** Optional JSON-LD structured data object. */
	jsonLd?: Record<string, unknown>;
	/** Override the site name (defaults to "JapaConnect Tools"). */
	siteName?: string;
	/** Mark as noindex for tools that should not be indexed. */
	noindex?: boolean;
	/** Canonical tool host for cross-domain canonicals (ADR-086 single-tool
	 *  subdomains, e.g. "crs-calculator.japaconnect.com"). When set, the
	 *  canonical points at that host instead of the request origin, and an
	 *  `alternates` list is returned for hreflang/self references. */
	canonicalHost?: string;
	/** Enabled locale codes for hreflang alternates (e.g. ["en","fr"]).
	 *  The Tools app serves /fr/<path> prefixes (hooks.server.ts). */
	locales?: string[];
	/** Default locale (no URL prefix). Defaults to "en". */
	defaultLocale?: string;
}

/** Canonical Tools hub origin (ADR-086). */
export const TOOLS_ORIGIN = 'https://tools.japaconnect.com';

/** Default per-tool canonical hosts (ADR-086 default subdomains). */
const TOOL_CANONICAL_HOSTS: Record<string, string> = {
	'/visa-checklist': 'visa-checklist.japaconnect.com',
	'/readiness': 'readiness.japaconnect.com',
	'/cost-of-living': 'cost-of-living.japaconnect.com',
	'/scholarships': 'scholarships.japaconnect.com',
	'/remittance': 'remittance.japaconnect.com',
	'/japabot': 'japabot.japaconnect.com',
	'/visa-routes': 'visa-routes.japaconnect.com',
	'/embassy-alerter': 'embassy-alerter.japaconnect.com',
	'/arrival-checklist': 'arrival-checklist.japaconnect.com',
	'/processing-times': 'processing-times.japaconnect.com',
	'/credential-checker': 'credential-checker.japaconnect.com',
	'/cv-formatter': 'cv-formatter.japaconnect.com',
	'/housing-guide': 'housing-guide.japaconnect.com',
	'/attestation-guide': 'attestation-guide.japaconnect.com',
	'/language-test': 'language-test.japaconnect.com',
	'/tax-calculator': 'tax-calculator.japaconnect.com',
	'/community-locator': 'community-locator.japaconnect.com',
	'/crs-calculator': 'crs-calculator.japaconnect.com',
	'/pof-calculator': 'pof-calculator.japaconnect.com',
	'/eca-guide': 'eca-guide.japaconnect.com',
	'/sop-generator': 'sop-generator.japaconnect.com',
	'/currency-converter': 'currency-converter.japaconnect.com'
};

/** Resolve the canonical host for a tools path (longest-prefix match so
 *  detail routes inherit their tool's host). */
export function canonicalHostForPath(path: string): string {
	let best = '';
	for (const prefix of Object.keys(TOOL_CANONICAL_HOSTS)) {
		if ((path === prefix || path.startsWith(`${prefix}/`)) && prefix.length > best.length) {
			best = prefix;
		}
	}
	return best ? TOOL_CANONICAL_HOSTS[best] : 'tools.japaconnect.com';
}

export interface HreflangAlternate {
	hreflang: string;
	href: string;
}

/**
 * Returns the canonical tools origin. In production this is
 * https://tools.japaconnect.com; in dev it falls back to the request origin.
 * The origin is not known at module scope on the server, so callers pass the
 * absolute URL via the `origin` parameter when needed. For SSR, use
 * `event.url.origin` from the load function.
 *
 * Cross-domain canonical (ADR-086): each tool's canonical host is its
 * single-tool subdomain (canonicalHostForPath), NOT the request origin — so a
 * tool served from tools.japaconnect.com/<tool> and from
 * <tool>.japaconnect.com consolidates to one canonical URL.
 */
export function buildSeoMeta(input: SeoInput & { origin?: string }): {
	title: string;
	description: string;
	canonical: string;
	ogImage: string;
	siteName: string;
	noindex: boolean;
	jsonLdString: string | null;
	alternates: HreflangAlternate[];
} {
	const origin = input.origin ?? TOOLS_ORIGIN;
	// Cross-domain canonical: prefer the tool's canonical host in production
	// (single-tool subdomain per ADR-086); keep the request origin for local
	// dev (localhost) so canonicals stay clickable.
	const canonicalHost =
		input.canonicalHost ??
		(/localhost|127\.0\.0\.1/.test(origin) ? new URL(origin).host : canonicalHostForPath(input.path));
	const canonicalOrigin = /localhost|127\.0\.0\.1/.test(origin)
		? origin
		: `https://${canonicalHost}`;
	const canonical = `${canonicalOrigin}${input.path}`;
	// Per-tool og:image: caller-provided absolute URL wins; otherwise fall
	// back to the static default under /og/ on the canonical origin.
	const ogImage = input.ogImage ?? `${canonicalOrigin}/og/og-default.png`;
	const siteName = input.siteName ?? DEFAULT_SITE_NAME;
	const jsonLdString = input.jsonLd ? JSON.stringify(input.jsonLd) : null;

	// hreflang alternates (EN-only when locales is unset — see per-page docs).
	const locales = input.locales ?? [];
	const defaultLocale = input.defaultLocale ?? 'en';
	const alternates: HreflangAlternate[] = locales.map((lang) => ({
		hreflang: lang,
		href:
			lang === defaultLocale
				? canonical
				: `${canonicalOrigin}/${lang}${input.path}`
	}));

	return {
		title: input.title,
		description: input.description,
		canonical,
		ogImage,
		siteName,
		noindex: input.noindex ?? false,
		jsonLdString,
		alternates
	};
}

const DEFAULT_SITE_NAME = 'JapaConnect Tools';

/** Serialize JSON-LD safely for inline <script> (escape closing tags). */
export function safeJsonLd(jsonLdString: string): string {
	return jsonLdString.replace(/<\//g, '<\\/');
}

/** Cross-app link to the Main app (login, signup, profile). */
export function mainAppUrl(path: string): string {
	return `${PUBLIC_MAIN_ORIGIN}${path}`;
}
