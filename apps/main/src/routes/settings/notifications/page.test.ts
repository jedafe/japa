import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

vi.mock('$app/forms', () => ({
	enhance: vi.fn()
}));

const mockData = {
	user: null,
	flags: {},
	notificationCount: 0,
	token: null,
	locale: 'en',
	translations: {},
	prefs: {
		email_notifications: { in_app: true, email: true, push: false },
		push_notifications: { in_app: true, email: false, push: true },
		follow_notifications: { in_app: true, email: false, push: false },
		comment_notifications: { in_app: true, email: false, push: false },
		mention_notifications: { in_app: true, email: false, push: false },
		dm_notifications: { in_app: true, email: false, push: false },
		group_notifications: { in_app: true, email: false, push: false },
		marketing_emails: { in_app: false, email: false, push: false }
	}
};

describe('SettingsNotifications +page.svelte', () => {
	it('renders the heading', () => {
		render(Page, { props: { data: mockData, form: null } });
		expect(screen.getByRole('heading', { name: /Notification Preferences/i })).toBeTruthy();
	});

	it('renders the subtitle text', () => {
		render(Page, { props: { data: mockData, form: null } });
		expect(screen.getByText('Choose how you want to be notified about activity on JapaConnect.')).toBeTruthy();
	});

	it('renders all notification type labels', () => {
		render(Page, { props: { data: mockData, form: null } });
		expect(screen.getByText('Email notifications')).toBeTruthy();
		expect(screen.getByText('Push notifications')).toBeTruthy();
		expect(screen.getByText('Follow notifications')).toBeTruthy();
		expect(screen.getByText('Comment notifications')).toBeTruthy();
		expect(screen.getByText('Mention notifications')).toBeTruthy();
		expect(screen.getByText('Direct message notifications')).toBeTruthy();
		expect(screen.getByText('Group notifications')).toBeTruthy();
		expect(screen.getByText('Marketing emails')).toBeTruthy();
	});

	it('renders the save button', () => {
		render(Page, { props: { data: mockData, form: null } });
		expect(screen.getByRole('button', { name: /Save preferences/i })).toBeTruthy();
	});

	it('renders the cancel link', () => {
		render(Page, { props: { data: mockData, form: null } });
		expect(screen.getByText('Cancel')).toBeTruthy();
	});
});
