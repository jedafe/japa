import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

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
		expect(screen.getByText('Security')).toBeTruthy();
		expect(screen.getByText('security@japaconnect.com')).toBeTruthy();
	});
});
