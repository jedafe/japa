import { render } from '@testing-library/svelte';
import { it, expect } from 'vitest';
import CreateGuidePanel from './CreateGuidePanel.svelte';

it('shows series help when the series field is focused', () => {
	const { getByText } = render(CreateGuidePanel, { props: { focusedField: 'series' } });
	expect(getByText(/series/i)).toBeTruthy();
});

it('shows default help when no field is focused', () => {
	const { getByText } = render(CreateGuidePanel, { props: { focusedField: null } });
	expect(getByText(/welcome/i)).toBeTruthy();
});

it('shows space help when the space field is focused', () => {
	const { getByText } = render(CreateGuidePanel, { props: { focusedField: 'space' } });
	expect(getByText(/space/i)).toBeTruthy();
});
