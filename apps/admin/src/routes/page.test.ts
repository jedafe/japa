import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import Page from './+page.svelte';

describe('Admin +page.svelte', () => {
	it('renders the admin heading', () => {
		const { getByText } = render(Page);
		expect(getByText('Admin Panel')).toBeTruthy();
	});
});
