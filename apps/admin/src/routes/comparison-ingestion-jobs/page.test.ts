import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api/comparison-data-sources', () => ({
	listIngestionJobs: vi.fn().mockResolvedValue([]),
	listDataSources: vi.fn().mockResolvedValue([])
}));

vi.mock('$app/state', () => ({
	page: {
		params: {},
		url: new URL('http://localhost/comparison-ingestion-jobs?content_id=test-content-id')
	}
}));
vi.mock('$app/navigation', () => ({ goto: vi.fn(), replaceState: vi.fn(), pushState: vi.fn() }));
vi.mock('$lib/stores/permissions.svelte', () => ({
	hasPermission: () => true,
	getPermissions: () => ['*'],
	getPermissionState: () => ({})
}));

describe('/comparison-ingestion-jobs', () => {
	it('renders the Comparison refresh jobs heading', () => {
		render(Page);
		expect(screen.getByText('Comparison refresh jobs')).toBeTruthy();
	});

	it('shows empty state when no jobs', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('No refresh jobs found for this comparison')).toBeTruthy();
		});
	});
});
