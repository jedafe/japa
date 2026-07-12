import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

const mockRequest = vi.fn();

vi.mock('$lib/api', () => ({
	request: (...args: unknown[]) => mockRequest(...args)
}));

describe('Universities +page.svelte', () => {
	beforeEach(() => {
		mockRequest.mockReset();
	});

	it('renders the heading', async () => {
		mockRequest.mockResolvedValue([]);
		render(Page);
		await waitFor(() => {
			const headings = screen.getAllByRole('heading', { name: /Universities/i });
			expect(headings.length).toBeGreaterThanOrEqual(1);
		});
	});

	it('renders university cards', async () => {
		mockRequest.mockResolvedValue([
			{ id: '1', name: 'Oxford University', slug: 'oxford', country: 'UK' },
			{ id: '2', name: 'MIT', slug: 'mit', country: 'USA' }
		]);
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Oxford University')).toBeTruthy();
			expect(screen.getByText('MIT')).toBeTruthy();
		});
	});

	it('shows empty state when no universities', async () => {
		mockRequest.mockResolvedValue([]);
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No universities found')).toBeTruthy();
		});
	});
});
