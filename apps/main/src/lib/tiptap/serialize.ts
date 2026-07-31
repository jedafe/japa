/**
 * SSR-safe TipTap JSON utilities — no DOM dependency.
 * Ref: japaconnect-engineering/sections/15-tiptap-rich-text.md
 *
 * The forum backend stores bodies as base64-encoded TipTap JSON documents
 * (unlike the blog backend, which pre-renders `body_html`). These helpers
 * decode and serialize that shape server-side so forum pages can SSR full
 * article HTML and plain-text excerpts without a ProseMirror/DOM runtime.
 */

/* ------------------------------------------------------------------ *
 * TipTap / ProseMirror node shape
 * ------------------------------------------------------------------ */

interface PMMark {
	type: string;
	attrs?: Record<string, unknown>;
}

interface PMNode {
	type: string;
	attrs?: Record<string, unknown>;
	content?: PMNode[];
	marks?: PMMark[];
	text?: string;
}

/** A decoded TipTap document is a top-level node whose `content` holds blocks. */
export type TipTapDoc = PMNode;

/* ------------------------------------------------------------------ *
 * Base64 decode (works in Node, Bun, and the browser)
 * ------------------------------------------------------------------ */

/** Decode a base64 string to UTF-8 text. Returns '' on failure. */
export function decodeBase64Text(b64: string): string {
	try {
		// atob is global in browsers and Node 16+/Bun; fall back to Buffer.
		if (typeof atob === 'function') {
			const binary = atob(b64);
			// Convert binary string to UTF-8.
			const bytes = Uint8Array.from(binary, (c) => c.charCodeAt(0));
			return new TextDecoder('utf-8').decode(bytes);
		}
		// Node/Bun server path.
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const g = globalThis as any;
		if (g.Buffer) return g.Buffer.from(b64, 'base64').toString('utf-8');
	} catch {
		/* fall through */
	}
	return '';
}

/** Parse a base64-encoded TipTap JSON document. Returns null if invalid.
 * The backend may store either a full `{type:"doc",content:[...]}` document
 * or a bare array of nodes; both are returned for the caller to handle. */
export function decodeTipTapDoc(b64: string | null | undefined): TipTapDoc | PMNode[] | null {
	if (!b64) return null;
	const text = decodeBase64Text(b64);
	if (!text) return null;
	try {
		return JSON.parse(text) as TipTapDoc | PMNode[];
	} catch {
		return null;
	}
}

/* ------------------------------------------------------------------ *
 * Plain-text extraction (for excerpts / previews)
 * ------------------------------------------------------------------ */

/** Walk a TipTap node tree and concatenate all text content. */
export function tiptapToText(doc: TipTapDoc | PMNode[] | null): string {
	if (!doc) return '';
	let out = '';
	const visit = (node: PMNode): void => {
		if (typeof node.text === 'string') out += node.text;
		if (node.content) for (const child of node.content) visit(child);
	};
	// The backend may store the body as a bare array of nodes (no `doc` wrapper)
	// or as a full TipTap document. Handle both.
	if (Array.isArray(doc)) {
		for (const node of doc) visit(node);
	} else {
		visit(doc);
	}
	return out.replace(/\s+/g, ' ').trim();
}

/** Decode a base64 TipTap body and return a truncated plain-text excerpt. */
export function tiptapExcerpt(b64: string | null | undefined, max = 180): string {
	const text = tiptapToText(decodeTipTapDoc(b64));
	if (text.length <= max) return text;
	return text.slice(0, max).trimEnd() + '…';
}

/* ------------------------------------------------------------------ *
 * HTML serialization
 * ------------------------------------------------------------------ */

