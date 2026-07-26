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
			/** Resolved UI locale, from the jc_locale cookie ('en' | 'fr'). */
			locale: Locale;
		}
		interface PageData {
			user: SessionUser | null;
			theme: ThemeName;
			locale: Locale;
		}
		// interface PageState {}
		// interface Platform {}
	}
}

export {};
