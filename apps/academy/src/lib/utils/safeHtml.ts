/**
 * Lightweight client-side HTML sanitizer and URL guard for sidebar widgets.
 *
 * Defense-in-depth alongside the server-side sanitizer in
 * internal/content.SanitizeHTML (called from internal/sidebars/service.go's
 * sanitizeWidgetProps). The backend is the primary XSS gate; this helper
 * ensures that even if a malformed widget row reaches the frontend (e.g. via
 * a stale cache, a direct DB insert, or a future widget type that bypasses
 * the server sanitizer), the rendered HTML cannot execute script.
 *
 * Ref: Bug Hunter's Guide §5.1 (Injection & XSS), JEOS Section 59.
 */

const ALLOWED_TAGS = new Set([
	'div', 'span', 'p', 'br', 'hr', 'b', 'i', 'strong', 'em', 'u', 's',
	'code', 'pre', 'blockquote', 'a', 'ul', 'ol', 'li', 'h1', 'h2', 'h3',
	'h4', 'h5', 'h6', 'img', 'figure', 'figcaption', 'section', 'article',
	'header', 'footer', 'nav', 'aside', 'table', 'thead', 'tbody', 'tr',
	'th', 'td', 'button', 'svg', 'rect', 'path', 'line', 'polyline',
	'circle', 'ellipse', 'g', 'defs', 'use'
]);

const ALLOWED_ATTRS = new Set([
	'class', 'style', 'title', 'href', 'target', 'rel',
	'd', 'x', 'y', 'x1', 'y1', 'x2', 'y2', 'width', 'height', 'viewBox',
	'fill', 'stroke', 'stroke-width', 'stroke-linecap', 'stroke-linejoin',
	'rx', 'ry', 'cx', 'cy', 'r', 'xmlns', 'xlink:href', 'points', 'src',
	'alt', 'colspan', 'rowspan', 'id', 'data-*'
]);

/**
 * Sanitize an HTML string for safe rendering via `{@html}`.
 *
 * Strips `<script>`/`<style>` blocks entirely, removes `on*` event-handler
 * attributes and `javascript:`/`data:`/`vbscript:` URIs, and restricts tags
 * to an allowlist of structural + formatting elements.
 */
