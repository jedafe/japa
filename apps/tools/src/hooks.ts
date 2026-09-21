import type { Reroute } from '@sveltejs/kit';

/**
 * Locale-prefixed URLs (/fr/visa-checklist, /yo/...) route to the same page
 * tree as the unprefixed path. Routing happens before the server `handle`
 * hook, so stripping the prefix in handle (by mutating event.url) has no
 * effect on route matching — it must happen here in `reroute`.
 *
 * The 2-letter segment is stripped unconditionally; hooks.server.ts then
 * 404s when the prefix is not one of the enabled i18n languages, preserving
 * the previous behavior for unsupported locales.
 *
 * Ref: §18 (i18n language packs), ADR-013.
 */
export const reroute: Reroute = ({ url }) => {
	const stripped = url.pathname.replace(/^\/[a-z]{2}(?=\/|$)/, '');
	return stripped || '/';
};
