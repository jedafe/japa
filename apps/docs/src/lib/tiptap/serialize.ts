import { generateHTML } from '@tiptap/html';
import { marked } from 'marked';
import { docExtensions } from './extensions';
import { sanitizeContentHtml } from '$lib/utils/safeHtml';

/**
 * Extensions shared between the docs prose renderer and the editor (admin).
 * Kept in sync with the backend's stored TipTap JSON shape (Section 44).
 */
const extensions = docExtensions;

/**
 * Render a stored TipTap JSON document to HTML on the server.
 *
 * Used by +page.server.ts load functions so the .prose block receives
 * ready HTML (no client-side TipTap runtime needed for reading). Returns
 * an empty string for null/empty/non-object content.
 *
 * Browser-guarded: @tiptap/html is safe in SSR (no DOM access), but we
 * only ever call this from server load functions regardless.
 */
export function renderDocContent(content: unknown): string {
	if (!content) return '';

	// doc_pages.content is jsonb — besides the canonical TipTap document
	// object, direct API writes and older editors can store a plain string
	// (markdown, HTML, or raw text). Rendering a string to nothing leaves a
	// blank article body, so fall back to markdown/HTML rendering instead.
	if (typeof content === 'string') {
		return renderStringContent(content);
	}
	if (typeof content !== 'object') return '';
	try {
		return generateHTML(content as never, extensions);
	} catch {
		// Malformed TipTap JSON should never blank the page — degrade to empty.
		return '';
	}
}

function escapeHtmlText(text: string): string {
	return text
		.replace(/&/g, '&amp;')
		.replace(/</g, '&lt;')
		.replace(/>/g, '&gt;')
		.replace(/"/g, '&quot;')
		.replace(/'/g, '&#039;');
}

/**
 * Render a string body: a JSON-encoded TipTap doc is parsed and delegated to
 * generateHTML; markdown/plain text goes through marked; anything else is
 * escaped as a paragraph. Output is always sanitized before rendering.
 */
function renderStringContent(content: string): string {
	const trimmed = content.trim();
	if (!trimmed) return '';

	// A JSON-encoded document string (e.g. content stored as a quoted string
	// of a TipTap doc) — parse and render it as a document object.
	if (trimmed.startsWith('{') || trimmed.startsWith('[')) {
		try {
			const parsed: unknown = JSON.parse(trimmed);
			if (parsed && typeof parsed === 'object') {
				return generateHTML(parsed as never, extensions);
			}
		} catch {
			// Not valid JSON — fall through to markdown.
		}
	}

	try {
		const html = marked.parse(trimmed, { async: false });
		return sanitizeContentHtml(html);
	} catch {
		return `<p>${escapeHtmlText(trimmed)}</p>`;
	}
}

/** A heading extracted from the rendered HTML for the in-page TOC. */
export interface TocHeading {
	id: string;
	text: string;
	level: 2 | 3;
}

/**
 * Extract H2/H3 headings (with ids) from rendered doc HTML for the right
 * sidebar TOC. Runs server-side so the TOC paints during SSR; the
 * IntersectionObserver on the client only handles active-highlighting.
 *
 * Headings without an id get a slugified one injected so anchor links work.
 */
export function extractTocHeadings(html: string): TocHeading[] {
	if (!html) return [];
	const headings: TocHeading[] = [];
	// Match <h2>...</h2> and <h3>...</h3> (case-insensitive, non-greedy).
	const re = /<h([23])\b([^>]*)>(.*?)<\/h\1>/gis;
	let m: RegExpExecArray | null;
	const seen = new Map<string, number>();
	while ((m = re.exec(html)) !== null) {
		const level = Number(m[1]) as 2 | 3;
		const attrs = m[2];
		const inner = m[3];
		const text = inner.replace(/<[^>]+>/g, '').trim();
		if (!text) continue;
		let id = /id="([^"]+)"/i.exec(attrs)?.[1] ?? slugify(text);
		// De-duplicate ids.
		if (seen.has(id)) {
			const n = seen.get(id)! + 1;
			seen.set(id, n);
			id = `${id}-${n}`;
		} else {
			seen.set(id, 1);
		}
		headings.push({ id, text, level });
	}
	return headings;
}

/**
 * Inject a hover-visible permalink (`#` link) inside each h2/h3 — the modern
 * docs convention. Must run AFTER extractTocHeadings: the anchor text would
 * otherwise leak a trailing '#' into the TOC label.
 */
export function injectHeadingAnchors(html: string): string {
	if (!html) return '';
	return html.replace(/<h([23])\b([^>]*)>(.*?)<\/h\1>/gis, (full, level, attrs, inner) => {
		const id = /id="([^"]+)"/i.exec(attrs)?.[1];
		if (!id) return full;
		return `<h${level}${attrs}>${inner}<a class="heading-anchor" href="#${id}" aria-hidden="true" tabindex="-1">#</a></h${level}>`;
	});
}

/** Inject/normalize id attributes on h2/h3 in the rendered HTML so anchor links resolve. */
export function ensureHeadingIds(html: string): string {
	if (!html) return '';
	const seen = new Map<string, number>();
	return html.replace(/<h([23])\b([^>]*)>(.*?)<\/h\1>/gis, (_full, level, attrs, inner) => {
		const text = inner.replace(/<[^>]+>/g, '').trim();
		if (!text) return _full;
		const existingId = /id="([^"]+)"/i.exec(attrs)?.[1];
		let id = existingId ?? slugify(text);
		if (seen.has(id)) {
			const n = seen.get(id)! + 1;
			seen.set(id, n);
			id = `${id}-${n}`;
		} else {
			seen.set(id, 1);
		}
		// Strip any existing id attribute to avoid duplicates, then add ours.
		const cleanedAttrs = attrs.replace(/\s*id="[^"]*"/i, '');
		return `<h${level}${cleanedAttrs} id="${id}">${inner}</h${level}>`;
	});
}

function slugify(text: string): string {
	return text
		.toLowerCase()
		.replace(/[^\w\s-]/g, '')
		.replace(/\s+/g, '-')
		.replace(/-+/g, '-')
		.replace(/^-|-$/g, '')
		.slice(0, 60) || 'section';
}
