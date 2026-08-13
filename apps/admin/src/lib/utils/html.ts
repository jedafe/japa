/**
 * Sanitize an HTML string for safe inline rendering via `{@html}`.
 *
 * Strips `<script>`/`<style>` blocks entirely, removes `on*` event-handler
 * attributes and `javascript:` URIs from every tag, and restricts the remaining
 * tags to a small allowlist of structural + formatting elements (`div`, `span`,
 * `p`, `b`, `i`, `strong`, `em`, `code`, `button`, `svg`, `rect`, `path`,
 * `line`, `polyline`, `h3`, `img`). Only `class`, `style`, `title`,
 * `d`, `x`, `y`, `width`, `height`, `viewBox`, `fill`, `stroke`,
 * `stroke-width`, `stroke-linecap`, `stroke-linejoin`, `rx`, `ry`,
 * `xmlns`, `points`, `src`, `alt` attributes survive on those tags.
 *
 * Used for DataTable cell renders and page-builder previews where the source
 * is user/admin content but we only want lightweight, non-executable markup.
 */
const ALLOWED_TAGS = new Set([
	'div', 'span', 'p', 'b', 'i', 'strong', 'em', 'code', 'button', 'a',
	'svg', 'rect', 'path', 'line', 'polyline', 'h3', 'img'
]);
const ALLOWED_ATTRS = new Set([
	'class', 'style', 'title', 'href',
	'd', 'x', 'y', 'width', 'height', 'viewBox', 'fill', 'stroke',
	'stroke-width', 'stroke-linecap', 'stroke-linejoin', 'rx', 'ry',
	'xmlns', 'points', 'src', 'alt'
]);

/**
 * Escape a string for safe insertion into HTML text content or an attribute
 * value. Use this when interpolating user/admin data into a render template
 * literal that will be passed to `{@html}`.
 */
export function escapeHtml(s: string): string {
	return s
		.replace(/&/g, '&amp;')
		.replace(/</g, '&lt;')
		.replace(/>/g, '&gt;')
		.replace(/"/g, '&quot;')
		.replace(/'/g, '&#39;');
}

export function sanitizeHtml(html: string): string {
	// 1. Drop <script> / <style> blocks (including their content).
	let out = html.replace(
		/<(script|style)\b[^>]*>[\s\S]*?<\/\1\s*>/gi,
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
			// Keep only allowlisted attributes, dropping on* handlers and
			// javascript: URIs defensively.
			const safeAttrs: string[] = [];
			const attrRe = /\s+([a-zA-Z_:][a-zA-Z0-9_.:-]*)\s*=\s*("[^"]*"|'[^']*'|\S+)/g;
			let m: RegExpExecArray | null;
			while ((m = attrRe.exec(attrs)) !== null) {
				const name = m[1].toLowerCase();
				const value = m[2];
				if (!ALLOWED_ATTRS.has(name)) continue;
				if (name === 'src' || name === 'href') {
					const unquoted = value.replace(/^["']|["']$/g, '').trim();
					if (/^\s*javascript:/i.test(unquoted)) continue;
				}
				safeAttrs.push(` ${name}=${value}`);
			}
			return `<${lower}${safeAttrs.join('')}>`;
		}
	);
	return out;
}
