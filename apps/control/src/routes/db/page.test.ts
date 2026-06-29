import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockResolvedValue({ ok: true, data: {} })
}));

describe('DB Health +page.svelte', () => {
	it('renders the db health heading', () => {
		render(Page);
		expect(screen.getByText('DB Health')).toBeTruthy();
	});

	it('shows the not-implemented fallback', () => {
		render(Page);
		expect(screen.getByText('DB health monitoring API is not implemented yet.')).toBeTruthy();
	});
});
