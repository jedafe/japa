// Ref: .jeos/sections/14-content-type-system.md (List amendment — multi-page)
//
// Multi-page listicle slicing for the public detail page. The serializer
// stores body_html as a flat run of `<div class="list-item" id="item-N">`
// blocks; field_values.items_per_page is the ONLY authority on pagination —
// markup never decides it. That keeps the contract safe on two fronts:
//
//   1. An API consumer can PATCH field_values.items_per_page alone and the
//      stored body paginates correctly — no body_html choreography needed.
//   2. An author pasting `class="list-item"`/`class="list-page"` into item
//      rich text can't corrupt pagination: item boundaries require the
//      serializer's exact `id="item-N"` pattern, and legacy `.list-page`
//      wrappers are flattened by strict id-pattern regexes before slicing.
//
// All functions are pure string surgery — no DOM, SSR-safe.

const ITEM_MARK = '<div class="list-item" id="item-';
// Wrappers emitted by the pre-field_values-driven serializer:
// `<div class="list-page" id="list-page-N">`. Sticky regex — matched only
// at the cursor position.
const PAGE_MARK = '<div class="list-page" id="list-page-';
const PAGE_TAG = /<div class="list-page" id="list-page-\d+">/;
const PAGE_TAG_AT = /<div class="list-page" id="list-page-\d+">/y;

/**
 * Flatten any `.list-page` wrappers out of stored body_html. Only
 * depth-0 wrapper tags are structural: a stack scan skips a strict-pattern
 * wrapper open and its own matching `</div>`, while a pasted/look-alike
 * wrapper nested inside an item's rich text is preserved as content (and
 * its close stays balanced — no stray `</div>` escapes into the flat
 * output). Bodies without wrappers are returned untouched.
 */
export function flattenListPageWrappers(bodyHtml: string): string {
	if (!bodyHtml.includes(PAGE_MARK)) return bodyHtml;
	// Fast path: no strict wrapper tags at all — pasted look-alikes alone
	// don't need rewriting.
	if (!PAGE_TAG.test(bodyHtml)) return bodyHtml;

	const stack: ('div' | 'page')[] = [];
	let out = '';
	for (let i = 0; i < bodyHtml.length; ) {
		if (stack.length === 0) {
			PAGE_TAG_AT.lastIndex = i;
			const m = PAGE_TAG_AT.exec(bodyHtml);
			if (m) {
				stack.push('page');
				i += m[0].length;
				continue; // drop the wrapper open tag
			}
		}
		if (bodyHtml.startsWith('</div>', i)) {
			const top = stack.pop();
			i += 6;
			if (top === 'page') continue; // drop the wrapper's own close
			out += '</div>';
			continue;
		}
		if (bodyHtml.startsWith('<div', i)) {
			stack.push('div');
			const gt = bodyHtml.indexOf('>', i);
			if (gt < 0) {
				out += bodyHtml.slice(i);
				break;
			}
			out += bodyHtml.slice(i, gt + 1);
			i = gt + 1;
			continue;
		}
		out += bodyHtml[i];
		i++;
	}
	return out;
}

/**
 * Split listicle body_html into the intro block and one HTML string per
 * `.list-item` block, in document order. Wrappers are flattened first, so
 * both flat and legacy-wrapped bodies produce identical chunks.
 *
 * Boundaries are only accepted at div-depth 0: serialized items are
 * top-level siblings, while a `<div class="list-item" id="item-N">`
 * nested inside an item's own rich text (e.g. a pasted rendered list —
 * the sanitizer whitelists class/id, so it can reach storage) is content,
 * not a boundary. The scan tracks `<div`/`</div>` depth so nested
 * look-alikes can never desync items_per_page slicing.
 */
export function splitListItems(bodyHtml: string): { intro: string; items: string[] } {
	const flat = flattenListPageWrappers(bodyHtml);
	const items: string[] = [];
	let intro = '';
	let depth = 0;
	let itemStart = -1;

	for (let i = 0; i < flat.length; ) {
		if (depth === 0 && flat.startsWith(ITEM_MARK, i)) {
			if (itemStart < 0) intro = flat.slice(0, i);
			else items.push(flat.slice(itemStart, i));
			itemStart = i;
			// The marker itself opens the item's div.
			const gt = flat.indexOf('>', i);
			if (gt < 0) break;
			i = gt + 1;
			depth = 1;
			continue;
		}
		if (flat.startsWith('</div>', i)) {
			if (depth > 0) depth--;
			i += 6;
			continue;
		}
		if (flat.startsWith('<div', i)) {
			depth++;
			i += 4;
			continue;
		}
		i++;
	}
	if (itemStart < 0) return { intro: flat, items };
	items.push(flat.slice(itemStart));
	return { intro, items };
}

/**
 * 0-based index of the item block whose OWN marker is `id="item-N"`;
 * -1 when absent. Matches the block's leading marker only — a nested
 * `id="item-N"` inside an item's rich text must not redirect the
 * reader to a phantom anchor's page.
 */
export function listItemIndexByDomId(items: string[], n: number): number {
	if (!Number.isInteger(n) || n < 1) return -1;
	const want = `${ITEM_MARK}${n}"`;
	return items.findIndex((html) => html.startsWith(want));
}

/** Number of item pages for a listicle; 0 = single-page (no pager). */
export function listPageCount(itemCount: number, itemsPerPage: number): number {
	if (itemsPerPage < 1 || itemCount <= itemsPerPage) return 0;
	return Math.ceil(itemCount / itemsPerPage);
}

/** Clamp an arbitrary ?page= param into a valid 1-based page number. */
export function clampListPage(raw: number, pageCount: number): number {
	if (pageCount < 1) return 1;
	if (!Number.isFinite(raw) || raw < 1) return 1;
	return Math.min(Math.floor(raw), pageCount);
}
