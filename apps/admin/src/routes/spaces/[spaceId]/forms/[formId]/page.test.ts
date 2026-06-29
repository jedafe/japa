import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected'))
}));

vi.mock('$app/stores', () => ({
	page: {
		subscribe: (fn: Function) => {
			fn({ params: { spaceId: 'test-space', formId: 'test-form' } });
			return () => {};
		}
	}
}));

vi.mock('$lib/components/Modal.svelte', async () => {
	const { default: Stub } = await import('$lib/components/Modal.stub.svelte');
	return { default: Stub };
});

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
