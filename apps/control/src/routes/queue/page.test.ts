import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected'))
}));

describe('Queue +page.svelte', () => {
	it('renders the queue monitor heading', () => {
		render(Page);
		expect(screen.getByText('Queue Monitor')).toBeTruthy();
	});

	it('shows queue tier cards', () => {
		render(Page);
		expect(screen.getByText('critical queue')).toBeTruthy();
		expect(screen.getByText('default queue')).toBeTruthy();
		expect(screen.getByText('low queue')).toBeTruthy();
		expect(screen.getByText('Dead Letter')).toBeTruthy();
	});

	it('shows recent tasks section', () => {
		render(Page);
		expect(screen.getByText('Recent Tasks')).toBeTruthy();
	});
});