export function sanitizeSidebarHtml(html: string): string {
	if (!html) return '';
	// 1. Drop <script> / <style> blocks (including their content).
	let out = html.replace(
		/<(script|style|iframe|object|embed|link|meta)\b[^>]*>[\s\S]*?<\/\1\s*>/gi,
		''
	);
	// 2. Walk every remaining tag and clean it.
	out = out.replace(
		/<(\/?)([a-zA-Z][a-zA-Z0-9]*)(\b[^>]*)?>/g,
		(_match, slash: string, tag: string, attrs: string) => {
			const lower = tag.toLowerCase();
			if (!ALLOWED_TAGS.has(lower)) return '';
			if (slash) return `</${lower}>`;
			if (!attrs) return `<${lower}>`;
			const safeAttrs: string[] = [];
			const attrRe = /\s+([a-zA-Z_:][a-zA-Z0-9_.:-]*)\s*=\s*("[^"]*"|'[^']*'|\S+)/g;
			let m: RegExpExecArray | null;
			while ((m = attrRe.exec(attrs)) !== null) {
				const name = m[1].toLowerCase();
				const value = m[2];
				// Drop any event handler.
				if (name.startsWith('on')) continue;
				// Reject executable / overlay styles (position:fixed phish veil).
				if (name === 'style') {
					const unquoted = value.replace(/^["']|["']$/g, '');
					if (UNSAFE_STYLE.test(unquoted)) continue;
				}
				// Allow data-* attributes.
				if (!ALLOWED_ATTRS.has(name) && !name.startsWith('data-')) continue;
				// Guard URL-bearing attributes against dangerous schemes.
				if (name === 'src' || name === 'href' || name === 'xlink:href') {
					const unquoted = value.replace(/^["']|["']$/g, '').trim();
					if (isDangerousUrl(unquoted)) continue;
				}
				// data-url survives the sanitizer and is later assigned to
				// link.href by hydrate actions — validate it like any URL.
				if (name === 'data-url') {
					const unquoted = value.replace(/^["']|["']$/g, '').trim();
					if (!safeHref(unquoted)) continue;
				}
				safeAttrs.push(` ${name}=${value}`);
			}
			ensureNoopenerRel(lower, safeAttrs);
			return `<${lower}${safeAttrs.join('')}>`;
		}
	);
	return out;
}

/**
 * Guard a URL for safe use in `href`/`src`. Returns the URL if it uses a safe
 * scheme (http, https, mailto, tel, or a relative/anchor path), or `''` if it
 * uses a dangerous scheme (`javascript:`, `data:`, `vbscript:`) that would
 * execute script when navigated.
 *
 * Use this for any admin-authored URL rendered into an `href` attribute
 * (social_links, cta_button, upcoming_events).
 */
export function safeHref(url: string | undefined | null): string {
	if (!url || typeof url !== 'string') return '';
	const trimmed = url.trim();
	if (!trimmed) return '';
	// Reject protocol-relative URLs (//evil.com inherits the page scheme).
	if (trimmed.startsWith('//')) return '';
	if (isDangerousUrl(trimmed)) return '';
	return trimmed;
}

// Any `style` value that could execute code, load remote content, or overlay
// the page (phish click-jacking via position:fixed) rejects the whole attribute.
const UNSAFE_STYLE =
	/url\s*\(|expression\s*\(|@import|@charset|javascript|vbscript|behavior\s*:|binding\s*:|-moz-binding|position\s*:/i;

/**
 * Decode numeric/known-named HTML entities (looped for double-encoding).
 * The browser decodes `&#106;avascript:` on click/navigation, so scheme
 * checks must run on the decoded value, not the raw attribute text.
 */
function decodeEntities(value: string): string {
	const named: Record<string, string> = {
		colon: ':',
		semi: ';',
		comma: ',',
		sol: '/',
		excl: '!',
		lpar: '(',
		rpar: ')',
		tab: '\t',
		newline: '\n',
		quot: '"',
		amp: '&',
		lt: '<',
		gt: '>',
		nbsp: ' '
	};
	let out = value;
	for (let i = 0; i < 5; i++) {
		const prev = out;
		out = out
			.replace(/&#(\d+);?/g, (_m, d: string) => {
				const c = parseInt(d, 10);
				return Number.isFinite(c) ? String.fromCharCode(c) : _m;
			})
			.replace(/&#[xX]([0-9a-fA-F]+);?/g, (_m, h: string) => {
				const c = parseInt(h, 16);
				return Number.isFinite(c) ? String.fromCharCode(c) : _m;
			})
			.replace(/&([a-zA-Z]+);?/g, (m, n: string) => named[n.toLowerCase()] ?? m);
		if (out === prev) break;
	}
	return out;
}

function isDangerousUrl(url: string): boolean {
	// Decode entities BEFORE the scheme check, then strip ASCII control
	// chars/whitespace (ignored around the scheme by browsers).
	const decoded = decodeEntities(url)
		.split('')
		.filter((ch) => ch.charCodeAt(0) > 32)
		.join('');
	return /^(javascript|data|vbscript):/i.test(decoded);
}

/** Force `rel="noopener noreferrer"` on `_blank` links (reverse tabnabbing). */
function ensureNoopenerRel(tag: string, safeAttrs: string[]): void {
	if (tag !== 'a') return;
	const hasBlank = safeAttrs.some((a) => /^\s*target\s*=\s*(["']?)_blank\1$/i.test(a.trim()));
	if (!hasBlank) return;
	const idx = safeAttrs.findIndex((a) => /^\s*rel\s*=/i.test(a));
	if (idx === -1) {
		safeAttrs.push(' rel="noopener noreferrer"');
		return;
	}
	const m = safeAttrs[idx].match(/^\s*rel\s*=\s*(["'])(.*)\1$/);
	if (!m) return;
	const tokens = m[2].split(/\s+/).filter(Boolean);
	if (!tokens.includes('noopener')) tokens.push('noopener');
	if (!tokens.includes('noreferrer')) tokens.push('noreferrer');
	safeAttrs[idx] = ` rel="${tokens.join(' ')}"`;
}

/**
 * Parse the host from an absolute `http(s)` URL. Returns empty string for
 * relative URLs or non-HTTP schemes. Works in both browser and SSR (Node/Bun).
 */
function urlHost(url: string): string {
	try {
		const u = new URL(url, 'http://localhost');
		return u.protocol === 'http:' || u.protocol === 'https:' ? u.host : '';
	} catch {
		return '';
	}
}

/**
 * Sanitize an HTML string intended for content body rendering (article/overview).
 *
 * Builds on `sanitizeSidebarHtml` but preserves `iframe` (and media tags) so
 * that safe embedded content (YouTube, Vimeo, etc.) survives the client-side
 * pass. The `allowedIframeHosts` parameter is the comma-separated setting from
 * `content.sanitization.allowed_iframe_hosts`; when empty it falls back to a
 * sensible default set. Iframes whose `src` is not on an allowed host are dropped.
 *
 * This is defense-in-depth alongside the server-side sanitizer
 * `internal/content.SanitizeHTML`. Ref: Bug Hunter's Guide §5.1.
 */
export function sanitizeContentHtml(html: string, allowedIframeHosts?: string): string {
	if (!html) return '';

	const defaultHosts = 'youtube.com,youtube-nocookie.com,player.vimeo.com';
	const hostList = (allowedIframeHosts || defaultHosts).split(',').map((h) => h.trim()).filter(Boolean);
	const allowedHosts = new Set(hostList);

	// 1. Drop <script> / <style> blocks and other active objects, but keep
	//    <iframe> for the second pass.
	let out = html.replace(
		/<(script|style|object|embed|link|meta)\b[^>]*>[\s\S]*?<\/\1\s*>/gi,
		''
	);

	// 2. Allow a larger tag set than sidebar widgets.
	const allowedTags = new Set([...ALLOWED_TAGS, 'iframe', 'video', 'audio', 'source', 'track', 'picture']);
	const allowedAttrs = new Set([
		...ALLOWED_ATTRS,
		'allow', 'allowfullscreen', 'frameborder', 'loading', 'referrerpolicy', 'controls', 'poster', 'preload', 'muted', 'loop', 'autoplay'
	]);

	out = out.replace(
		/<(\/?)([a-zA-Z][a-zA-Z0-9]*)(\b[^>]*)?>/g,
		(_match, slash: string, tag: string, attrs: string) => {
			const lower = tag.toLowerCase();

			if (lower === 'iframe') {
				if (slash) return '</iframe>';
				if (!attrs) return '';
				const srcMatch = attrs.match(/\ssrc\s*=\s*("[^"]*"|'[^']*'|\S+)/i);
				if (!srcMatch) return '';
				const src = srcMatch[1].replace(/^["']|["']$/g, '').trim();
				const host = urlHost(src);
				if (!host || !allowedHosts.has(host)) return '';

				const safeAttrs: string[] = [];
				const attrRe = /\s+([a-zA-Z_:][a-zA-Z0-9_.:-]*)\s*=\s*("[^"]*"|'[^']*'|\S+)/g;
				let m: RegExpExecArray | null;
				while ((m = attrRe.exec(attrs)) !== null) {
					const name = m[1].toLowerCase();
					const value = m[2];
					if (name.startsWith('on')) continue;
					if (!allowedAttrs.has(name)) continue;
					if (['src', 'href', 'xlink:href'].includes(name)) {
						const unquoted = value.replace(/^["']|["']$/g, '').trim();
						if (isDangerousUrl(unquoted)) continue;
					}
					safeAttrs.push(` ${name}=${value}`);
				}
				return `<iframe${safeAttrs.join('')}></iframe>`;
			}

			if (!allowedTags.has(lower)) return '';
			if (slash) return `</${lower}>`;
			if (!attrs) return `<${lower}>`;

			const safeAttrs: string[] = [];
			const attrRe = /\s+([a-zA-Z_:][a-zA-Z0-9_.:-]*)\s*=\s*("[^"]*"|'[^']*'|\S+)/g;
			let m: RegExpExecArray | null;
			while ((m = attrRe.exec(attrs)) !== null) {
				const name = m[1].toLowerCase();
				const value = m[2];
				// Drop any event handler.
				if (name.startsWith('on')) continue;
				if (name === 'style') {
					const unquoted = value.replace(/^["']|["']$/g, '');
					if (UNSAFE_STYLE.test(unquoted)) continue;
				}
				// Allow data-* attributes.
				if (!allowedAttrs.has(name) && !name.startsWith('data-')) continue;
				// Guard URL-bearing attributes against dangerous schemes.
				if (['src', 'href', 'xlink:href'].includes(name)) {
					const unquoted = value.replace(/^["']|["']$/g, '').trim();
					if (isDangerousUrl(unquoted)) continue;
					if (lower === 'video' || lower === 'audio' || lower === 'source') {
						// For media tags we require an http(s) src.
						if (urlHost(unquoted) === '') continue;
					}
				}
				if (name === 'data-url') {
					const unquoted = value.replace(/^["']|["']$/g, '').trim();
					if (!safeHref(unquoted)) continue;
				}
				safeAttrs.push(` ${name}=${value}`);
			}
			ensureNoopenerRel(lower, safeAttrs);
			return `<${lower}${safeAttrs.join('')}>`;
		}
	);

	// Ref: japaconnect-engineering/sections/23-main-site-frontend.md
	if (!/<div[^>]*\bclass\s*=\s*["'][^"']*\bprose-table-wrapper\b[^"']*["']/i.test(out)) {
		out = out.replace(/(<table\b[\s\S]*?<\/table>)/gi, '<div class="prose-table-wrapper">$1</div>');
	}

	return out;
}
