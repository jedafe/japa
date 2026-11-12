import type { JCEDimension, MatrixDimension, MatrixItem, ResolvedItem } from '#lib/types/jce.js';

/**
 * Plain-text excerpt for list cards — the list row has no excerpt column,
 * so cards derive one from body (strip tags, collapse whitespace, clamp
 * at a word boundary).
 */
export function excerptText(text?: string | null, max = 140): string {
	if (!text) return '';
	const plain = text.replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim();
	if (plain.length <= max) return plain;
	return plain.slice(0, max).replace(/\s+\S*$/, '') + '…';
}

/**
 * Return the first occurrence of each item keyed by a computed value.
 * Order is preserved.
 */
export function uniqueBy<T, K>(items: T[], keyFn: (item: T) => K): T[] {
	return [...new Map(items.map((i) => [keyFn(i), i])).values()];
}

/** Deduplicate resolved item cards by position + item_ref + title — the
 *  item_ref (type:id) is part of the key so two different entities with the
 *  same title never collapse (mirrors the backend's itemRefDedupKey). */
export function uniqueResolvedItems(items: ResolvedItem[]): ResolvedItem[] {
	return uniqueBy(items, (i) => `${i.position ?? ''}:${i.type ?? ''}:${i.id ?? ''}:${i.title}`);
}

/** Deduplicate comparison dimensions by position + label, preserving the first. */
export function uniqueDimensions(dimensions: JCEDimension[]): JCEDimension[] {
	return uniqueBy(dimensions, (d) => `${d.position ?? ''}:${d.label}`);
}

/** Deduplicate matrix rows by position + item_ref + title and merge any
 *  split dimension values — the row key mirrors the backend's
 *  (position, item_ref, title) and the per-cell merge mirrors
 *  mergeComparisonValues (first non-null wins). */
export function deduplicateMatrix(matrix: MatrixItem[]): MatrixItem[] {
	const byItem = new Map<string, MatrixItem>();

	for (const item of matrix) {
		const ref = item.item_ref as { type?: string; id?: string } | undefined;
		const key = `${item.position ?? ''}:${ref?.type ?? ''}:${ref?.id ?? ''}:${item.title}`;
		let merged = byItem.get(key);
		if (!merged) {
			merged = { ...item, dimensions: [] };
			byItem.set(key, merged);
		}

		const byDim = new Map<string, MatrixDimension>();
		for (const d of merged.dimensions) {
			byDim.set(d.dimension_id, { ...d });
		}

		for (const d of item.dimensions) {
			const existing = byDim.get(d.dimension_id);
			if (!existing) {
				byDim.set(d.dimension_id, { ...d });
				continue;
			}
			if (existing.value === undefined || existing.value === null) {
				existing.value = d.value;
			}
			if (existing.value_rating === undefined || existing.value_rating === null) {
				existing.value_rating = d.value_rating;
			}
			if (existing.value_boolean === undefined || existing.value_boolean === null) {
				existing.value_boolean = d.value_boolean;
			}
		}

		merged.dimensions = [...byDim.values()];
	}

	return [...byItem.values()];
}

/**
 * Best-value cell per dimension row. Numeric comparisons only — value_rating
 * first, then a numeric parse of value (non-numeric strings are skipped).
 * Cost/rank-like dimensions (cost|price|fee|tuition|rent|expense|rank in the
 * label) invert to lower-is-better. Keys are `${item.id}:${dim.id}` for the
 * matrix <td> lookup. Rows with fewer than two numeric cells are skipped, and
 * ties mark every equal-best cell.
 */
const LOWER_IS_BETTER_LABEL = /cost|price|fee|tuition|rent|expense|rank/i;

export function matrixBestCells(matrix: MatrixItem[], dims: JCEDimension[]): Set<string> {
	const best = new Set<string>();
	for (const dim of dims) {
		const lowerIsBetter = LOWER_IS_BETTER_LABEL.test(dim.label);
		let bestVal: number | null = null;
		let numericCount = 0;
		let bestKeys: string[] = [];
		for (const item of matrix) {
			const cell = item.dimensions.find((d) => d.dimension_id === dim.id);
			const v =
				cell?.value_rating ??
				(cell?.value != null && cell.value !== '' ? Number(cell.value) : null);
			if (v == null || Number.isNaN(v)) continue;
			numericCount++;
			const key = `${item.id}:${dim.id}`;
			if (bestVal === null || (lowerIsBetter ? v < bestVal : v > bestVal)) {
				bestVal = v;
				bestKeys = [key];
			} else if (v === bestVal) {
				bestKeys.push(key);
			}
		}
		if (numericCount >= 2) {
			for (const k of bestKeys) best.add(k);
		}
	}
	return best;
}
