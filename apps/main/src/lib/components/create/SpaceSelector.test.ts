import { describe, it, expect } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import SpaceSelector from './SpaceSelector.svelte';

const spaces = [
	{ id: 's1', slug: 'nigeria', name: 'Nigeria', is_primary: true },
	{ id: 's2', slug: 'uk', name: 'UK', is_primary: false }
];

describe('SpaceSelector', () => {
	it('renders a search input with placeholder when no space is selected', () => {
		const { getByRole } = render(SpaceSelector, {
			props: { spaces, selectedId: null, onSelect: () => {} }
		});
		const input = getByRole('combobox');
		expect(input).toBeTruthy();
		expect(input.getAttribute('placeholder')).toMatch(/search spaces/i);
	});

	it('opens the dropdown and shows all spaces when focused', async () => {
		const { getByRole, getAllByRole } = render(SpaceSelector, {
			props: { spaces, selectedId: null, onSelect: () => {} }
		});
		const input = getByRole('combobox');
		await fireEvent.focus(input);
		const options = getAllByRole('option');
		expect(options.length).toBe(2);
		expect(options[0].textContent).toContain('Nigeria');
		expect(options[1].textContent).toContain('UK');
	});

	it('marks the primary space with a star indicator', async () => {
		const { getByRole, getAllByRole } = render(SpaceSelector, {
			props: { spaces, selectedId: null, onSelect: () => {} }
		});
		const input = getByRole('combobox');
		await fireEvent.focus(input);
		const options = getAllByRole('option');
		expect(options[0].textContent).toContain('★');
	});

	it('calls onSelect with the chosen space id', async () => {
		let chosen: string | null = null;
		const { getByRole, getAllByRole } = render(SpaceSelector, {
			props: { spaces, selectedId: null, onSelect: (id: string) => (chosen = id) }
		});
		const input = getByRole('combobox');
		await fireEvent.focus(input);
		const options = getAllByRole('option');
		await fireEvent.mouseDown(options[1]);
		expect(chosen).toBe('s2');
	});
});
