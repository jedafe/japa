import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockResolvedValue({ ok: true, data: {} })
}));

describe('Queue +page.svelte', () => {
	it('renders the queue monitor heading', () => {
		render(Page);
		expect(screen.getByText('Queue Monitor')).toBeTruthy();
	});

	it('shows the not-implemented fallback', () => {
		render(Page);
		expect(screen.getByText('Queue monitoring API is not implemented yet.')).toBeTruthy();
	});
});
