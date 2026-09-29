import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { EntityPage } from './index';
import type { EntityPageConfig } from './index';

vi.mock('$lib/api', () => ({}));
vi.mock('$app/state', () => ({ page: { params: {}, url: new URL('http://localhost/') } }));
vi.mock('$app/navigation', () => ({ goto: vi.fn(), replaceState: vi.fn(), pushState: vi.fn() }));
vi.mock('$lib/stores/permissions.svelte', () => ({
	hasPermission: () => true,
	hasAllPermissions: () => true,
	hasAnyPermission: () => true,
	getPermissions: () => ['*'],
	getPermissionState: () => ({ permissions: ['*'], loaded: true }),
	isPermissionsLoaded: () => true,
	setPermissions: vi.fn(),
	fetchPermissions: vi.fn().mockResolvedValue(['*'])
}));

function makeConfig(overrides: Partial<EntityPageConfig> = {}): EntityPageConfig {
	return {
		title: 'Tags',
		description: 'Manage platform tags',
		columns: [{ key: 'name', label: 'Name', sortable: true }],
		fields: [{ key: 'name', label: 'Name', type: 'text' }],
		createLabel: 'Create Tag',
		load: vi.fn().mockResolvedValue({ data: [{ id: '1', name: 'Alpha' }], total: 1, totalPages: 1 }),
		create: vi.fn().mockResolvedValue({ ok: true }),
		update: vi.fn().mockResolvedValue({ ok: true }),
		remove: vi.fn().mockResolvedValue({ ok: true }),
		...overrides
	} as EntityPageConfig;
}

describe('EntityPage', () => {
	it('renders the title and calls load on mount', async () => {
		const config = makeConfig();
		render(EntityPage, { config });
		expect(screen.getByText('Tags')).toBeTruthy();
		await waitFor(() => expect(config.load).toHaveBeenCalled());
	});

	it('renders rows from the load result', async () => {
		const config = makeConfig();
		render(EntityPage, { config });
		await waitFor(() => expect(screen.getByText('Alpha')).toBeTruthy());
	});

	it('shows a Create button that opens the form slide-over', async () => {
		const config = makeConfig();
		render(EntityPage, { config });
		await waitFor(() => expect(screen.getByText('Alpha')).toBeTruthy());
		const createBtn = screen.getByRole('button', { name: 'Create Tag' });
		await fireEvent.click(createBtn);
		// SlideOver opens with the form field label rendered inside EntityForm.
		await waitFor(() => expect(screen.getByLabelText('Name')).toBeTruthy());
	});

	it('submits the create form and calls config.create', async () => {
		const config = makeConfig();
		render(EntityPage, { config });
		await waitFor(() => expect(screen.getByText('Alpha')).toBeTruthy());
		await fireEvent.click(screen.getByRole('button', { name: 'Create Tag' }));
		await waitFor(() => expect(screen.getByLabelText('Name')).toBeTruthy());
		await fireEvent.input(screen.getByLabelText('Name'), { target: { value: 'Beta' } });
		// Submit the EntityForm via the Save button (form attribute) or form submit.
		const form = document.querySelector('form') as HTMLFormElement;
		expect(form).toBeTruthy();
		await fireEvent.submit(form);
		await waitFor(() => expect(config.create).toHaveBeenCalled());
		expect((config.create as any).mock.calls[0][0]).toMatchObject({ name: 'Beta' });
	});

	it('Edit action opens the form seeded with the row and calls config.update on submit', async () => {
		const config = makeConfig();
		const { container } = render(EntityPage, { config });
		await waitFor(() => expect(screen.getByText('Alpha')).toBeTruthy());
		// Open the row action menu.
		const trigger = container.querySelector('[data-action-menu-trigger]') as HTMLElement;
		expect(trigger).toBeTruthy();
		await fireEvent.click(trigger);
		await waitFor(() => expect(screen.getByText('Edit')).toBeTruthy());
		await fireEvent.click(screen.getByText('Edit'));
		// Form slide-over opens with the row value seeded.
		await waitFor(() => expect(screen.getByLabelText('Name')).toBeTruthy());
		expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('Alpha');
		await fireEvent.submit(document.querySelector('form') as HTMLFormElement);
		await waitFor(() => expect(config.update).toHaveBeenCalledWith('1', expect.objectContaining({ name: 'Alpha' })));
	});

	it('Delete action opens the confirm modal and Confirm calls config.remove', async () => {
		const config = makeConfig();
		const { container } = render(EntityPage, { config });
		await waitFor(() => expect(screen.getByText('Alpha')).toBeTruthy());
		const trigger = container.querySelector('[data-action-menu-trigger]') as HTMLElement;
		await fireEvent.click(trigger);
		await waitFor(() => expect(screen.getByText('Delete')).toBeTruthy());
		await fireEvent.click(screen.getByText('Delete'));
		// Confirm modal opens — find the Confirm button inside the modal.
		await waitFor(() => expect(screen.getByRole('button', { name: 'Confirm' })).toBeTruthy());
		await fireEvent.click(screen.getByRole('button', { name: 'Confirm' }));
		await waitFor(() => expect(config.remove).toHaveBeenCalledWith('1'));
	});

	it('hides the Create button when readonly=true', async () => {
		const config = makeConfig({ readonly: true });
		render(EntityPage, { config });
		await waitFor(() => expect(screen.getByText('Alpha')).toBeTruthy());
		expect(screen.queryByRole('button', { name: 'Create Tag' })).toBeNull();
	});

	it('hides the Create button when hideCreate=true but keeps edit/delete flows', async () => {
		const config = makeConfig({ hideCreate: true });
		const { container } = render(EntityPage, { config });
		await waitFor(() => expect(screen.getByText('Alpha')).toBeTruthy());
		expect(screen.queryByRole('button', { name: 'Create Tag' })).toBeNull();
		// Unlike readonly, edit/delete row actions remain available.
		const trigger = container.querySelector('[data-action-menu-trigger]') as HTMLElement;
		await fireEvent.click(trigger);
		await waitFor(() => expect(screen.getByText('Edit')).toBeTruthy());
	});
});
