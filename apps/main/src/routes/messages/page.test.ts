import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null),
	api: {
		listDMConversations: vi.fn().mockRejectedValue(new Error('Not connected'))
	}
}));

vi.mock('$lib/stores/i18n.svelte', () => {
	const translations: Record<string, string> = {
		'messages.title': 'Messages',
		'messages.subtitle': 'Private conversations with other members.',
		'messages.no_conversations': 'No conversations yet',
		'messages.no_conversations_msg': 'Start a conversation.',
		'messages.not_available': 'Messages is not available',
		'messages.no_messages': 'No messages',
		'forum.feature_disabled': 'This feature is currently disabled.',
		'forum.go_home': 'Go home'
	};
	return { t: (key: string) => translations[key] ?? key };
});

vi.mock('$lib/stores/features.svelte', () => ({
	isFeatureEnabled: () => true,
	setFeatureFlags: () => {}
}));

vi.mock('$lib/components/ui', async () => {
	const { default: Comp } = await import('$lib/components/EmptyStateMock.svelte');
	return { EmptyState: Comp };
});

describe('Messages +page.svelte', () => {
	it('renders the Messages heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: /Messages/i })).toBeTruthy();
	});

	it('shows the subtitle text', () => {
		render(Page);
		expect(screen.getByText('Private conversations with other members.')).toBeTruthy();
	});

	it('renders the page structure with main element', () => {
		const { container } = render(Page);
		expect(container.querySelector('main')).toBeTruthy();
	});
});
