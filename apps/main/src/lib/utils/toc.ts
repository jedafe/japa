/**
 * Table-of-contents extraction for article/wiki/published pages.
 *
 * Two entry points:
 *   - extractToc(html): pure string transform (SSR-safe, same technique as
 *     InArticleAd) — injects slugified `id`s into <h2>/<h3> headings and
 *     returns the rewritten HTML plus the ordered item list. Deterministic,
 *     so server and client output match (no hydration drift).
 *   - scanDomToc(root): client-side pass for render trees we don't own as
 *     strings (Page Builder LandingRenderer blocks) — assigns ids to
 *     id-less headings in the live DOM.
 *
 * Per-page opt-out: tocHidden(source) reads the documented flags —
 * `hide_toc: true` or `toc: false` on the entity itself, its `meta` bag
 * (Page Builder pages) or its `field_values` bag (content custom fields).
 * Absent flags mean the TOC is on.
 */
import { goto } from '$app/navigation';
import { page } from '$app/state';

export interface TocItem {
	/** Element id the TOC link targets (`#id`). */
	id: string;
	/** Heading text (tags stripped, entities decoded). */
	text: string;
	/** Heading level — 2 for h2, 3 for h3. */
	level: number;
}

const HEADING_RE = /<(h[2-4])([^>]*)>([\s\S]*?)<\/\1>/gi;
const ID_ATTR_RE = /\bid\s*=\s*(["'])(.*?)\1/i;
const TAG_RE = /<[^>]*>/g;

const ENTITIES: Record<string, string> = {
	amp: '&',
	lt: '<',
	gt: '>',
	quot: '"',
	apos: "'",
	nbsp: ' ',
	'#39': "'"
};

function decodeEntities(s: string): string {
	return s.replace(/&(#x?[0-9a-f]+|\w+);/gi, (m, e: string) => {
		if (e.startsWith('#')) {
			const n = e[1]?.toLowerCase() === 'x' ? parseInt(e.slice(2), 16) : parseInt(e.slice(1), 10);
			return Number.isFinite(n) ? String.fromCodePoint(n) : m;
		}
		return ENTITIES[e.toLowerCase()] ?? m;
	});
}

/** Unicode-aware slug: letters/numbers kept, everything else collapses to '-'. */
export function slugifyHeading(text: string): string {
	return text
		.toLowerCase()
		.replace(/[^\p{L}\p{N}]+/gu, '-')
		.replace(/^-+|-+$/g, '');
}

/**
 * Leading enumerator authors bake into headings — "1. Foo", "2) Bar",
 * "• Baz", "- Qux", "A. Lettered" — is redundant next to the TOC's own
 * dot marker, so it's stripped from the item label (and thus the slug).
 * Deliberately narrow: requires trailing whitespace and won't touch
 * "2024 outlook" (>2 digits), "U.S. relations" (no space after `U.`),
 * or "Section 2: things" (word first).
 */
const PREFIX_RE = /^\s*(?:[-*•·▪◦‣»–—]|\d{1,2}[.)]|[a-zA-Z][.)])\s+/;

export function stripItemPrefix(text: string): string {
	return text.replace(PREFIX_RE, '').trim();
}

function headingText(inner: string): string {
	return stripItemPrefix(decodeEntities(inner.replace(TAG_RE, '')).replace(/\s+/g, ' ').trim());
}

function uniqueSlug(base: string, taken: (id: string) => boolean): string {
	let id = base;
	let n = 2;
	while (taken(id)) id = `${base}-${n++}`;
	return id;
}

/**
 * Inject `id`s into <h2>/<h3> headings (h4 opt-in via `levels`) and return
 * the rewritten HTML + ordered TOC items. Headings with an existing id are
 * kept as-is and listed; empty headings are skipped entirely.
 */
export function extractToc(html: string, opts?: { levels?: number[] }): { html: string; items: TocItem[] } {
	const levels = new Set(opts?.levels ?? [2, 3]);
	const items: TocItem[] = [];
	const used = new Set<string>();
	const out = html.replace(HEADING_RE, (whole: string, tag: string, attrs: string, inner: string) => {
		const level = Number(tag[1]);
		if (!levels.has(level)) return whole;
		const text = headingText(inner);
		if (!text) return whole;
		let id = attrs.match(ID_ATTR_RE)?.[2] ?? '';
		if (id) {
			used.add(id);
			items.push({ id, text, level });
			return whole;
		}
		id = uniqueSlug(slugifyHeading(text) || 'section', (c) => used.has(c));
		used.add(id);
		items.push({ id, text, level });
		return `<${tag} id="${id}"${attrs}>${inner}</${tag}>`;
	});
	return { html: out, items };
}

