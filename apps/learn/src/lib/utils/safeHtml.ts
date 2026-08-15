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
				// Allow data-* attributes.
				if (!ALLOWED_ATTRS.has(name) && !name.startsWith('data-')) continue;
				// Guard URL-bearing attributes against dangerous schemes.
				if (name === 'src' || name === 'href' || name === 'xlink:href') {
					const unquoted = value.replace(/^["']|["']$/g, '').trim();
					if (isDangerousUrl(unquoted)) continue;
				}
				safeAttrs.push(` ${name}=${value}`);
			}
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
	if (isDangerousUrl(trimmed)) return '';
	return trimmed;
}

function isDangerousUrl(url: string): boolean {
	// Match `javascript:`, `data:`, `vbscript:` with optional whitespace/control chars.
	return /^\s*(javascript|data|vbscript)\s*:/i.test(url);
}
