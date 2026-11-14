import { describe, it, expect } from 'vitest';
import { extractToc, slugifyHeading, stripItemPrefix, tocHidden } from './toc';

describe('slugifyHeading', () => {
	it('lowercases and hyphenates', () => {
		expect(slugifyHeading('Getting a Visa')).toBe('getting-a-visa');
	});

	it('keeps unicode letters (accents, non-latin)', () => {
		expect(slugifyHeading('Étudier à Montréal')).toBe('étudier-à-montréal');
	});

	it('strips edge dashes and collapses punctuation runs', () => {
		expect(slugifyHeading('  Costs — 2026!  ')).toBe('costs-2026');
	});
});

describe('extractToc', () => {
	it('injects slugified ids and lists items in order', () => {
		const html = '<p>intro</p><h2>First Steps</h2><p>x</p><h3>Sub One</h3><h2>Second</h2>';
		const { html: out, items } = extractToc(html);
		expect(items).toEqual([
			{ id: 'first-steps', text: 'First Steps', level: 2 },
			{ id: 'sub-one', text: 'Sub One', level: 3 },
			{ id: 'second', text: 'Second', level: 2 }
		]);
		expect(out).toContain('<h2 id="first-steps">First Steps</h2>');
		expect(out).toContain('<h3 id="sub-one">Sub One</h3>');
	});

	it('dedupes repeated heading texts', () => {
		const { html: out, items } = extractToc('<h2>Visa</h2><h2>Visa</h2><h2>Visa</h2>');
		expect(items.map((i) => i.id)).toEqual(['visa', 'visa-2', 'visa-3']);
		expect(out).toContain('id="visa-2"');
	});

	it('keeps existing ids and uses them as anchors', () => {
		const { html: out, items } = extractToc('<h2 id="custom-x">Head</h2>');
		expect(out).toBe('<h2 id="custom-x">Head</h2>');
		expect(items).toEqual([{ id: 'custom-x', text: 'Head', level: 2 }]);
	});

	it('preserves other attributes when injecting id', () => {
		const { html: out } = extractToc('<h2 class="big" data-x="1">T</h2>');
		expect(out).toContain('<h2 id="t" class="big" data-x="1">T</h2>');
	});

	it('strips inline tags and decodes entities in item text', () => {
		const { items } = extractToc('<h2>Cost &amp; <strong>Fees</strong></h2>');
		expect(items).toEqual([{ id: 'cost-fees', text: 'Cost & Fees', level: 2 }]);
	});

	it('skips h4 by default, includes it when listed in levels', () => {
		const html = '<h2>A</h2><h4>Deep</h4>';
		expect(extractToc(html).items.map((i) => i.level)).toEqual([2]);
		expect(extractToc(html, { levels: [2, 3, 4] }).items.map((i) => i.level)).toEqual([2, 4]);
	});

	it('skips empty headings', () => {
		const { html: out, items } = extractToc('<h2></h2><h2>Real</h2>');
		expect(items).toHaveLength(1);
		expect(out).toContain('<h2></h2>');
	});

	it('strips prepended enumerators from item text and slug', () => {
		const { html: out, items } = extractToc('<h2>1. Introduction</h2><h2>• Notes</h2><h2>2) Fees</h2>');
		expect(items).toEqual([
			{ id: 'introduction', text: 'Introduction', level: 2 },
			{ id: 'notes', text: 'Notes', level: 2 },
			{ id: 'fees', text: 'Fees', level: 2 }
		]);
		expect(out).toContain('<h2 id="introduction">1. Introduction</h2>');
	});

	it('ignores data-id — it is not an anchor id', () => {
		const { html: out, items } = extractToc('<h2 data-id="not-an-anchor">Head</h2>');
		expect(items).toEqual([{ id: 'head', text: 'Head', level: 2 }]);
		expect(out).toContain('<h2 id="head" data-id="not-an-anchor">Head</h2>');
	});

	it('suffixes slugs that collide with reserved template ids', () => {
		const { items } = extractToc('<h2>Comparison items</h2>', {
			reserved: ['comparison-items', 'comparison-matrix']
		});
		expect(items).toEqual([{ id: 'comparison-items-2', text: 'Comparison items', level: 2 }]);
	});

	it('suffixes slugs that collide with non-heading ids inside the html', () => {
		const { html: out, items } = extractToc('<div id="item-1">x</div><h2>Item 1</h2>');
		expect(items).toEqual([{ id: 'item-1-2', text: 'Item 1', level: 2 }]);
		expect(out).toContain('<h2 id="item-1-2">Item 1</h2>');
	});

	it('decodes common named entities in item text', () => {
		const { items } = extractToc('<h2>Costs &mdash; what&rsquo;s real</h2>');
		expect(items).toEqual([{ id: 'costs-what-s-real', text: 'Costs — what’s real', level: 2 }]);
	});
});

describe('stripItemPrefix', () => {
	it('removes numeric, bullet, and lettered prefixes', () => {
		expect(stripItemPrefix('1. Intro')).toBe('Intro');
		expect(stripItemPrefix('12) Fees')).toBe('Fees');
		expect(stripItemPrefix('- Dash')).toBe('Dash');
		expect(stripItemPrefix('• Bullet')).toBe('Bullet');
		expect(stripItemPrefix('A. Lettered')).toBe('Lettered');
	});

	it('leaves non-enumerator text alone', () => {
		expect(stripItemPrefix('2024 Outlook')).toBe('2024 Outlook');
		expect(stripItemPrefix('U.S. Relations')).toBe('U.S. Relations');
		expect(stripItemPrefix('Section 2: Costs')).toBe('Section 2: Costs');
		expect(stripItemPrefix('Étape 1')).toBe('Étape 1');
	});

	it('strips roman numeral enumerators', () => {
		expect(stripItemPrefix('iv. Scope')).toBe('Scope');
		expect(stripItemPrefix('XI. Appendix')).toBe('Appendix');
		expect(stripItemPrefix('iOS apps')).toBe('iOS apps');
	});
});

describe('tocHidden', () => {
	it('is fail-open on empty/missing flags', () => {
		expect(tocHidden(null)).toBe(false);
		expect(tocHidden({})).toBe(false);
		expect(tocHidden({ meta: {}, field_values: {} })).toBe(false);
	});

	it('honors top-level hide_toc and toc:false', () => {
		expect(tocHidden({ hide_toc: true })).toBe(true);
		expect(tocHidden({ toc: false })).toBe(true);
		expect(tocHidden({ hide_toc: false, toc: true })).toBe(false);
	});

	it('honors meta and field_values bags', () => {
		expect(tocHidden({ meta: { hide_toc: true } })).toBe(true);
		expect(tocHidden({ meta: { toc: false } })).toBe(true);
		expect(tocHidden({ field_values: { hide_toc: true } })).toBe(true);
		expect(tocHidden({ meta: { title: 'x' }, field_values: { a: 1 } })).toBe(false);
	});
});
