import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import Page from './+page.svelte';

describe('Docs landing page', () => {
	it('renders the docs heading', () => {
		const { getByText } = render(Page);
		expect(getByText('JapaConnect Documentation')).toBeTruthy();
	});

	it('shows all audience paths', () => {
		const { getByText } = render(Page);
		expect(getByText('User Guide')).toBeTruthy();
		expect(getByText('Premium')).toBeTruthy();
		expect(getByText('Organizations')).toBeTruthy();
		expect(getByText('Moderators')).toBeTruthy();
		expect(getByText('API Reference')).toBeTruthy();
		expect(getByText('Admin')).toBeTruthy();
	});
});