/**
 * Client-side variant for DOM we don't control as HTML (Page Builder
 * blocks). Assigns slugified ids to id-less h2/h3 elements in `root` and
 * returns the item list. No-ops outside the browser.
 */
export function scanDomToc(root: HTMLElement, levels = [2, 3]): TocItem[] {
	const sel = levels.map((l) => `h${l}`).join(',');
	const used = new Set<string>();
	const items: TocItem[] = [];
	for (const el of root.querySelectorAll<HTMLElement>(sel)) {
		const text = stripItemPrefix((el.textContent ?? '').replace(/\s+/g, ' ').trim());
		if (!text) continue;
		const level = Number(el.tagName[1]);
		let id = el.id;
		if (!id) {
			id = uniqueSlug(slugifyHeading(text) || 'section', (c) => used.has(c) || !!document.getElementById(c));
			el.id = id;
		}
		// Keep anchors below sticky shell chrome — .prose gets this from a
		// stylesheet rule, but DOM-scanned trees (LandingRenderer blocks)
		// aren't prose.
		if (!el.style.scrollMarginTop) el.style.scrollMarginTop = '6rem';
		used.add(id);
		items.push({ id, text, level });
	}
	return items;
}

type FlagBag = Record<string, unknown> | null | undefined;

function bagOptsOut(bag: FlagBag): boolean {
	return bag?.hide_toc === true || bag?.toc === false;
}

/**
 * Per-page TOC opt-out — true when the entity carries `hide_toc: true` or
 * `toc: false` at the top level, inside `meta` (Page Builder pages) or
 * inside `field_values` (content custom fields). Fail-open by design:
 * every absent flag means the TOC renders.
 */
export function tocHidden(source: unknown): boolean {
	if (!source || typeof source !== 'object') return false;
	const s = source as { hide_toc?: unknown; toc?: unknown; meta?: FlagBag; field_values?: FlagBag };
	return s.hide_toc === true || s.toc === false || bagOptsOut(s.meta) || bagOptsOut(s.field_values);
}

/**
 * Smooth-scroll to a TOC target and sync the URL hash through a shallow,
 * history-replacing goto — TOC hops shouldn't pollute history, and
 * shallow navs skip SvelteKit's scroll reset so nothing races our smooth
 * scroll.
 */
export function scrollToTocItem(id: string): void {
	const el = document.getElementById(id);
	if (!el) return;
	el.scrollIntoView({ behavior: 'smooth', block: 'start' });
	goto(`#${id}`, { replace: true, shallow: true, state: page.state });
}

/**
 * Scroll-spy for TOC lists — reports the id of the section the reader is
 * currently in. The active heading is the first one inside a band just
 * under the sticky chrome (96px→30% of the viewport); when no heading
 * sits in the band (mid-section), the last heading scrolled above it
 * wins. Returns a cleanup that disconnects the observer.
 */
export function watchTocSpy(ids: string[], onActive: (id: string) => void): () => void {
	if (typeof IntersectionObserver === 'undefined') return () => {};
	const els = ids
		.map((id) => document.getElementById(id))
		.filter((el): el is HTMLElement => el instanceof HTMLElement);
	if (els.length === 0) return () => {};
	const inBand = new Set<string>();
	const io = new IntersectionObserver(
		(entries) => {
			for (const e of entries) {
				if (e.isIntersecting) inBand.add(e.target.id);
				else inBand.delete(e.target.id);
			}
			let active = '';
			for (const el of els) {
				if (inBand.has(el.id)) {
					active = el.id;
					break;
				}
			}
			if (!active) {
				for (const el of els) {
					if (el.getBoundingClientRect().top < 96) active = el.id;
				}
			}
			onActive(active);
		},
		{ rootMargin: '-96px 0px -70% 0px', threshold: 0 }
	);
	for (const el of els) io.observe(el);
	return () => io.disconnect();
}
