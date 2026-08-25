import { describe, it, expect, vi, beforeEach, type MockedFunction } from 'vitest';
import {
	listComparisonTypes,
	getComparisonType,
	searchEntities,
	resolveEntity,
	createGuestSelection,
	createQuickComparisonFromToken,
	createQuickComparison,
	getQuickComparison,
	listComparisons,
	promoteComparison,
	listProfiles,
	createProfile,
	getScore,
	previewScore
} from './jce';
import { api } from './client';
import type { ComparisonProfile, ComparisonProfileInput, ComparisonType, CreateQuickComparisonInput, JCEComparisonSummary, ScorePreviewInput, ScoreResult } from '$lib/types/jce';

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
		expect(result).toEqual(summary);
	});

	it('returns null on API error', async () => {
		mockedGet.mockResolvedValue({ data: null, error: { code: 'api_error', message: 'fail', status: 500 }, status: 500 });

		const result = await getQuickComparison('missing');

		expect(result).toBeNull();
	});

	it('lists profiles', async () => {
		const profiles: ComparisonProfile[] = [
			{
				id: 'p1',
				title: 'My profile',
				target_space_ids: [],
				goals: [],
				preferences: {},
				demographics: {},
				is_default: true,
				is_template: false
			}
		];
		mockedGet.mockResolvedValue({ data: profiles, error: null, status: 200 });

		const result = await listProfiles();

		expect(mockedGet).toHaveBeenCalledWith('/jce/profiles');
		expect(result).toEqual(profiles);
	});

	it('creates a profile', async () => {
		const input: ComparisonProfileInput = {
			title: 'New profile',
			target_space_ids: ['s1'],
			goals: [{ key: 'low_cost', weight: 0.25 }]
		};
		const profile: ComparisonProfile = {
			id: 'p2',
			title: input.title ?? '',
			source_space_id: input.source_space_id,
			target_space_ids: input.target_space_ids,
			goals: input.goals ?? [],
			preferences: input.preferences ?? {},
			demographics: input.demographics ?? {},
			is_default: input.is_default ?? false,
			is_template: input.is_template ?? false
		};
		mockedPost.mockResolvedValue({ data: profile, error: null, status: 200 });

		const result = await createProfile(input);

		expect(mockedPost).toHaveBeenCalledWith('/jce/profiles', input);
		expect(result).toEqual(profile);
	});

	it('gets a score for a comparison and profile', async () => {
		const score: ScoreResult = {
			content_id: 'c1',
			profile_id: 'p1',
			smart_mode: 'eligibility',
			scores: [],
			recommendations: [],
			weights_snapshot: {},
			recommendation_summary: {
				top_item_id: '',
				top_japa_score: 0,
				recommendations_count: 0,
				last_scored_at: '2026-08-25T00:00:00Z'
			},
			version: 1
		};
		mockedGet.mockResolvedValue({ data: score, error: null, status: 200 });

		const result = await getScore('c1', 'p1');

		expect(mockedGet).toHaveBeenCalledWith('/jce/comparisons/c1/score?profile_id=p1');
		expect(result).toEqual(score);
	});

	it('previews a score without creating content', async () => {
		const input: ScorePreviewInput = {
			type_id: 't1',
			smart_mode: 'eligibility',
			item_refs: [{ type: 'university', id: 'u1' }]
		};
		const score: ScoreResult = {
			content_id: '',
			profile_id: null,
			smart_mode: 'eligibility',
			scores: [],
			recommendations: [],
			weights_snapshot: {},
			recommendation_summary: {
				top_item_id: '',
				top_japa_score: 0,
				recommendations_count: 0,
				last_scored_at: '2026-08-25T00:00:00Z'
			},
			version: 1
		};
		mockedPost.mockResolvedValue({ data: score, error: null, status: 200 });

		const result = await previewScore(input);

		expect(mockedPost).toHaveBeenCalledWith('/jce/score-preview', input);
		expect(result).toEqual(score);
	});
});
