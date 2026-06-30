import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/stores/i18n.svelte', () => {
	const translations: Record<string, string> = {
		'contact.title': 'Contact Us',
		'search.find': 'Have a question, suggestion, or need support? Reach out to us.',
		'tickets.category': 'General Inquiries',
		'tickets.account': 'For account and general questions',
		'tickets.technical': 'Support',
		'tickets.billing': 'For technical issues and billing',
		'tickets.feature_request': 'Partnerships',
		'tickets.content': 'For partnership opportunities',
		'tickets.other': 'Security',
	};
	return { t: (key: string) => translations[key] ?? key };
});

describe('Contact +page.svelte', () => {
	it('renders the Contact Us heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: 'Contact Us' })).toBeTruthy();
	});

	it('shows the introductory text', () => {
		render(Page);
		expect(screen.getByText(/Have a question, suggestion, or need support/)).toBeTruthy();
	});

	it('shows the General Inquiries card', () => {
		render(Page);
		expect(screen.getByText('General Inquiries')).toBeTruthy();
		expect(screen.getByText('hello@japaconnect.com')).toBeTruthy();
	});

	it('shows the Support card', () => {
		render(Page);
		expect(screen.getByText('Support')).toBeTruthy();
		expect(screen.getByText('support@japaconnect.com')).toBeTruthy();
	});

	it('shows the Partnerships card', () => {
		render(Page);
		expect(screen.getByText('Partnerships')).toBeTruthy();
		expect(screen.getByText('partners@japaconnect.com')).toBeTruthy();
	});

	it('shows the Security card', () => {
		render(Page);
		expect(screen.getAllByText('Security').length).toBeGreaterThanOrEqual(1);
		expect(screen.getByText('security@japaconnect.com')).toBeTruthy();
	});
});
