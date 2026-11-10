/**
 * §19 shared consent state — mirrors the jc_consent cookie so
 * consent-gated scripts (Umami) and the backend jc_guest gate can never
 * disagree. CookieConsentBanner writes it on decide()/reopen; the root
 * layout reads it to gate script injection.
 *
 * Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
 */
import { browser } from '$app/env';

export const consent = $state({ granted: false });

/** Strict opt-in: only jc_consent=all counts — absent/unknown means denied. */
export function consentGranted(): boolean {
	return document.cookie.split('; ').some((c) => c === 'jc_consent=all');
}

/** Sync state from the cookie — call once on mount (SSR keeps granted=false). */
export function initConsent(): void {
	consent.granted = browser && consentGranted();
}
