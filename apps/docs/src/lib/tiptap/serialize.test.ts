import { describe, it, expect } from 'vitest';
import { ensureHeadingIds, extractTocHeadings, injectHeadingAnchors } from './serialize';

const doc = '<h2>Getting started</h2><p>Intro</p><h3>First steps</h3><p>Body</p><h2>Getting started</h2>';

describe('serialize heading pipeline', () => {
	it('ensureHeadingIds injects slugified, de-duplicated ids', () => {
		const out = ensureHeadingIds(doc);
		expect(out).toContain('id="getting-started"');
		expect(out).toContain('id="first-steps"');
		expect(out).toContain('id="getting-started-2"');
	});

	it('extractTocHeadings returns clean text (no anchor residue)', () => {
		const withIds = ensureHeadingIds(doc);
		const toc = extractTocHeadings(withIds);
		expect(toc.map((h) => h.text)).toEqual(['Getting started', 'First steps', 'Getting started']);
		expect(toc.map((h) => h.id)).toEqual(['getting-started', 'first-steps', 'getting-started-2']);
		expect(toc.map((h) => h.level)).toEqual([2, 3, 2]);
	});

	it('injectHeadingAnchors adds a permalink per heading', () => {
		const withIds = ensureHeadingIds(doc);
		const out = injectHeadingAnchors(withIds);
		expect(out).toContain(
			'Getting started<a class="heading-anchor" href="#getting-started" aria-hidden="true" tabindex="-1">#</a>'
		);
		expect(out).toContain('href="#first-steps"');
	});

	it('anchors injected after TOC extraction keep TOC labels clean', () => {
		// Order matters: extract first, then inject — mirroring +page.server.ts.
		const withIds = ensureHeadingIds(doc);
		const toc = extractTocHeadings(withIds);
		injectHeadingAnchors(withIds);
		expect(toc.every((h) => !h.text.includes('#'))).toBe(true);
	});
});
