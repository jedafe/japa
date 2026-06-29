import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$app/state', () => ({
	page: {
		params: { audience: 'users', slug: 'getting-started' }
	}
}));

const mockDoc = {
	title: 'Getting Started',
	content: {
		type: 'doc',
		content: [
			{
				type: 'heading',
				attrs: { level: 2 },
				content: [{ type: 'text', text: 'Welcome' }]
			},
			{
				type: 'paragraph',
				content: [{ type: 'text', text: 'This is the getting started guide.' }]
			}
		]
	},
	audience: 'users',
	updated_at: '2026-01-15T10:00:00Z'
};

describe('Doc detail +page.svelte', () => {
	it('renders the doc title', () => {
		render(Page, { props: { data: { doc: mockDoc } } });
		expect(screen.getByRole('heading', { name: 'Getting Started' })).toBeTruthy();
	});

	it('renders the audience tag', () => {
		render(Page, { props: { data: { doc: mockDoc } } });
		expect(screen.getByText('users')).toBeTruthy();
	});

	it('renders the updated date', () => {
		render(Page, { props: { data: { doc: mockDoc } } });
		expect(screen.getByText(/updated/i)).toBeTruthy();
	});

	it('renders the doc content', () => {
		render(Page, { props: { data: { doc: mockDoc } } });
		expect(screen.getByText('Welcome')).toBeTruthy();
		expect(screen.getByText('This is the getting started guide.')).toBeTruthy();
	});

	it('renders back link to audience page', () => {
		render(Page, { props: { data: { doc: mockDoc } } });
		const backLink = screen.getByText('Back to docs');
		expect(backLink.closest('a')?.getAttribute('href')).toBe('/users');
	});

	it('shows not found state when doc is null', () => {
		render(Page, { props: { data: { doc: null } } });
		expect(screen.getByRole('heading', { name: /page not found/i })).toBeTruthy();
		expect(screen.getByText(/doesn't exist or isn't published yet/i)).toBeTruthy();
	});
});
