import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

const mockData = {
	space: {
		id: '1',
		name: 'Nigerians in Toronto',
		description: 'A space for Nigerians living in Toronto.',
		slug: 'nigerians-in-toronto',
		space_type: 'country',
		member_count: 42,
		rules: [
			{ id: '1', title: 'Be respectful', description: 'Treat everyone with respect.' }
		]
	}
};

describe('SpaceDetail +page.svelte', () => {
	it('renders the space name', () => {
		render(Page, { props: { data: mockData } });
		expect(screen.getByRole('heading', { name: /Nigerians in Toronto/i })).toBeTruthy();
	});

	it('renders the member count', () => {
		render(Page, { props: { data: mockData } });
		expect(screen.getByText('42 members')).toBeTruthy();
	});

	it('renders the space type badge', () => {
		render(Page, { props: { data: mockData } });
		expect(screen.getByText('country')).toBeTruthy();
	});

	it('renders the join button', () => {
		render(Page, { props: { data: mockData } });
		expect(screen.getByRole('button', { name: /Join/i })).toBeTruthy();
	});

	it('renders space rules when present', () => {
		render(Page, { props: { data: mockData } });
		expect(screen.getByText('Space Rules')).toBeTruthy();
		expect(screen.getByText('Be respectful')).toBeTruthy();
	});
});
