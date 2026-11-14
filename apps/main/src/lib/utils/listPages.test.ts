import { describe, it, expect } from 'vitest';
import {
	clampListPage,
	flattenListPageWrappers,
	listItemIndexByDomId,
	listPageCount,
	splitListItems
} from './listPages';

const it3 = (n: number, inner = '') =>
	`<div class="list-item" id="item-${n}"><h3>${inner || `H${n}`}</h3></div>`;

describe('splitListItems', () => {
	it('splits flat body into intro + item chunks', () => {
		const body = `<p>intro</p>${it3(1)}${it3(2)}${it3(3)}`;
		const { intro, items } = splitListItems(body);
		expect(intro).toBe('<p>intro</p>');
		expect(items).toHaveLength(3);
		expect(items[0]).toContain('id="item-1"');
		expect(items[2]).toContain('id="item-3"');
	});

	it('returns no items for a body with no list-items', () => {
		const { intro, items } = splitListItems('<p>just text</p>');
		expect(intro).toBe('<p>just text</p>');
		expect(items).toHaveLength(0);
	});

	it('ignores class="list-item" look-alikes without the id pattern', () => {
		const evil = `<p>i</p><div class="list-item"><p>not an item</p></div>${it3(1)}`;
		const { items } = splitListItems(evil);
		expect(items).toHaveLength(1);
		expect(items[0]).toContain('id="item-1"');
	});

	it('ignores item markers nested inside an item body', () => {
		// A rendered list pasted into item rich text survives the class/id
		// whitelist — nested markers are content, never a page boundary.
		const nested = `<div class="list-item" id="item-99"><p>pasted</p></div>`;
		const body = `<p>i</p>${it3(1)}${it3(2, nested)}${it3(3)}`;
		const { items } = splitListItems(body);
		expect(items).toHaveLength(3);
		expect(items[1]).toContain('id="item-99"');
		expect(items[1]).toContain('id="item-2"');
	});

	it('keeps item bodies containing other divs balanced', () => {
		const body = `<p>i</p>${it3(1, '<div data-type="link-preview"></div>')}${it3(2)}`;
		const { items } = splitListItems(body);
		expect(items).toHaveLength(2);
		expect(items[1]).toContain('id="item-2"');
	});
});

describe('listItemIndexByDomId', () => {
	const blocks = [it3(1), it3(2, '<div class="list-item" id="item-9"><p>x</p></div>'), it3(3)];

	it('matches the block-own marker only', () => {
		expect(listItemIndexByDomId(blocks, 1)).toBe(0);
		expect(listItemIndexByDomId(blocks, 2)).toBe(1);
		expect(listItemIndexByDomId(blocks, 3)).toBe(2);
		// The nested id="item-9" inside block 2 must not resolve.
		expect(listItemIndexByDomId(blocks, 9)).toBe(-1);
	});

	it('rejects invalid ids', () => {
		expect(listItemIndexByDomId(blocks, 0)).toBe(-1);
		expect(listItemIndexByDomId(blocks, -2)).toBe(-1);
		expect(listItemIndexByDomId(blocks, 1.5)).toBe(-1);
		expect(listItemIndexByDomId(blocks, NaN)).toBe(-1);
		expect(listItemIndexByDomId(blocks, 99)).toBe(-1);
	});
});

describe('flattenListPageWrappers', () => {
	const wrapped = (groups: number[][]) =>
		`<p>intro</p>` +
		groups
			.map(
				(g, i) =>
					`<div class="list-page" id="list-page-${i + 1}">${g.map((n) => it3(n)).join('')}</div>`
			)
			.join('');

	it('unwraps multi-page bodies into flat item runs', () => {
		const flat = flattenListPageWrappers(wrapped([[1, 2], [3, 4], [5]]));
		expect(flat).not.toContain('list-page');
		expect(flat).toBe(`<p>intro</p>${it3(1)}${it3(2)}${it3(3)}${it3(4)}${it3(5)}`);
	});

	it('round-trips wrapped bodies through splitListItems', () => {
		const { intro, items } = splitListItems(wrapped([[1, 2], [3]]));
		expect(intro).toBe('<p>intro</p>');
		expect(items).toHaveLength(3);
		expect(items[2]).toContain('id="item-3"');
	});

	it('leaves flat bodies untouched', () => {
		const body = `<p>i</p>${it3(1)}`;
		expect(flattenListPageWrappers(body)).toBe(body);
	});

	it('ignores look-alike wrappers without the id pattern', () => {
		const body = `${it3(1, 'X')}<div class="list-page"><p>kept</p></div>`;
		const flat = flattenListPageWrappers(body);
		expect(flat).toContain('<p>kept</p>');
	});

	it('keeps wrapper markup nested inside an item body balanced', () => {
		// A pasted list-page wrapper inside an item is content, not a
		// structural wrapper — its open AND close must survive so no stray
		// </div> escapes into the flat output.
		const nested = `<div class="list-page" id="list-page-7"><p>pasted</p></div>`;
		const flat = flattenListPageWrappers(`<p>i</p>${it3(1, nested)}${it3(2)}`);
		expect(flat).toBe(`<p>i</p>${it3(1, nested)}${it3(2)}`);
	});

	it('still flattens real wrappers around a nested pasted one', () => {
		const nested = `<div class="list-page" id="list-page-9"><p>x</p></div>`;
		const body = `<div class="list-page" id="list-page-1">${it3(1, nested)}</div><div class="list-page" id="list-page-2">${it3(2)}</div>`;
		expect(flattenListPageWrappers(body)).toBe(`${it3(1, nested)}${it3(2)}`);
	});

	it('tolerates an unclosed trailing wrapper', () => {
		const body = `<div class="list-page" id="list-page-1">${it3(1)}${it3(2)}`;
		expect(flattenListPageWrappers(body)).toBe(`${it3(1)}${it3(2)}`);
	});
});

describe('listPageCount', () => {
	it('is 0 for single-page configs', () => {
		expect(listPageCount(5, 0)).toBe(0);
		expect(listPageCount(5, 5)).toBe(0);
		expect(listPageCount(5, 9)).toBe(0);
		expect(listPageCount(0, 3)).toBe(0);
	});

	it('ceil-divides items over per-page', () => {
		expect(listPageCount(7, 2)).toBe(4);
		expect(listPageCount(6, 3)).toBe(2);
		expect(listPageCount(7, 1)).toBe(7);
	});
});

describe('clampListPage', () => {
	it('clamps into [1, pageCount]', () => {
		expect(clampListPage(2, 4)).toBe(2);
		expect(clampListPage(0, 4)).toBe(1);
		expect(clampListPage(-3, 4)).toBe(1);
		expect(clampListPage(99, 4)).toBe(4);
		expect(clampListPage(NaN, 4)).toBe(1);
		expect(clampListPage(2.9, 4)).toBe(2);
	});

	it('always returns 1 for an unpaginated list', () => {
		expect(clampListPage(5, 0)).toBe(1);
	});
});
