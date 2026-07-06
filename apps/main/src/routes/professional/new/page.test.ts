import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null),
	api: {
		createProfessionalProfile: vi.fn().mockResolvedValue({ slug: 'test-profile' })
	}
}));

describe('Professional New +page.svelte', () => {
	it('renders the Create professional profile heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: 'Create professional profile' })).toBeTruthy();
	});

	it('shows the subtitle text', () => {
		render(Page);
		expect(screen.getByText('Showcase your skills and relocation journey.')).toBeTruthy();
	});

	it('renders the Back to profiles link', () => {
		render(Page);
		expect(screen.getByText('Back to profiles')).toBeTruthy();
	});

	it('renders the name input field', () => {
		render(Page);
		expect(screen.getByPlaceholderText('Your professional name')).toBeTruthy();
	});

	it('renders the headline input field', () => {
		render(Page);
		expect(screen.getByPlaceholderText('e.g. Senior Software Engineer | Open to relocation')).toBeTruthy();
	});

	it('renders the skills input field', () => {
		render(Page);
		expect(screen.getByPlaceholderText('Type a skill and press Enter')).toBeTruthy();
	});

	it('renders the Create profile submit button', () => {
		render(Page);
		expect(screen.getByText('Create profile')).toBeTruthy();
	});

	it('renders the Make profile public checkbox', () => {
		render(Page);
		expect(screen.getByText('Make profile public')).toBeTruthy();
	});
});
