import { describe, it, expect } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import SpaceSelector from './SpaceSelector.svelte';

const spaces = [
	{ id: 's1', slug: 'nigeria', name: 'Nigeria', is_primary: true },
	{ id: 's2', slug: 'uk', name: 'UK', is_primary: false },
	{ id: 's3', slug: 'brazil', name: 'Brazil', is_primary: false }
];

describe('SpaceSelector', () => {
	it('renders a search input with placeholder when no space is selected', () => {
		const { getByRole } = render(SpaceSelector, {
			props: { spaces, spaceIds: [], onChange: () => {} }
		});
		const input = getByRole('combobox');
		expect(input).toBeTruthy();
		expect(input.getAttribute('placeholder')).toMatch(/search spaces/i);
	});

	it('opens the dropdown and shows unselected spaces when focused', async () => {
		const { getByRole, getAllByRole } = render(SpaceSelector, {
			props: { spaces, spaceIds: ['s1'], onChange: () => {} }
		});
		const input = getByRole('combobox');
		await fireEvent.focus(input);
		const options = getAllByRole('option');
		expect(options.length).toBe(2);
		expect(options[0].textContent).toContain('UK');
		expect(options[1].textContent).toContain('Brazil');
	});

	it('marks the primary space with a star indicator in suggestions', async () => {
		const { getByRole, getAllByRole } = render(SpaceSelector, {
			props: { spaces, spaceIds: [], onChange: () => {} }
		});
		const input = getByRole('combobox');
		await fireEvent.focus(input);
		const options = getAllByRole('option');
		expect(options[0].textContent).toContain('★');
	});

	it('calls onChange with the existing selection plus the chosen space id', async () => {
		let chosen: string[] = [];
		const { getByRole, getAllByRole } = render(SpaceSelector, {
			props: { spaces, spaceIds: ['s1'], onChange: (ids: string[]) => (chosen = ids) }
		});
		const input = getByRole('combobox');
		await fireEvent.focus(input);
		const options = getAllByRole('option');
		await fireEvent.mouseDown(options[0]);
		expect(chosen).toEqual(['s1', 's2']);
	});

	it('does not select more than three spaces', async () => {
		let changed: string[] | null = null;
		const { getByRole, queryAllByRole } = render(SpaceSelector, {
			props: { spaces, spaceIds: ['s1', 's2', 's3'], onChange: (ids: string[]) => (changed = ids) }
		});
		const input = getByRole('combobox');
		await fireEvent.focus(input);
		// All spaces are already selected, so the dropdown should be empty.
		const options = queryAllByRole('option');
		expect(options.length).toBe(0);
		expect(changed).toBeNull();
	});

	it('shows selected spaces as chips below the field', () => {
		const { getByText } = render(SpaceSelector, {
			props: { spaces, spaceIds: ['s1', 's2'], onChange: () => {} }
		});
		expect(getByText('Nigeria')).toBeTruthy();
		expect(getByText('UK')).toBeTruthy();
	});
});
