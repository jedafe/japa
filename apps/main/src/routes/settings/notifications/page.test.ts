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
	prefs: {
		email_notifications: true,
		push_notifications: false,
		follow_notifications: true,
		comment_notifications: true,
		mention_notifications: false,
		dm_notifications: true,
		group_notifications: false,
		marketing_emails: false
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
