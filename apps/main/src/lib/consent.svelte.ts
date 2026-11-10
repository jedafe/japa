/**
 * §19 shared consent model — mirrors the jc_consent cookie so
 * consent-gated features (Umami injection, the backend jc_guest gate)
 * and the audit records can never disagree. CookieConsentBanner writes
 * it on decide()/reopen; the root layout reads it to gate scripts;
 * DataPrivacySection keeps it in sync when a cookie-gated type toggles.
 *
 * Cookie value encoding: 'all' | 'necessary' | a '+'-joined list of
 * granted consent_type tokens (e.g. 'functional_cookies+analytics_tracking')
 * — the same contract GuestSessionMiddleware.ConsentGrants parses.
 * '+' is a legal RFC 6265 cookie-octet; ',' is not — parsers on both
 * sides also accept ','/whitespace so older cookies still decode.
 *
 * Lives in a .svelte.ts module because it uses $state (required for
 * runes outside components).
 *
 * Ref: japaconnect-engineering/sections/19-gdpr-ndpr-compliance.md
 */
import { browser } from '$app/env';
import { PUBLIC_JC_API_URL } from '$app/env/public';

const CONSENT_COOKIE = 'jc_consent';
const CONSENT_MAX_AGE = 365 * 24 * 60 * 60; // 1 year

// The consent_type tokens jc_consent can carry — a cookie containing
// anything else counts as no decision (fail closed + re-prompt).
const KNOWN_TYPES = new Set(['functional_cookies', 'analytics_tracking']);

// A granted-token list may be separated by '+', ',', or whitespace —
// writers emit '+', older cookies used ',', and any layer that
// URL-decodes '+' into a space still parses.
const SPLIT_RE = /[,+\s]+/;

/** Consent state mirrored into every consumer. Strict opt-in: both flags
 *  stay false until an affirmative decision is read from the cookie. */
export const consent = $state({ functional: false, analytics: false });

function rawConsentValue(): string {
	return (
		document.cookie
			.split('; ')
			.find((c) => c.startsWith(`${CONSENT_COOKIE}=`))
			?.slice(CONSENT_COOKIE.length + 1) ?? ''
	);
}

function grantedTokens(raw: string): string[] {
	if (raw === 'all') return [...KNOWN_TYPES];
	if (raw === '' || raw === 'necessary') return [];
	return raw.split(SPLIT_RE).filter(Boolean);
}

/** Parse the jc_consent cookie into per-category flags. */
export function readConsent(): { functional: boolean; analytics: boolean } {
	const granted = grantedTokens(rawConsentValue());
	return {
		functional: granted.includes('functional_cookies'),
		analytics: granted.includes('analytics_tracking')
	};
}

/** True only when jc_consent holds a value we recognize — a malformed or
 *  unknown value means the banner should re-prompt rather than silently
 *  suppress itself forever on a cookie that grants nothing. */
export function hasConsentDecision(): boolean {
	const raw = rawConsentValue();
	if (raw === 'all' || raw === 'necessary') return true;
	const tokens = raw.split(SPLIT_RE).filter(Boolean);
	return tokens.length > 0 && tokens.every((tok) => KNOWN_TYPES.has(tok));
}

/** Encode a category decision: 'all' when everything is granted,
 *  'necessary' when nothing is, else the '+'-joined list of granted
 *  consent_type tokens. */
export function consentCookieValue(functional: boolean, analytics: boolean): string {
	if (functional && analytics) return 'all';
	if (!functional && !analytics) return 'necessary';
	return [
		functional ? 'functional_cookies' : '',
		analytics ? 'analytics_tracking' : ''
	]
		.filter(Boolean)
		.join('+');
}

// Scope the cookie to the shared base domain so api.<domain> receives it,
// but only when the current host is same-site with the API host. On
// custom-domain surfaces a .japa.to attribute would be rejected outright —
// host-only fallback (api.* is cross-site there regardless).
function cookieDomain(): string {
	try {
		const apiHost = new URL(PUBLIC_JC_API_URL ?? '').hostname;
		if (!apiHost.startsWith('api.')) return '';
		const base = apiHost.slice(4);
		const host = location.hostname;
		return host === base || host.endsWith(`.${base}`) ? `.${base}` : '';
	} catch {
		return '';
	}
}

function writeCookie(value: string, maxAge: number): void {
	const domain = cookieDomain();
	const secure = location.protocol === 'https:' ? '; Secure' : '';
	document.cookie =
		`${CONSENT_COOKIE}=${value}; path=/; max-age=${maxAge}; SameSite=Lax` +
		(domain ? `; domain=${domain}` : '') +
		secure;
}

/** Apply a decision: write the cookie + mirror into shared state so
 *  consent-gated scripts react without a reload. */
export function setConsent(functional: boolean, analytics: boolean): void {
	writeCookie(consentCookieValue(functional, analytics), CONSENT_MAX_AGE);
	consent.functional = functional;
	consent.analytics = analytics;
}

/** Withdraw the decision — delete the cookie and reset state. */
export function clearConsent(): void {
	writeCookie('', 0);
	consent.functional = false;
	consent.analytics = false;
}

/** Sync state from the cookie — call once on mount (SSR keeps flags false). */
export function initConsent(): void {
	if (!browser) return;
	const c = readConsent();
	consent.functional = c.functional;
	consent.analytics = c.analytics;
}
