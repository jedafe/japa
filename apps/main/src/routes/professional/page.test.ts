import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

vi.mock('$app/state', () => ({
	page: {
		url: {
			searchParams: {
				get: vi.fn().mockReturnValue(null)
			}
		}
	}
}));

vi.mock('$lib/stores/i18n.svelte', () => {
	const translations: Record<string, string> = {
		'professional.title': 'Professional Profiles',
		'professional.subtitle': 'Discover professionals ready for their next chapter abroad.',
		'professional.create': 'Create profile',
		'professional.search_placeholder': 'Search by name or skill...',
		'professional.search': 'Search',
		'professional.none': 'No professional profiles yet',
		'professional.none_msg': 'Be the first.',
		'professional.not_available': 'Professional Profiles is not available',
		'professional.public': 'Public',
		'professional.private': 'Private',
		'professional.showing': 'Showing {count} of {total}',
		'forum.feature_disabled': 'This feature is currently disabled.',
		'forum.go_home': 'Go home'
	};
	return { t: (key: string) => translations[key] ?? key };
});

vi.mock('$lib/stores/features.svelte', () => ({
	isFeatureEnabled: () => true,
	setFeatureFlags: () => {}
}));

vi.mock('@formkit/auto-animate', () => ({
	default: () => {}
}));

vi.mock('$lib/components/ui', async () => {
	const { default: Comp } = await import('$lib/components/EmptyStateMock.svelte');
	return { EmptyState: Comp };
});

describe('Professional Profiles +page.svelte', () => {
	const defaultData = {
		user: null,
		flags: {},
		notificationCount: 0,
		token: null,
		locale: 'en',
		translations: {},
		profiles: [],
		filters: { search: '' },
		total: 0,
		totalPages: 1
	};

	it('renders the Professional Profiles heading', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByRole('heading', { name: 'Professional Profiles' })).toBeTruthy();
	});

	it('shows the subtitle text', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('Discover professionals ready for their next chapter abroad.')).toBeTruthy();
	});

	it('renders the Create profile button', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('Create profile')).toBeTruthy();
	});

	it('renders the search input', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByPlaceholderText('Search by name or skill...')).toBeTruthy();
	});

	it('shows empty state when no profiles exist', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('No professional profiles yet')).toBeTruthy();
	});
});
