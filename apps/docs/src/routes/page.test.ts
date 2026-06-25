import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import Page from './+page.svelte';

describe('Docs +page.svelte', () => {
	it('renders the docs heading', () => {
		const { getByText } = render(Page);
		expect(getByText('JapaConnect Docs')).toBeTruthy();
	});
});
