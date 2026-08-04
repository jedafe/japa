import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected')),
	fieldGroupsAdminApiExtended: {
		list: vi.fn().mockRejectedValue(new Error('not connected')),
		create: vi.fn().mockRejectedValue(new Error('not connected')),
		update: vi.fn().mockRejectedValue(new Error('not connected')),
		delete: vi.fn().mockRejectedValue(new Error('not connected')),
		addField: vi.fn().mockRejectedValue(new Error('not connected')),
		updateField: vi.fn().mockRejectedValue(new Error('not connected')),
		deleteField: vi.fn().mockRejectedValue(new Error('not connected')),
	},
	userManagementApi: {
		list: vi.fn().mockRejectedValue(new Error('not connected')),
		listCustomFieldEntityTypes: vi.fn().mockRejectedValue(new Error('not connected')),
	}
}));

vi.mock('$lib/stores/permissions.svelte', () => ({
	permissions: [],
	hasPermission: vi.fn().mockReturnValue(true)
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

describe('Custom Fields +page.svelte', () => {
	it('renders the Custom Fields heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: 'Field Groups' })).toBeTruthy();
	});

	it('renders the subtitle', () => {
		render(Page);
		expect(screen.getByText('Manage custom field groups')).toBeTruthy();
	});

	it('shows Create Field link', () => {
		render(Page);
		expect(screen.getByText('Create Group')).toBeTruthy();
	});

	it('shows empty state when no fields', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No field groups')).toBeTruthy();
		});
	});
});
