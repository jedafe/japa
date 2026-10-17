/**
 * Sanitize an HTML string for safe inline rendering via `{@html}`.
 *
 * Parser-based allowlist matching apps/control/src/lib/utils.ts: the input is
 * parsed with DOMParser and only allowlisted elements/attributes survive.
 * Script-style containers (script, style, iframe, object, embed, form, svg,
 * math, template, noscript) lose their entire subtree so payloads never
 * survive as text content either. URL-bearing attributes reject dangerous
 * schemes (javascript:, data:, vbscript:) — relative / http(s) / mailto
 * links only. `style` attributes survive only when free of executable or
 * remote-loading content (url(, expression(, @import, javascript:, etc.).
 *
 * style/button/svg do NOT survive: DataTable cell renders and page-builder
 * previews only need lightweight inline formatting (span, div, b, i, strong,
 * em, code, a, br, p, h3, img). Dropping them closes the
 * `<svg><animate onbegin>` / `<button formaction="javascript:…">` vectors
 * the previous regex pass left open.
 *
 * Used for DataTable cell renders and page-builder previews where the source
 * is user/admin content but we only want lightweight, non-executable markup.
 */

const ALLOWED_TAGS = new Set([
	'div', 'span', 'p', 'b', 'i', 'strong', 'em', 'code', 'a',
	'br', 'h3', 'img'
]);

// Elements whose entire subtree is unsafe/unwanted — dropped wholesale so
// script/style payloads never survive as text content either.
const DROP_SUBTREE = new Set([
	'script', 'style', 'iframe', 'object', 'embed', 'template', 'noscript',
	'svg', 'math', 'form', 'link', 'meta', 'button', 'input', 'select',
	'textarea', 'video', 'audio', 'source', 'track', 'canvas'
]);

const GLOBAL_ATTRS = new Set(['class', 'title']);
const ANCHOR_ATTRS = new Set(['class', 'title', 'href', 'target', 'rel']);
const IMG_ATTRS = new Set(['class', 'title', 'src', 'alt']);

