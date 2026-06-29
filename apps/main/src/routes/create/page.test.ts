import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

describe('Create +page.svelte', () => {
	it('renders the heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: /Create New Post/i })).toBeTruthy();
	});

	it('renders the subtitle text', () => {
		render(Page);
		expect(screen.getByText('Choose a content type to get started')).toBeTruthy();
	});

	it('renders the content type selector', () => {
		render(Page);
		expect(screen.getByText('Blog Post')).toBeTruthy();
		expect(screen.getByText('Discussion')).toBeTruthy();
		expect(screen.getByText('Guide')).toBeTruthy();
	});
});
