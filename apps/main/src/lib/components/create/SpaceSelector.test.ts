import { describe, it, expect } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import SpaceSelector from './SpaceSelector.svelte';

const spaces = [
	{ id: 's1', slug: 'nigeria', name: 'Nigeria', is_primary: true },
	{ id: 's2', slug: 'uk', name: 'UK', is_primary: false }
];

describe('SpaceSelector', () => {
	it('renders all spaces as options', () => {
		const { getByRole } = render(SpaceSelector, {
			props: { spaces, selectedId: null, onSelect: () => {} }
		});
		const select = getByRole('combobox') as HTMLSelectElement;
		expect(select.options.length).toBe(3); // placeholder + 2 spaces
		expect(select.options[1].textContent).toContain('Nigeria');
		expect(select.options[2].textContent).toBe('UK');
	});

	it('marks the primary space with an indicator', () => {
		const { getByRole } = render(SpaceSelector, {
			props: { spaces, selectedId: null, onSelect: () => {} }
		});
		const select = getByRole('combobox') as HTMLSelectElement;
		expect(select.options[1].textContent).toContain('★');
	});

	it('calls onSelect with the chosen space id', async () => {
		let chosen: string | null = null;
		const { getByRole } = render(SpaceSelector, {
			props: { spaces, selectedId: null, onSelect: (id: string) => (chosen = id) }
		});
		const select = getByRole('combobox') as HTMLSelectElement;
		await fireEvent.change(select, { target: { value: 's2' } });
		expect(chosen).toBe('s2');
	});
});
