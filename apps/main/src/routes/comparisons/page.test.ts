import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
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
