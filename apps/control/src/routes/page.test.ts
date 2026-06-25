import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import Page from './+page.svelte';

describe('Control +page.svelte', () => {
	it('renders the control heading', () => {
		const { getByText } = render(Page);
		expect(getByText('Control Panel')).toBeTruthy();
	});
});
