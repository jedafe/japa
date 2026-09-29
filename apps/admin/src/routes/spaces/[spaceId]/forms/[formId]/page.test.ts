import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected')),
	apiRaw: vi.fn().mockRejectedValue(new Error('not connected')),
	formsAdminApi: {
		delete: vi.fn().mockResolvedValue({ ok: true })
	},
	adminCatalogApi: {
		get: vi.fn().mockResolvedValue({ ok: true, data: [] })
	}
}));

vi.mock('$app/state', () => ({
	page: {
		params: { spaceId: 'test-space', formId: 'test-form' },
		url: new URL('http://localhost/spaces/test-space/forms/test-form')
	}
}));

vi.mock('$app/navigation', () => ({ goto: vi.fn() }));

vi.mock('$lib/stores/permissions.svelte', () => ({
	hasPermission: () => true,
	getPermissions: () => ['*'],
	getPermissionState: () => ({ permissions: ['*'], loaded: true })
}));

describe('Form Detail +page.svelte', () => {
	it('shows loading state initially', () => {
		render(Page);
		expect(screen.getByText('Loading...')).toBeTruthy();
	});

	it('shows Back link', () => {
		render(Page);
		expect(screen.getByText(/Back/)).toBeTruthy();
	});

	it('shows Delete button', () => {
		render(Page);
		expect(screen.getByText('Delete')).toBeTruthy();
	});
});
