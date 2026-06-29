import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null),
	api: {
		getDMMessages: vi.fn().mockRejectedValue(new Error('Not connected')),
		listDMConversations: vi.fn().mockRejectedValue(new Error('Not connected')),
		sendDMMessage: vi.fn().mockRejectedValue(new Error('Not connected'))
	}
}));

vi.mock('$app/state', () => ({
	page: {
		params: { channelId: 'channel-123' }
	}
}));

describe('Message Channel +page.svelte', () => {
	it('renders the chat header with default name', () => {
		render(Page);
		expect(screen.getByText('Chat')).toBeTruthy();
	});

	it('shows the message input textarea', () => {
		render(Page);
		expect(screen.getByPlaceholderText('Type a message...')).toBeTruthy();
	});

	it('renders the send button (disabled when empty)', () => {
		const { container } = render(Page);
		const sendButton = container.querySelector('button[type="submit"]');
		expect(sendButton).toBeTruthy();
		expect(sendButton?.getAttribute('disabled')).not.toBeNull();
	});

	it('has a link back to messages', () => {
		render(Page);
		const backLink = screen.getByRole('link');
		expect(backLink.getAttribute('href')).toBe('/messages');
	});
});
