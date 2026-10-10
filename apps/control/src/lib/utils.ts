export function escapeHtml(str: unknown): string {
	if (str == null) return '';
	return String(str)
		.replace(/&/g, '&amp;')
		.replace(/</g, '&lt;')
		.replace(/>/g, '&gt;')
		.replace(/"/g, '&quot;')
		.replace(/'/g, '&#39;');
}

// Tags permitted in DataTable cell render output: lightweight inline
// formatting plus the block anchors the cell renderers actually emit
// (progress bars, truncated spans, links).
const SANITIZE_ALLOWED_TAGS = new Set([
	'span', 'div', 'b', 'i', 'strong', 'em', 'code', 'a', 'br', 'p'
]);

// Elements whose entire subtree is unsafe/unwanted — dropped wholesale so
// script/style payloads never survive as text content either.
const SANITIZE_DROP_SUBTREE = new Set([
	'script', 'style', 'iframe', 'object', 'embed', 'template', 'noscript', 'svg', 'math', 'form'
]);

const GLOBAL_ATTRS = new Set(['class', 'title']);
const ANCHOR_ATTRS = new Set(['class', 'title', 'href', 'target', 'rel']);

// style is needed for the cache-page progress bars (width: N%). Any style
// value that could execute code or load remote content rejects the whole
// attribute rather than attempting a property-level allowlist.
const UNSAFE_STYLE = /url\s*\(|expression\s*\(|@import|@charset|javascript|vbscript|behavior\s*:|binding\s*:|position\s*:/i;

function isSafeHref(href: string): boolean {
	const v = href.trim().toLowerCase();
	if (v === '') return true;
	// Reject protocol-relative URLs (//evil.com inherits the page scheme).
	if (v.startsWith('//')) return false;
	if (v.startsWith('#') || v.startsWith('/') || v.startsWith('./') || v.startsWith('../')) return true;
	return /^(https?|mailto):/i.test(v);
}

function sanitizeElement(el: Element): string {
	const tag = el.tagName.toLowerCase();
	if (SANITIZE_DROP_SUBTREE.has(tag)) return '';

	const inner = Array.from(el.childNodes).map(sanitizeNode).join('');

	if (!SANITIZE_ALLOWED_TAGS.has(tag)) {
		// Unknown tag — keep the (sanitized) children so textual content
		// survives, only the wrapper is dropped.
		return inner;
	}

	const attrs: string[] = [];
	const allowed = tag === 'a' ? ANCHOR_ATTRS : GLOBAL_ATTRS;
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
		if (name === 'target' && attr.value !== '_blank' && attr.value !== '_self') continue;
		attrs.push(`${name}="${escapeHtml(attr.value)}"`);
	}
	// target="_blank" must never keep a window.opener back-reference.
	if (tag === 'a' && attrs.some((a) => a.startsWith('target="_blank"'))) {
		attrs.push('rel="noopener noreferrer"');
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

/**
 * Whitelist-sanitize HTML produced by DataTable column render functions.
 * The previous regex pass-through left non-whitelisted markup untouched,
 * so a hostile payload (e.g. `<img src=x onerror=…>` smuggled in API data)
 * reached {@html} rendering intact. This parser-based pass drops every
 * disallowed element/attribute; script-style containers lose their
 * subtree entirely.
 */
export function sanitizeHtml(html: string): string {
	if (!html) return '';
	// SSR safety: the control panel is a client-rendered SPA, but guard so
	// a stray server call degrades to fully-escaped text instead of throwing.
	if (typeof DOMParser === 'undefined') return escapeHtml(html);
	const doc = new DOMParser().parseFromString(`<div>${html}</div>`, 'text/html');
	const root = doc.body.firstElementChild;
	if (!root) return escapeHtml(html);
	return Array.from(root.childNodes).map(sanitizeNode).join('');
}
