import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null),
	api: {
		createContent: vi.fn().mockRejectedValue(new Error('Not connected')),
		updateContent: vi.fn().mockRejectedValue(new Error('Not connected'))
	}
}));

vi.mock('$app/state', () => ({
	page: {
		params: { content_type: 'invalid_type' }
	}
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

describe('CreateContentType +page.svelte', () => {
	it('renders the not found heading for invalid content type', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: /Content type not found/i })).toBeTruthy();
	});

	it('shows the invalid type message', () => {
		render(Page);
		expect(screen.getByText(/does not exist/)).toBeTruthy();
	});

	it('renders the back link to choose another type', () => {
		render(Page);
		expect(screen.getByText('Choose another type')).toBeTruthy();
	});
});
