import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

describe('SpacesNew +page.svelte', () => {
	it('renders the heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: /Create a space/i })).toBeTruthy();
	});

	it('renders the subtitle text', () => {
		render(Page);
		expect(screen.getByText('Build a community around a shared topic.')).toBeTruthy();
	});

	it('renders the form fields', () => {
		render(Page);
		expect(screen.getByLabelText('Name')).toBeTruthy();
		expect(screen.getByLabelText('Description')).toBeTruthy();
		expect(screen.getByLabelText('Type')).toBeTruthy();
	});

	it('renders the submit button', () => {
		render(Page);
		expect(screen.getByRole('button', { name: /Create space/i })).toBeTruthy();
	});

	it('renders the back link', () => {
		render(Page);
		expect(screen.getByText('Back to spaces')).toBeTruthy();
	});
});
