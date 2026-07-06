import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null),
	api: {
		getProfessionalProfile: vi.fn().mockRejectedValue(new Error('Not found')),
		updateProfessionalProfile: vi.fn().mockResolvedValue({}),
		deleteProfessionalProfile: vi.fn().mockResolvedValue({})
	}
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

vi.mock('$app/state', () => ({
	page: {
		params: { slug: 'test-user' }
	}
}));

describe('Professional Profile Edit +page.svelte', () => {
	it('renders the back to profile link', () => {
		render(Page);
		expect(screen.getByText('Back to profile')).toBeTruthy();
	});

	it('shows loading skeleton initially', () => {
		const { container } = render(Page);
		expect(container.querySelector('.animate-pulse')).toBeTruthy();
	});

	it('shows error state after failed load', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Failed to load profile.')).toBeTruthy();
		});
	});

	it('renders the error link back to profiles', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Back to profiles')).toBeTruthy();
		});
	});
});
