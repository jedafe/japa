import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	userManagementApi: {
		list: vi.fn().mockResolvedValue({ ok: true, data: { items: [], total: 0, page: 1, total_pages: 1 } }),
		issueWarning: vi.fn(),
		banUser: vi.fn(),
		deleteUser: vi.fn(),
	},
	impersonationApi: {
		status: vi.fn().mockResolvedValue({ ok: true, data: { impersonating: false } }),
		start: vi.fn(),
		end: vi.fn(),
	},
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn(),
}));

vi.mock('$lib/stores/permissions.svelte', () => ({
	hasPermission: vi.fn().mockReturnValue(false),
}));

vi.mock('$lib/stores/i18n.svelte', () => ({
	t: vi.fn((key: string, fallback: string) => fallback),
}));

describe('Users +page.svelte', () => {
	it('renders the users heading', () => {
		render(Page);
		expect(screen.getByText('Users')).toBeTruthy();
	});

	it('shows filter controls', () => {
		render(Page);
		expect(screen.getByText('Search')).toBeTruthy();
	});

	it('has search input', () => {
		render(Page);
		expect(screen.getByPlaceholderText('Search...')).toBeTruthy();
	});
});
