// See https://svelte.dev/docs/kit/types#app.d.ts
import type { SessionUser, ThemeName, Locale } from '$lib/types';

declare global {
	namespace App {
		interface Error {
			code: string;
			message: string;
		}
		interface Locals {
			/** Bearer token from the jc_token cookie, or null for guests. */
			token: string | null;
			/** Resolved current user, or null when unauthenticated (guest). */
			user: SessionUser | null;
			/** Active colour theme selection, resolved from the jc_theme cookie.
			 *  May be 'system' — see resolvedTheme for the SSR-rendered value. */
			theme: ThemeName;
			/** The theme written to <html data-theme> during SSR. Equals `theme`
			 *  unless 'system', where it falls back to dark_mode_default. */
			resolvedTheme: 'light' | 'dark';
			/** Hostname (port stripped) used for reverse-proxy and host-based routing. */
			requestHost: string;
			/** Per-request CSP nonce for the staff-gated custom-code blocks. */
			cspNonce: string;
			/** Resolved UI locale code (e.g. 'en', 'fr', 'yo'), from the jc_locale
			 *  cookie or the locale prefix in the URL. Dynamic per ADR-013. */
			locale: Locale;
			/** True when the request host is the Japa Academy surface (ADR-063). */
			isAcademyHost: boolean;
			/** True when the request host is the public roadmap surface (ADR-127). */
			isRoadmapHost: boolean;
			/**
			 * Double-submit CSRF token fetched from GET /csrf (Ref: §5.2). Threaded
			 * through ApiContext so server-side mutations attach X-CSRF-Token. Null
			 * for guests (no mutations) or when the backend is unreachable.
			 */
			csrfToken: string | null;
			/**
			 * Per-request CSP nonce for the staff-gated custom-code blocks
			 * (`site.custom_css` / `site.custom_js`). Emitted server-side in
			 * hooks.server.ts; threaded through `data.cspNonce` so the layout's
			 * inline `<style>`/`<script>` tags carry `nonce={...}` and stay
			 * executable under a nonce-based style-src/script-src policy.
			 */
			cspNonce: string;
			/**
			 * Public site settings fetched once per request (cached) and reused by
			 * page load functions. Ref: conventions/system-settings-catalog.md.
			 */
			siteSettings: Record<string, unknown>;
			/**
			 * Edge-CDN cache tags captured from upstream API responses during SSR
			 * (ADR-134) — header name (cf-cache-tag / cdn-tag) → deduped tag set,
			 * re-emitted on the HTML response so the edge can tag-purge pages.
			 */
			edgeCacheTags?: Map<string, Set<string>>;
		}
		interface PageData {
			user: SessionUser | null;
			theme: ThemeName;
			locale: Locale;
			isAcademyHost: boolean;
			/** True on roadmap.japa.to — renders the minimal roadmap shell (ADR-127). */
			isRoadmapHost: boolean;
			/** CSRF token for client-side mutations (echoed as X-CSRF-Token). */
			csrfToken: string | null;
			/** Per-request CSP nonce for the staff-gated custom-code blocks. */
			cspNonce: string;
			/** Feature flags from GET /feature-flags (public endpoint). */
			features?: Record<string, boolean>;
		}
		// interface PageState {}
		// interface Platform {}
	}
}

export {};
