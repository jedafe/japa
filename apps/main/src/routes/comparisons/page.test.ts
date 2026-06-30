import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null),
	api: {
		getComparisonTypes: vi.fn().mockResolvedValue([])
	}
}));

vi.mock('$lib/stores/i18n.svelte', () => {
	const translations: Record<string, string> = {
		'comparisons.title': 'Comparisons',
		'comparisons.subtitle': 'Make informed decisions by comparing options side by side.',
		'comparisons.none': 'No comparisons yet',
		'comparisons.none_msg': 'Be the first.',
		'comparisons.not_available': 'Comparisons is not available',
		'forum.feature_disabled': 'This feature is currently disabled.',
		'forum.go_home': 'Go home'
	};
	return { t: (key: string) => translations[key] ?? key };
});

vi.mock('$lib/stores/features.svelte', () => ({
	isFeatureEnabled: () => true,
	setFeatureFlags: () => {}
}));

vi.mock('$lib/components/ui', async () => {
	const { default: Comp } = await import('$lib/components/EmptyStateMock.svelte');
	return { EmptyState: Comp };
});

describe('Comparisons +page.svelte', () => {
	it('renders the Comparisons heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: /Comparisons/i })).toBeTruthy();
	});

	it('shows the subtitle text', () => {
		render(Page);
		expect(screen.getByText('Make informed decisions by comparing options side by side.')).toBeTruthy();
	});

	it('renders the page structure with main element', () => {
		const { container } = render(Page);
		expect(container.querySelector('main')).toBeTruthy();
	});
});
