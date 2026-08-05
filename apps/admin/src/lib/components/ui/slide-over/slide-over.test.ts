import { describe, it, expect } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import SlideOver from './slide-over.svelte';
import SlideOverWrapper from './slide-over-test-wrapper.svelte';

describe('SlideOver', () => {
	it('does not render content when closed', () => {
		const { container } = render(SlideOver, { open: false, title: 'Edit Tag' });
		expect(container.querySelector('[data-slideover-content]')).toBeNull();
		expect(document.body.querySelector('[data-slideover-content]')).toBeNull();
	});

	it('renders title and body when open', async () => {
		render(SlideOverWrapper, { open: true, title: 'Edit Tag', body: 'form fields' });
		await waitFor(() => {
			expect(screen.getByText('Edit Tag')).toBeTruthy();
		});
		expect(screen.getByText('form fields')).toBeTruthy();
	});

	it('renders footer snippet when provided', async () => {
		render(SlideOverWrapper, {
			open: true,
			title: 'Edit',
			body: '',
			footerText: 'Save'
		});
		await waitFor(() => {
			expect(screen.getByRole('button', { name: 'Save' })).toBeTruthy();
		});
	});
});
