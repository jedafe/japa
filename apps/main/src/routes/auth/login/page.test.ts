import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

describe('AuthLogin +page.svelte', () => {
	it('renders the redirecting message', () => {
		render(Page);
		expect(screen.getByText('Redirecting to login...')).toBeTruthy();
	});
});
