import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null),
	api: {
		addRelocation: vi.fn().mockResolvedValue({}),
		addDocument: vi.fn().mockResolvedValue({})
	}
}));

vi.mock('$lib/stores/features.svelte', () => ({
	isFeatureEnabled: () => true,
	setFeatureFlags: () => {}
}));

vi.mock('$lib/components/Reviews.svelte', () => ({
	default: () => null
}));

const mockProfile = {
	id: '123',
	name: 'Test User',
	headline: 'Software Engineer',
	slug: 'test-user',
	skills: ['JavaScript', 'TypeScript'],
	is_public: true,
	author_user_id: 'user-1',
	created_at: '2026-01-01T00:00:00Z',
	updated_at: '2026-01-01T00:00:00Z',
	relocations: [],
	documents: [],
	verifiers: []
};

describe('Professional Profile +page.svelte', () => {
	const baseData = {
		user: null,
		flags: {},
		notificationCount: 0,
		token: null,
		locale: 'en',
		translations: {}
	};

	it('renders the profile name as heading', () => {
		render(Page, { props: { data: { ...baseData, profile: mockProfile } } });
		expect(screen.getByRole('heading', { name: /Test User/i })).toBeTruthy();
	});

	it('shows the headline text', () => {
		render(Page, { props: { data: { ...baseData, profile: mockProfile } } });
		expect(screen.getByText('Software Engineer')).toBeTruthy();
	});

	it('shows empty state for relocations', () => {
		render(Page, { props: { data: { ...baseData, profile: mockProfile } } });
		expect(screen.getByText('No relocations added yet.')).toBeTruthy();
	});

	it('shows empty state for documents', () => {
		render(Page, { props: { data: { ...baseData, profile: mockProfile } } });
		expect(screen.getByText('No documents added yet.')).toBeTruthy();
	});

	it('renders the back to profiles link', () => {
		render(Page, { props: { data: { ...baseData, profile: mockProfile } } });
		expect(screen.getByText('Back to profiles')).toBeTruthy();
	});
});
