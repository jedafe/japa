import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/stores/i18n.svelte', () => {
	const translations: Record<string, string> = {
		'about.title': 'About JapaConnect',
		'about.mission_text_1': 'JapaConnect is a community platform built for the Nigerian diaspora',
		'about.mission_text_2': 'mission text 2',
		'about.offer_heading': 'What We Offer',
		'about.offer_1': 'Community spaces organized by country',
		'about.offer_2': 'A rich content editor',
		'about.offer_3': 'University discovery',
		'about.offer_4': 'Real-time notifications',
		'about.offer_5': 'Curated immigration'
	};
	return { t: (key: string) => translations[key] ?? key };
});

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
