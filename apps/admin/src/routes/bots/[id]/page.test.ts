import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected'))
}));

vi.mock('$app/state', () => ({
	page: {
		params: { id: 'test-bot-id' },
		url: new URL('http://localhost/admin/bots/test-bot-id')
	}
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

describe('Bot Detail +page.svelte', () => {
	it('renders the Back to Bots link', () => {
		render(Page);
		expect(screen.getByText(/Back to Bots/)).toBeTruthy();
	});

	it('shows loading state initially', () => {
		render(Page);
		expect(screen.getByText('Loading...')).toBeTruthy();
	});
});
