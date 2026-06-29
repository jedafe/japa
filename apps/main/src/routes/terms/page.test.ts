import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

describe('Terms of Service +page.svelte', () => {
	it('renders the Terms of Service heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: 'Terms of Service' })).toBeTruthy();
	});

	it('shows the last updated date', () => {
		render(Page);
		expect(screen.getByText('Last updated: June 2026')).toBeTruthy();
	});

	it('renders the Acceptance of Terms section', () => {
		render(Page);
		expect(screen.getByText('1. Acceptance of Terms')).toBeTruthy();
	});
});
