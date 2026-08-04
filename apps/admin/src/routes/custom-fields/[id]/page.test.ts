import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
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

vi.mock('$app/state', () => ({
	page: {
		params: { id: 'test-id' },
		url: new URL('http://localhost/custom-fields/test-id')
	}
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

describe('Edit Custom Field +page.svelte', () => {
	it('renders the loading heading', () => {
		render(Page);
		expect(screen.getByText('Loading…')).toBeTruthy();
	});

	it('renders the breadcrumb back link', () => {
		render(Page);
		expect(screen.getByText('Field Groups')).toBeTruthy();
	});

	it('shows loading state initially', () => {
		render(Page);
		expect(screen.getAllByText(/Loading/).length).toBeGreaterThan(0);
	});
});
