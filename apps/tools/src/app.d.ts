// See https://svelte.dev/docs/kit/types#app.d.ts
import type { SessionUser, ThemeName, Locale, ToolSurface } from '$lib/types';

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
			/** Active colour theme, resolved from the jc_theme cookie. */
			theme: ThemeName;
			/** Hostname (port stripped) used for reverse-proxy and host-based routing. */
			requestHost: string;
			/** Resolved UI locale code (e.g. 'en', 'fr', 'yo'), from the jc_locale
			 *  cookie or the locale prefix in the URL. Dynamic per ADR-013. */
			locale: Locale;
			/**
			 * ADR-086 / §78: which tool surface the request host maps to.
			 * 'hub' = full tools listing (tools.japaconnect.com default);
			 * a specific tool slug = single-tool shell mode.
			 */
			toolSurface: ToolSurface;
			/**
			 * Double-submit CSRF token fetched from GET /csrf (Ref: §5.2).
			 * Threaded through ApiContext so server-side mutations attach
			 * X-CSRF-Token. Null for guests or when the backend is unreachable.
			 */
			csrfToken: string | null;
			/**
			 * Public site settings fetched once in hooks.server.ts and reused
			 * by layout/page load functions to avoid redundant backend fetches.
			 * Ref: §4.1 (performance — eliminate duplicate /site/settings/public).
			 */
			siteSettings: Record<string, unknown>;
			/**
			 * Feature flags fetched once in hooks.server.ts and reused by
			 * layout/page load functions to avoid redundant backend fetches.
			 */
			features: Record<string, boolean>;
		}
		interface PageData {
			user: SessionUser | null;
			theme: ThemeName;
			locale: Locale;
			toolSurface: ToolSurface;
			/** CSRF token for client-side mutations (echoed as X-CSRF-Token). */
			csrfToken: string | null;
		}
		// interface PageState {}
		// interface Platform {}
	}
}

export {};
