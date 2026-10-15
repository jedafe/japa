import { describe, it, expect, vi, beforeEach, type MockedFunction } from 'vitest';
import {
	listComparisonTypes,
	getComparisonType,
	searchEntities,
	resolveEntity,
	createGuestSelection,
	createQuickComparison,
	getQuickComparison,
	promoteComparison
} from './jce';
import { api } from './client';
import type { ComparisonType, CreateQuickComparisonInput, JCEComparisonSummary } from '$lib/types/jce';

vi.mock('./client', () => ({
	api: {
		get: vi.fn(),
		post: vi.fn(),
		patch: vi.fn(),
		delete: vi.fn()
	}
}));

type API = typeof api;

const mockedGet = api.get as MockedFunction<API['get']>;
const mockedPost = api.post as MockedFunction<API['post']>;

describe('JCE API client', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('lists comparison types', async () => {
		const types: ComparisonType[] = [
			{
				id: '1',
				name: 'Universities',
				slug: 'universities',
				description: '',
				item_label: 'University',
				config: { max_item_count: 4, item_ref_types: ['university'] }
			},
			{
				id: '2',
				name: 'Banks',
				slug: 'banks',
				description: '',
				item_label: 'Bank'
			}
		];
		mockedGet.mockResolvedValue({ data: types, error: null, status: 200 });

		const result = await listComparisonTypes();

		expect(mockedGet).toHaveBeenCalledWith('/jce/comparison-types');
		expect(result).toEqual(types);
	});

	it('creates quick comparison with forced private defaults', async () => {
		const summary: JCEComparisonSummary = {
			id: 'c1',
			slug: 'test',
			title: 'Test',
			status: 'draft',
			visibility: 'private',
			depth: 'quick'
		};
		mockedPost.mockResolvedValue({ data: summary, error: null, status: 200 });

		const input: CreateQuickComparisonInput = {
			title: 'Quick',
			type_id: 't1',
			depth: 'quick',
			status: 'draft',
			visibility: 'private',
			items: [],
			dimensions: []
		};
		const result = await createQuickComparison(input);

		expect(mockedPost).toHaveBeenCalledWith('/jce/comparisons', {
			...input,
			depth: 'quick',
			status: 'draft',
			visibility: 'private'
		});
		expect(result).toEqual({ data: summary, error: null, status: 200 });
	});

	it('returns null on API error', async () => {
		mockedGet.mockResolvedValue({ data: null, error: { code: 'api_error', message: 'fail', status: 500 }, status: 500 });

		const result = await getQuickComparison('missing');

		expect(result).toBeNull();
	});

});
