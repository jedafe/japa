import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	jsonLdApi: {
		listTemplates: vi.fn().mockResolvedValue({ ok: true, data: [] }),
		upsertTemplate: vi.fn().mockResolvedValue({ ok: true }),
		deleteTemplate: vi.fn().mockResolvedValue({ ok: true }),
	},
}));
vi.mock('$app/state', () => ({
	page: { params: {}, url: new URL('http://localhost/') },
}));
vi.mock('$app/navigation', () => ({ goto: vi.fn(), replaceState: vi.fn(), pushState: vi.fn() }));
vi.mock('$lib/stores/permissions.svelte', () => ({
	hasPermission: () => true,
	getPermissions: () => ['*'],
	getPermissionState: () => ({}),
}));

describe('/jsonld-templates', () => {
	it('renders the JSON-LD Templates heading', () => {
		render(Page);
		expect(screen.getByText('JSON-LD Templates')).toBeTruthy();
	});

	it('shows empty state when no templates', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No data available')).toBeTruthy();
		});
	});
});
