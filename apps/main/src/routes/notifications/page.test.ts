import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null),
	api: {
		getNotifications: vi.fn().mockResolvedValue([]),
		markNotificationRead: vi.fn().mockResolvedValue({}),
		markAllRead: vi.fn().mockResolvedValue({})
	}
}));

vi.mock('$lib/websocket', () => ({
	wsClient: {
		on: vi.fn().mockReturnValue(() => {})
	}
}));

vi.mock('$lib/stores/realtime.svelte', () => ({
	realtime: {
		setNotificationCount: vi.fn()
	}
}));

vi.mock('$lib/stores/i18n.svelte', () => {
	const translations: Record<string, string> = {
		'notifications.title': 'Notifications',
		'notifications.mark_all_read': 'Mark all read',
		'notifications.empty_title': 'All caught up!',
		'notifications.empty_message': "You'll see notifications here when there's activity.",
		'notifications.unread': '{count} unread'
	};
	return { t: (key: string) => translations[key] ?? key };
});

vi.mock('@formkit/auto-animate', () => ({
	default: () => {}
}));

vi.mock('$lib/components/ui', async () => {
	const { default: Comp } = await import('$lib/components/EmptyStateMock.svelte');
	return { EmptyState: Comp };
});

describe('Notifications +page.svelte', () => {
	it('renders the Notifications heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: 'Notifications' })).toBeTruthy();
	});

	it('shows empty state after load with no notifications', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('All caught up!')).toBeTruthy();
		});
	});

	it('shows the empty state message', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText("You'll see notifications here when there's activity.")).toBeTruthy();
		});
	});
});
