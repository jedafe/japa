import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$app/state', () => ({
	page: {
		params: { audience: 'users' }
	}
}));

const mockPages = [
	{ id: '1', title: 'Getting Started', slug: 'getting-started', review_status: 'published', position: 1 },
	{ id: '2', title: 'Profiles', slug: 'profiles', review_status: 'published', position: 2 },
];

describe('Docs audience +page.svelte', () => {
	it('renders the section label as heading', () => {
		render(Page, { props: { data: { pages: mockPages } } });
		expect(screen.getByRole('heading', { name: /user guide/i })).toBeTruthy();
	});

	it('renders page titles as links', () => {
		render(Page, { props: { data: { pages: mockPages } } });
		expect(screen.getByText('Getting Started')).toBeTruthy();
		expect(screen.getByText('Profiles')).toBeTruthy();
	});

	it('links each page to the correct audience slug', () => {
		render(Page, { props: { data: { pages: mockPages } } });
		const links = screen.getAllByRole('link');
		const docLinks = links.filter((l) => l.getAttribute('href')?.startsWith('/users/'));
		expect(docLinks).toHaveLength(2);
		expect(docLinks[0].getAttribute('href')).toBe('/users/getting-started');
		expect(docLinks[1].getAttribute('href')).toBe('/users/profiles');
	});

	it('shows empty state when no pages exist', () => {
		render(Page, { props: { data: { pages: [] } } });
		expect(screen.getByText(/no documentation pages available yet/i)).toBeTruthy();
	});

	it('shows description text', () => {
		render(Page, { props: { data: { pages: mockPages } } });
		expect(screen.getByText(/documentation for the user guide section/i)).toBeTruthy();
	});
});
