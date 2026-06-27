import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

describe('Broadcast +page.svelte', () => {
	it('renders the broadcast heading', () => {
		render(Page);
		expect(screen.getByText('Notification Broadcast')).toBeTruthy();
	});

	it('has title input', () => {
		render(Page);
		expect(screen.getByPlaceholderText('Notification title')).toBeTruthy();
	});

	it('has message textarea', () => {
		render(Page);
		expect(screen.getByPlaceholderText('Notification body')).toBeTruthy();
	});

	it('has target audience selector', () => {
		render(Page);
		expect(screen.getByText('Target Audience')).toBeTruthy();
	});

	it('has send button', () => {
		render(Page);
		expect(screen.getByText('Send Now')).toBeTruthy();
	});
});
