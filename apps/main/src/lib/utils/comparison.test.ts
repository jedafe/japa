import { describe, expect, it } from 'vitest';
import { fieldGroupLabel, groupDimensionsByPool, matrixBestCells } from './comparison.js';
import type { JCEDimension, JCEFieldPoolGroup, MatrixItem } from '#lib/types/jce.js';

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

const field = (id: string) => ({ id, key: id, label: id, field_type: 'text', comparable: true });
const pool: JCEFieldPoolGroup[] = [
	{ group_id: 'g-cost', slug: 'cost', label: 'Cost of Living (city)', is_default: true, fields: [field('f-rent'), field('f-food')] },
	{ group_id: 'g-jobs', slug: 'jobs', label: '{name} Jobs', is_default: false, fields: [field('f-salary')] }
];

describe('fieldGroupLabel', () => {
	it('strips {name} tokens and trailing (scope) suffixes', () => {
		expect(fieldGroupLabel('Cost of Living (city)')).toBe('Cost of Living');
		expect(fieldGroupLabel('{name} Jobs')).toBe('Jobs');
		expect(fieldGroupLabel('Overview')).toBe('Overview');
	});
});

describe('groupDimensionsByPool', () => {
	it('buckets dims by source_field_id, keeps pool order, cleans labels', () => {
		const grouped = groupDimensionsByPool(
			[
				{ id: 'd1', label: 'Salary', source_field_id: 'f-salary' },
				{ id: 'd2', label: 'Rent', source_field_id: 'f-rent' },
				{ id: 'd3', label: 'Groceries', source_field_id: 'f-food' }
			],
			pool
		);
		expect(grouped.map((g) => g.key)).toEqual(['g-cost', 'g-jobs']);
		expect(grouped[0].label).toBe('Cost of Living');
		expect(grouped[0].dims.map((d) => d.id)).toEqual(['d2', 'd3']);
		expect(grouped[1].label).toBe('Jobs');
	});
	it('routes dims without a source field to a trailing other bucket', () => {
		const grouped = groupDimensionsByPool(
			[
				{ id: 'd1', label: 'Salary', source_field_id: 'f-salary' },
				{ id: 'd2', label: 'Notes', source_field_id: null },
				{ id: 'd3', label: 'Legacy', source_field_id: 'f-removed' }
			],
			pool
		);
		expect(grouped.map((g) => g.key)).toEqual(['g-jobs', 'other']);
		expect(grouped[1].dims.map((d) => d.id)).toEqual(['d2', 'd3']);
	});
	it('empty pool → single other bucket with every dim', () => {
		const grouped = groupDimensionsByPool(dims, []);
		expect(grouped).toHaveLength(1);
		expect(grouped[0].key).toBe('other');
		expect(grouped[0].dims).toHaveLength(3);
	});
	it('skips pool groups with no matching dims', () => {
		const grouped = groupDimensionsByPool(
			[{ id: 'd1', label: 'Salary', source_field_id: 'f-salary' }],
			pool
		);
		expect(grouped.map((g) => g.key)).toEqual(['g-jobs']);
	});
});
