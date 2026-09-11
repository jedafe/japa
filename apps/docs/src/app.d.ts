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
			/** Resolved UI locale code (e.g. 'en', 'fr', 'yo'), from the jc_locale
			 *  cookie or the locale prefix in the URL. Dynamic per ADR-013. */
			locale: Locale;
			/**
			 * Audiences the current session is permitted to read, server-resolved
			 * from the user's groups + the always-public 'api' audience. The nav
			 * tree's audience switcher only offers these; gated pages 403 →
			 * /docs/access-denied (ADR-060 — server-side gating only).
			 */
			accessibleAudiences: DocAudience[];
			/**
			 * RBAC permission node required for each gated docs audience.
			 * Fetched from GET /docs/audience-gates so the frontend mirrors the
			 * backend AudienceGates map (DOC-007).
			 */
			audienceGates: Record<string, string>;
			/**
			 * Canonical audience order fetched from GET /docs/audiences.
			 * Used in place of the hardcoded AUDIENCE_ORDER constant (Rule 16).
			 */
			audienceOrder: DocAudience[];
			/**
			 * Double-submit CSRF token fetched from GET /csrf (Ref: §5.2).
			 * Threaded through ApiContext so server-side mutations attach
			 * X-CSRF-Token. Null for guests or when the backend is unreachable.
			 */
			csrfToken: string | null;
			/**
			 * Whether the left navigation rail is collapsed. Persisted via a
			 * `jc_nav_collapsed` cookie (ADR-058). The backend /me/account endpoint
			 * does not currently store UI preferences, so a cookie is used.
			 */
			navCollapsed: boolean;
		}
		interface PageData {
			user: SessionUser | null;
			theme: ThemeName;
			locale: Locale;
			accessibleAudiences: DocAudience[];
			audienceGates: Record<string, string>;
			/** Feature flags from GET /feature-flags (public endpoint). */
			features?: Record<string, boolean>;
			/** Canonical audience order from GET /docs/audiences (Rule 16). */
			audienceOrder: DocAudience[];
			/** Left navigation collapsed state from the `jc_nav_collapsed` cookie. */
			navCollapsed: boolean;
		}
		// interface PageState {}
		// interface Platform {}
	}
}

export {};