// style is needed for progress-bar style renders (width: N%). Any style
// value that could execute code or load remote content rejects the whole
// attribute rather than attempting a property-level allowlist.
const UNSAFE_STYLE =
	/url\s*\(|expression\s*\(|@import|@charset|javascript|vbscript|behavior\s*:|binding\s*:|-moz-binding|position\s*:/i;

function isSafeHref(href: string): boolean {
	const v = href.trim().toLowerCase();
	if (v === '') return true;
	// Reject protocol-relative URLs (//evil.com inherits the page scheme).
	if (v.startsWith('//')) return false;
	if (v.startsWith('#') || v.startsWith('/') || v.startsWith('./') || v.startsWith('../')) return true;
	return /^(https?|mailto):/i.test(v);
}

function isSafeSrc(src: string): boolean {
	const v = src.trim().toLowerCase();
	if (v === '') return true;
	// Reject protocol-relative URLs (//evil.com/img.png tracking pixel).
	if (v.startsWith('//')) return false;
	if (v.startsWith('/') || v.startsWith('./') || v.startsWith('../')) return true;
	return /^https?:/i.test(v);
}

/**
 * Escape a string for safe insertion into HTML text content or an attribute
 * value. Use this when interpolating user/admin data into a render template
 * literal that will be passed to `{@html}`.
 */
/**
 * Guard a URL for safe use in an `href` binding. Returns the URL when it is
 * a safe scheme (http(s), mailto, or a site-relative path); returns '#' for
 * `javascript:`/`data:`/`vbscript:`/protocol-relative values so a stored
 * payload can never execute on click.
 */
export function safeHref(url: string | null | undefined): string {
	if (!url || typeof url !== 'string') return '#';
	return isSafeHref(url) ? url : '#';
}

export function escapeHtml(s: string): string {
	return String(s ?? '')
		.replace(/&/g, '&amp;')
		.replace(/</g, '&lt;')
		.replace(/>/g, '&gt;')
		.replace(/"/g, '&quot;')
		.replace(/'/g, '&#39;');
}

function sanitizeElement(el: Element): string {
	const tag = el.tagName.toLowerCase();
	if (DROP_SUBTREE.has(tag)) return '';

	const inner = Array.from(el.childNodes).map(sanitizeNode).join('');

	if (!ALLOWED_TAGS.has(tag)) {
		// Unknown tag — keep the (sanitized) children so textual content
		// survives, only the wrapper is dropped.
		return inner;
	}

	const attrs: string[] = [];
	const allowed = tag === 'a' ? ANCHOR_ATTRS : tag === 'img' ? IMG_ATTRS : GLOBAL_ATTRS;
	for (const attr of Array.from(el.attributes)) {
		const name = attr.name.toLowerCase();
		if (name.startsWith('on')) continue;
		if (name === 'style') {
			if (!UNSAFE_STYLE.test(attr.value)) {
				attrs.push(`style="${escapeHtml(attr.value)}"`);
			}
			continue;
		}
		if (!allowed.has(name)) continue;
		if (name === 'href' && !isSafeHref(attr.value)) continue;
		if (name === 'src' && !isSafeSrc(attr.value)) continue;
		if (name === 'target' && attr.value !== '_blank' && attr.value !== '_self') continue;
		attrs.push(`${name}="${escapeHtml(attr.value)}"`);
	}
	// target="_blank" must never keep a window.opener back-reference.
	if (tag === 'a' && attrs.some((a) => a.startsWith('target="_blank"'))) {
		attrs.push('rel="noopener noreferrer"');
	}
	if (tag === 'img') {
		return `<img${attrs.length ? ' ' + attrs.join(' ') : ''}>`;
	}
	return `<${tag}${attrs.length ? ' ' + attrs.join(' ') : ''}>${inner}</${tag}>`;
}

function sanitizeNode(node: Node): string {
	if (node.nodeType === Node.TEXT_NODE) {
		return escapeHtml(node.textContent ?? '');
	}
	if (node.nodeType === Node.ELEMENT_NODE) {
		return sanitizeElement(node as Element);
	}
	return '';
}

export function sanitizeHtml(html: string): string {
	if (!html) return '';
	// SSR safety: guard so a stray server call degrades to fully-escaped
	// text instead of throwing (DOMParser is browser-only).
	if (typeof DOMParser === 'undefined') return escapeHtml(html);
	const doc = new DOMParser().parseFromString(`<div>${html}</div>`, 'text/html');
	const root = doc.body.firstElementChild;
	if (!root) return escapeHtml(html);
	return Array.from(root.childNodes).map(sanitizeNode).join('');
}

// Email-layout allowlist for newsletter campaign HTML: tables, headings,
// lists and inline images on top of the base inline set. Same engine as
// sanitizeHtml — script-style containers still lose their whole subtree,
// URL attrs still reject dangerous schemes, and `style` still drops the
// whole attribute on any executable/remote/overlay value.
const EMAIL_TAGS = new Set([
	...ALLOWED_TAGS,
	'h1', 'h2', 'h4', 'h5', 'h6', 'ul', 'ol', 'li', 'blockquote',
	'table', 'thead', 'tbody', 'tfoot', 'tr', 'th', 'td', 'hr', 'u', 's', 'sub', 'sup'
]);

const EMAIL_ATTRS: Record<string, Set<string>> = {
	a: new Set(['class', 'title', 'href', 'target', 'rel', 'style']),
	img: new Set(['class', 'title', 'src', 'alt', 'width', 'height', 'style']),
	table: new Set(['class', 'style', 'cellpadding', 'cellspacing', 'border', 'width', 'align']),
	td: new Set(['class', 'style', 'colspan', 'rowspan', 'align', 'valign', 'width']),
	th: new Set(['class', 'style', 'colspan', 'rowspan', 'align', 'valign', 'width'])
};

function sanitizeEmailElement(el: Element): string {
	const tag = el.tagName.toLowerCase();
	if (DROP_SUBTREE.has(tag)) return '';

	const inner = Array.from(el.childNodes).map(sanitizeEmailNode).join('');

	if (!EMAIL_TAGS.has(tag)) return inner;

	const attrs: string[] = [];
	const allowed = EMAIL_ATTRS[tag] ?? (tag === 'a' ? ANCHOR_ATTRS : tag === 'img' ? IMG_ATTRS : GLOBAL_ATTRS);
	for (const attr of Array.from(el.attributes)) {
		const name = attr.name.toLowerCase();
		if (name.startsWith('on')) continue;
		if (name === 'style') {
			if (!UNSAFE_STYLE.test(attr.value)) {
				attrs.push(`style="${escapeHtml(attr.value)}"`);
			}
			continue;
		}
		if (!allowed.has(name)) continue;
		if (name === 'href' && !isSafeHref(attr.value)) continue;
		if (name === 'src' && !isSafeSrc(attr.value)) continue;
		if (name === 'target' && attr.value !== '_blank' && attr.value !== '_self') continue;
		attrs.push(`${name}="${escapeHtml(attr.value)}"`);
	}
	if (tag === 'a' && attrs.some((a) => a.startsWith('target="_blank"'))) {
		attrs.push('rel="noopener noreferrer"');
	}
	if (tag === 'img') {
		return `<img${attrs.length ? ' ' + attrs.join(' ') : ''}>`;
	}
	return `<${tag}${attrs.length ? ' ' + attrs.join(' ') : ''}>${inner}</${tag}>`;
}

function sanitizeEmailNode(node: Node): string {
	if (node.nodeType === Node.TEXT_NODE) {
		return escapeHtml(node.textContent ?? '');
	}
	if (node.nodeType === Node.ELEMENT_NODE) {
		return sanitizeEmailElement(node as Element);
	}
	return '';
}

/**
 * Sanitize newsletter campaign HTML before it is stored/sent.
 *
 * Campaign HTML is mailed verbatim to subscribers, so this is the last
 * XSS gate — strictly stronger than the regex strip it replaces
 * (entity-encoded schemes, unquoted handlers, nested payloads). Email
 * layout tags (table/headings/lists) survive; script-style containers,
 * forms, iframes, event handlers and dangerous URL schemes do not.
 */
export function sanitizeEmailHtml(html: string): string {
	if (!html) return '';
	if (typeof DOMParser === 'undefined') return escapeHtml(html);
	const doc = new DOMParser().parseFromString(`<div>${html}</div>`, 'text/html');
	const root = doc.body.firstElementChild;
	if (!root) return escapeHtml(html);
	return Array.from(root.childNodes).map(sanitizeEmailNode).join('');
}
