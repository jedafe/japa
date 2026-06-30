import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null),
	api: {
		getComparisonMatrix: vi.fn().mockResolvedValue({ items: [] })
	}
}));

vi.mock('$lib/stores/features.svelte', () => ({
	isFeatureEnabled: () => true,
	setFeatureFlags: () => {}
}));

vi.mock('$lib/utils/timezone', () => ({
	formatTimestamp: () => 'Jan 15, 2026'
}));

vi.mock('$lib/components/ui', async () => {
	const { default: Comp } = await import('$lib/components/EmptyStateMock.svelte');
	return { EmptyState: Comp };
});

const mockType = {
	id: 'type-1',
	name: 'Countries',
	slug: 'countries',
	description: 'Compare countries for relocation',
	field_groups: []
};

const mockComparisons: { id: string; title: string; slug: string; item_count: number; vote_count: number; created_at: string }[] = [];

describe('Comparison Type +page.svelte', () => {
	const baseData = {
		user: null,
		flags: {},
		notificationCount: 0,
		token: null,
		locale: 'en',
		translations: {}
	};

	it('renders the type name as heading', () => {
		render(Page, { props: { data: { ...baseData, type: mockType, comparisons: mockComparisons } } });
		expect(screen.getByRole('heading', { name: /Countries/i })).toBeTruthy();
	});

	it('shows the type description', () => {
		render(Page, { props: { data: { ...baseData, type: mockType, comparisons: mockComparisons } } });
		expect(screen.getByText('Compare countries for relocation')).toBeTruthy();
	});

	it('renders the back to comparisons link', () => {
		render(Page, { props: { data: { ...baseData, type: mockType, comparisons: mockComparisons } } });
		expect(screen.getByText('Back to comparisons')).toBeTruthy();
	});

	it('renders the Create comparison button', () => {
		render(Page, { props: { data: { ...baseData, type: mockType, comparisons: mockComparisons } } });
		expect(screen.getByText('Create comparison')).toBeTruthy();
	});

	it('shows empty state when no comparisons', () => {
		render(Page, { props: { data: { ...baseData, type: mockType, comparisons: mockComparisons } } });
		expect(screen.getByText('No comparisons yet')).toBeTruthy();
	});
});
