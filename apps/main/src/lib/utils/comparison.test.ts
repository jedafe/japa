import { describe, expect, it } from 'vitest';
import { matrixBestCells } from './comparison.js';
import type { JCEDimension, MatrixItem } from '#lib/types/jce.js';

const dims: JCEDimension[] = [
	{ id: 'd-rating', label: 'Job Opportunities', dimension_type: 'rating' },
	{ id: 'd-cost', label: 'Cost of Living', dimension_type: 'currency' },
	{ id: 'd-rank', label: 'QS Ranking', dimension_type: 'number' }
];
const matrix: MatrixItem[] = [
	{
		id: 'a',
		title: 'A',
		dimensions: [
			{ dimension_id: 'd-rating', label: '', type: 'rating', value_rating: 4 },
			{ dimension_id: 'd-cost', label: '', type: 'currency', value_rating: 2000, currency_code: 'USD' },
			{ dimension_id: 'd-rank', label: '', type: 'number', value_rating: 8 }
		]
	},
	{
		id: 'b',
		title: 'B',
		dimensions: [
			{ dimension_id: 'd-rating', label: '', type: 'rating', value_rating: 5 },
			{ dimension_id: 'd-cost', label: '', type: 'currency', value_rating: 2400, currency_code: 'USD' },
			{ dimension_id: 'd-rank', label: '', type: 'number', value_rating: 3 }
		]
	}
];

describe('matrixBestCells', () => {
	it('higher wins for ratings, lower wins for cost/rank dims', () => {
		const best = matrixBestCells(matrix, dims);
		expect(best.has('b:d-rating')).toBe(true);
		expect(best.has('a:d-cost')).toBe(true);
		expect(best.has('b:d-rank')).toBe(true);
		expect(best.size).toBe(3);
	});
	it('marks all tied best cells', () => {
		const tied: MatrixItem[] = [
			{ id: 'a', title: 'A', dimensions: [{ dimension_id: 'd-rating', label: '', type: 'rating', value_rating: 5 }] },
			{ id: 'b', title: 'B', dimensions: [{ dimension_id: 'd-rating', label: '', type: 'rating', value_rating: 5 }] }
		];
		const best = matrixBestCells(tied, dims);
		expect(best.has('a:d-rating')).toBe(true);
		expect(best.has('b:d-rating')).toBe(true);
	});
	it('skips dimensions with <2 numeric cells', () => {
		const sparse: MatrixItem[] = [
			{ id: 'a', title: 'A', dimensions: [{ dimension_id: 'd-rating', label: '', type: 'rating', value_rating: 4 }] }
		];
		expect(matrixBestCells(sparse, dims).size).toBe(0);
	});
	it('skips non-numeric string cells', () => {
		const texty: MatrixItem[] = [
			{ id: 'a', title: 'A', dimensions: [{ dimension_id: 'd-rating', label: '', type: 'text', value: 'Excellent' }] },
			{ id: 'b', title: 'B', dimensions: [{ dimension_id: 'd-rating', label: '', type: 'text', value: 'Good' }] }
		];
		expect(matrixBestCells(texty, dims).size).toBe(0);
	});
});
