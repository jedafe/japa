import { generateHTML } from '@tiptap/html';
import StarterKit from '@tiptap/starter-kit';
import Link from '@tiptap/extension-link';
import { Table } from '@tiptap/extension-table';
import TableRow from '@tiptap/extension-table-row';
import TableCell from '@tiptap/extension-table-cell';
import TableHeader from '@tiptap/extension-table-header';
import TaskList from '@tiptap/extension-task-list';
import TaskItem from '@tiptap/extension-task-item';

/**
 * Extensions shared between the docs prose renderer and the editor (admin).
 * Kept in sync with the backend's stored TipTap JSON shape (Section 44).
 */
const extensions = [
	StarterKit,
	Link,
	Table,
	TableRow,
	TableCell,
	TableHeader,
	TaskList,
	TaskItem
];

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
	if (!content || typeof content !== 'object') return '';
	try {
		return generateHTML(content as never, extensions);
	} catch {
		// Malformed TipTap JSON should never blank the page — degrade to empty.
		return '';
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
