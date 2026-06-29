import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	api: vi.fn().mockRejectedValue(new Error('not connected'))
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

describe('Create Custom Field +page.svelte', () => {
	it('renders the Create Custom Field heading', () => {
		render(Page);
		expect(screen.getByText('Create Custom Field')).toBeTruthy();
	});

	it('renders the subtitle', () => {
		render(Page);
		expect(screen.getByText('Define a new field for an entity type')).toBeTruthy();
	});

	it('shows form fields and Create Field button', () => {
		render(Page);
		expect(screen.getByLabelText('Name')).toBeTruthy();
		expect(screen.getByLabelText('Slug')).toBeTruthy();
		expect(screen.getByLabelText('Field Type')).toBeTruthy();
		expect(screen.getByLabelText('Entity Type')).toBeTruthy();
		expect(screen.getByText('Create Field')).toBeTruthy();
	});
});
