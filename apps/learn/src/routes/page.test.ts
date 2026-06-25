import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import Page from './+page.svelte';

describe('Learn +page.svelte', () => {
	it('renders the learn heading', () => {
		const { getByText } = render(Page);
		expect(getByText('JapaConnect Learn')).toBeTruthy();
	});
});
