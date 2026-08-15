// See https://svelte.dev/docs/kit/types#app.d.ts
import type { SessionUser, ThemeName, Locale, DocAudience } from '$lib/types';

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
			/** Resolved UI locale, from the jc_locale cookie ('en' | 'fr'). */
			locale: Locale;
			/**
			 * Audiences the current session is permitted to read, server-resolved
			 * from the user's groups + the always-public 'api' audience. The nav
			 * tree's audience switcher only offers these; gated pages 403 →
			 * /docs/access-denied (ADR-060 — server-side gating only).
			 */
			accessibleAudiences: DocAudience[];
			/**
			 * Double-submit CSRF token fetched from GET /csrf (Ref: §5.2).
			 * Threaded through ApiContext so server-side mutations attach
			 * X-CSRF-Token. Null for guests or when the backend is unreachable.
			 */
			csrfToken: string | null;
		}
		interface PageData {
			user: SessionUser | null;
			theme: ThemeName;
			locale: Locale;
			accessibleAudiences: DocAudience[];
			/** Feature flags from GET /feature-flags (public endpoint). */
			features?: Record<string, boolean>;
		}
		// interface PageState {}
		// interface Platform {}
	}
}

export {};