const escapeHtml = (s: string): string =>
	s
		.replace(/&/g, '&amp;')
		.replace(/</g, '&lt;')
		.replace(/>/g, '&gt;')
		.replace(/"/g, '&quot;');

const escapeAttr = (s: string): string => s.replace(/"/g, '&quot;');

/** Render inline marks (bold/italic/code/link) around a text node. */
function renderMarks(text: string, marks: PMMark[] | undefined): string {
	let out = escapeHtml(text);
	if (!marks) return out;
	for (const mark of marks) {
		switch (mark.type) {
			case 'bold':
				out = `<strong>${out}</strong>`;
				break;
			case 'italic':
				out = `<em>${out}</em>`;
				break;
			case 'code':
				out = `<code>${out}</code>`;
				break;
			case 'strike':
				out = `<s>${out}</s>`;
				break;
			case 'link': {
				const href = String(mark.attrs?.href ?? '#');
				const target = mark.attrs?.target ? ` target="${escapeAttr(String(mark.attrs.target))}"` : '';
				const rel = ` rel="noopener noreferrer"`;
				out = `<a href="${escapeAttr(href)}"${target}${rel}>${out}</a>`;
				break;
			}
		}
	}
	return out;
}

/** Serialize a single TipTap node to an HTML string.
 * @param deleted Optional set of erased user IDs — mentions of these
 *   users render as `@[deleted user]` (Section 15 / ADR-029).
 */
function renderNode(node: PMNode, deleted?: Set<string>): string {
	// Text node — apply marks.
	if (typeof node.text === 'string') {
		return renderMarks(node.text, node.marks);
	}

	const inner = (node.content ?? []).map((n) => renderNode(n, deleted)).join('');

	switch (node.type) {
		case 'doc':
			return inner;
		case 'paragraph':
			return `<p>${inner}</p>`;
		case 'heading': {
			const level = Number(node.attrs?.level ?? 2);
			const tag = level >= 1 && level <= 6 ? `h${level}` : 'h2';
			return `<${tag}>${inner}</${tag}>`;
		}
		case 'bulletList':
			return `<ul>${inner}</ul>`;
		case 'orderedList':
			return `<ol>${inner}</ol>`;
		case 'listItem':
			return `<li>${inner}</li>`;
		case 'taskList':
			return `<ul data-type="taskList">${inner}</ul>`;
		case 'taskItem': {
			const checked = node.attrs?.checked ? ' checked' : '';
			return `<li><label><input type="checkbox"${checked} disabled /> </label><div>${inner}</div></li>`;
		}
		case 'blockquote':
			return `<blockquote>${inner}</blockquote>`;
		case 'codeBlock': {
			const lang = node.attrs?.language ? ` data-language="${escapeAttr(String(node.attrs.language))}"` : '';
			return `<pre${lang}><code>${inner}</code></pre>`;
		}
		case 'hardBreak':
			return '<br />';
		case 'horizontalRule':
			return '<hr />';
		case 'image': {
			const src = escapeAttr(String(node.attrs?.src ?? ''));
			const alt = escapeAttr(String(node.attrs?.alt ?? ''));
			return `<img src="${src}" alt="${alt}" loading="lazy" />`;
		}
		case 'mention': {
			// Section 15 / ADR-029: a mention of an erased user renders as
			// `@[deleted user]` with no link.
			const mentionId = String(node.attrs?.id ?? '');
			if (deleted && deleted.has(mentionId)) {
				return '<span class="mention mention-deleted">@[deleted user]</span>';
			}
			const label = escapeHtml(String(node.attrs?.label ?? ''));
			return `<span class="mention" data-user-id="${escapeAttr(mentionId)}">${label}</span>`;
		}
		case 'hashtag': {
			const label = escapeHtml(String(node.attrs?.label ?? ''));
			return `<span class="hashtag">${label}</span>`;
		}
		case 'interlink': {
			const label = escapeHtml(String(node.attrs?.label ?? ''));
			return `<span class="interlink">${label}</span>`;
		}
		case 'table':
			return `<div class="prose-table-wrapper"><table>${inner}</table></div>`;
		case 'tableRow':
			return `<tr>${inner}</tr>`;
		case 'tableCell':
			return `<td>${inner}</td>`;
		case 'tableHeader':
			return `<th>${inner}</th>`;
		default:
			// Unknown node — render its children inline if any, else skip.
			return inner;
	}
}

/** Serialize a TipTap document to an HTML string (SSR-safe, no DOM).
 *
 * @param doc The TipTap document (or bare array of nodes).
 * @param opts Optional render-time context:
 *   - deletedUserIds: Set of user UUIDs whose accounts have been erased.
 *     Mentions of these users render as `@[deleted user]` with no link
 *     (Section 15 / ADR-029).
 */
export function tiptapToHtml(
	doc: TipTapDoc | PMNode[] | null,
	opts?: { deletedUserIds?: Set<string> }
): string {
	if (!doc) return '';
	const deleted = opts?.deletedUserIds;
	// The backend may store the body as a bare array of nodes (no `doc` wrapper)
	// or as a full TipTap document. Handle both.
	if (Array.isArray(doc)) {
		return doc.map((n) => renderNode(n, deleted)).join('');
	}
	return renderNode(doc, deleted);
}

/** Decode a base64 TipTap body and serialize to HTML in one call.
 * @param opts Optional render-time context (see tiptapToHtml).
 */
export function tiptapBodyToHtml(
	b64: string | null | undefined,
	opts?: { deletedUserIds?: Set<string> }
): string {
	return tiptapToHtml(decodeTipTapDoc(b64), opts);
}

/* ------------------------------------------------------------------ *
 * Encode (plain-text reply composer → base64 TipTap JSON)
 * The forum reply endpoint expects `body` as a base64 string of a
 * TipTap JSON document. The compact reply composer produces a single
 * paragraph; this wraps the text and base64-encodes it for transport.
 * ------------------------------------------------------------------ */

/** Base64-encode a UTF-8 string. Works in Node, Bun, and the browser. */
function encodeBase64Text(text: string): string {
	try {
		if (typeof btoa === 'function') {
			const bytes = new TextEncoder().encode(text);
			let binary = '';
			for (let i = 0; i < bytes.length; i++) binary += String.fromCharCode(bytes[i]);
			return btoa(binary);
		}
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const g = globalThis as any;
		if (g.Buffer) return g.Buffer.from(text, 'utf-8').toString('base64');
	} catch {
		/* fall through */
	}
	return '';
}

/**
 * Wrap a plain-text reply string into a minimal TipTap paragraph document
 * and base64-encode it for POST /forum/{id}/replies. Empty input yields an
 * empty paragraph so the backend always receives a valid document.
 */
export function encodeReplyBody(text: string): string {
	const cleaned = text.replace(/\r\n/g, '\n');
	const paragraphs = cleaned.split('\n').map((line) => ({
		type: 'paragraph',
		content: line.length ? [{ type: 'text', text: line }] : []
	}));
	const doc: TipTapDoc = { type: 'doc', content: paragraphs };
	return encodeBase64Text(JSON.stringify(doc));
}
