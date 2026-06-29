import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';

// Import the stub before vi.mock hoists
const stubModule = await import('./ModalStub.svelte');

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected'))
}));

vi.mock('$app/stores', () => ({
	page: {
		subscribe: vi.fn((fn) => {
			fn({ params: { id: 'test-bot-id' } });
			return () => {};
		})
	}
}));

vi.mock('$lib/components/Modal.svelte', () => stubModule);

const { default: Page } = await import('./+page.svelte');

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
