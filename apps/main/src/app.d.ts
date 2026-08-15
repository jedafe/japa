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
			/** Active colour theme, resolved from the jc_theme cookie. */
			theme: ThemeName;
			/** Hostname (port stripped) used for backend tenant resolution. */
			tenantHost: string;
			/** Resolved UI locale code (e.g. 'en', 'fr', 'yo'), from the jc_locale
			 *  cookie or the locale prefix in the URL. Dynamic per ADR-013. */
			locale: Locale;
			/** True when the request host is the Japa University surface (ADR-063). */
			isUniversityHost: boolean;
			/**
			 * Double-submit CSRF token fetched from GET /csrf (Ref: §5.2). Threaded
			 * through ApiContext so server-side mutations attach X-CSRF-Token. Null
			 * for guests (no mutations) or when the backend is unreachable.
			 */
			csrfToken: string | null;
		}
		interface PageData {
			user: SessionUser | null;
			theme: ThemeName;
			locale: Locale;
			isUniversityHost: boolean;
			/** CSRF token for client-side mutations (echoed as X-CSRF-Token). */
			csrfToken: string | null;
			/** Feature flags from GET /feature-flags (public endpoint). */
			features?: Record<string, boolean>;
		}
		// interface PageState {}
		// interface Platform {}
	}
}

export {};
