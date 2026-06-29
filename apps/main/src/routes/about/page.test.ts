import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

describe('About +page.svelte', () => {
	it('renders the About JapaConnect heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: 'About JapaConnect' })).toBeTruthy();
	});

	it('shows the mission statement', () => {
		render(Page);
		expect(screen.getByText(/JapaConnect is a community platform built for the Nigerian diaspora/)).toBeTruthy();
	});

	it('shows the What We Offer heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: 'What We Offer' })).toBeTruthy();
	});

	it('lists the platform features', () => {
		render(Page);
		expect(screen.getByText(/Community spaces organized by country/)).toBeTruthy();
		expect(screen.getByText(/A rich content editor/)).toBeTruthy();
		expect(screen.getByText(/University discovery/)).toBeTruthy();
		expect(screen.getByText(/Real-time notifications/)).toBeTruthy();
		expect(screen.getByText(/Curated immigration/)).toBeTruthy();
	});
});
