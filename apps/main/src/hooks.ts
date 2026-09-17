import type { Reroute } from '@sveltejs/kit';

// §18 / ADR-013 — locale-prefixed URLs (/fr/blog, /yo/blog) render the
// unprefixed route in that language; the locale cookie is set in
// hooks.server.ts. Route matching happens BEFORE `handle`, so the prefix
// strip must live in `reroute` — mutating event.url.pathname in `handle`
// only changes what load functions see, not which route matched.
//
// The allowlist mirrors i18n.enabled_languages minus the unprefixed default
// (en). `reroute` cannot await system_settings, so keep this in sync with
// the shipped packs in src/lib/i18n/*.json — an unknown /xx/ prefix must
// still 404 rather than silently render a different route (e.g. /me/*).
const PREFIXED_LOCALES = new Set(['fr', 'yo']);

export const reroute: Reroute = ({ url }) => {
	const m = url.pathname.match(/^\/([a-z]{2})(?=\/|$)/);
	if (m && PREFIXED_LOCALES.has(m[1])) {
		return url.pathname.slice(3) || '/';
	}
	return url.pathname;
};
