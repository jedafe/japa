import { render } from '@testing-library/svelte';
import { it, expect } from 'vitest';
import CreateGuidePanel from './CreateGuidePanel.svelte';

const baseProps = {
	series: [],
	seriesId: null,
	coverMediaId: null,
	coverPreviewUrl: null,
	onseries: () => {},
	onseriesfocus: () => {},
	oncoveruploaded: () => {},
	oncoverremove: () => {},
	onedit: () => {}
};

it('shows series help when the series field is focused', () => {
	const { getByText } = render(CreateGuidePanel, { props: { ...baseProps, focusedField: 'series' } });
	expect(getByText(/series/i)).toBeTruthy();
});

it('shows default help when no field is focused', () => {
	const { getByText } = render(CreateGuidePanel, { props: { ...baseProps, focusedField: null } });
	expect(getByText(/welcome/i)).toBeTruthy();
});

it('shows space help when the space field is focused', () => {
	const { getByText } = render(CreateGuidePanel, { props: { ...baseProps, focusedField: 'space' } });
	expect(getByText(/space/i)).toBeTruthy();
});
