import { describe, it, expect } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import EntityForm from './entity-form.svelte';
import type { FieldDef } from './index';

describe('EntityForm', () => {
	it('renders a labeled text input for type=text', () => {
		render(EntityForm, {
			fields: [{ key: 'name', label: 'Name', type: 'text', placeholder: 'e.g. javascript' }] as FieldDef[],
			value: { name: '' }
		});
		const input = screen.getByLabelText('Name') as HTMLInputElement;
		expect(input).toBeTruthy();
		expect(input.tagName).toBe('INPUT');
		expect(input.type).toBe('text');
		expect(input.placeholder).toBe('e.g. javascript');
	});

	it('renders a textarea for type=textarea', () => {
		render(EntityForm, {
			fields: [{ key: 'desc', label: 'Description', type: 'textarea' }] as FieldDef[],
			value: { desc: '' }
		});
		const el = screen.getByLabelText('Description') as HTMLElement;
		expect(el.tagName).toBe('TEXTAREA');
	});

	it('renders an autocomplete for type=select', () => {
		render(EntityForm, {
			fields: [{
				key: 'status',
				label: 'Status',
				type: 'select',
				options: [
					{ value: 'draft', label: 'Draft' },
					{ value: 'published', label: 'Published' }
				]
			}] as FieldDef[],
			value: { status: 'draft' }
		});
		const input = screen.getByLabelText('Status') as HTMLInputElement;
		expect(input.tagName).toBe('INPUT');
		expect(input.getAttribute('role')).toBe('combobox');
		// The selected option's label is shown as the autocomplete chip.
		expect(screen.getByText('Draft')).toBeTruthy();
	});

	it('renders a checkbox for type=checkbox', () => {
		render(EntityForm, {
			fields: [{ key: 'active', label: 'Active', type: 'checkbox' }] as FieldDef[],
			value: { active: false }
		});
		const cb = screen.getByLabelText('Active') as HTMLInputElement;
		expect(cb.tagName).toBe('INPUT');
		expect(cb.type).toBe('checkbox');
	});

	it('binds text input back into value', async () => {
		const value: Record<string, any> = { name: '' };
		render(EntityForm, {
			fields: [{ key: 'name', label: 'Name', type: 'text' }] as FieldDef[],
			value
		});
		await fireEvent.input(screen.getByLabelText('Name'), { target: { value: 'hello' } });
		expect(value.name).toBe('hello');
	});

	it('binds checkbox toggle back into value', async () => {
		const value: Record<string, any> = { active: false };
		render(EntityForm, {
			fields: [{ key: 'active', label: 'Active', type: 'checkbox' }] as FieldDef[],
			value
		});
		await fireEvent.click(screen.getByLabelText('Active'));
		expect(value.active).toBe(true);
	});

	it('binds autocomplete selection back into value for type=select', async () => {
		const value: Record<string, any> = { status: 'draft' };
		render(EntityForm, {
			fields: [{
				key: 'status',
				label: 'Status',
				type: 'select',
				options: [
					{ value: 'draft', label: 'Draft' },
					{ value: 'published', label: 'Published' }
				]
			}] as FieldDef[],
			value
		});
		await fireEvent.focus(screen.getByLabelText('Status'));
		// Autocomplete options are picked via mousedown (before input blur).
		const option = await screen.findByText('Published');
		await fireEvent.mouseDown(option);
		expect(value.status).toBe('published');
	});

	it('renders an error message below the field when errors[key] is set', () => {
		render(EntityForm, {
			fields: [{ key: 'name', label: 'Name', type: 'text' }] as FieldDef[],
			value: { name: '' },
			errors: { name: 'Name is required' }
		});
		expect(screen.getByText('Name is required')).toBeTruthy();
	});

	it('shows a required asterisk for required fields', () => {
		render(EntityForm, {
			fields: [{ key: 'name', label: 'Name', type: 'text', required: true }] as FieldDef[],
			value: { name: '' }
		});
		expect(screen.getByText('*')).toBeTruthy();
	});

	it('renders help text below the field when help is set', () => {
		render(EntityForm, {
			fields: [{ key: 'name', label: 'Name', type: 'text', help: 'Use a short slug.' }] as FieldDef[],
			value: { name: '' }
		});
		expect(screen.getByText('Use a short slug.')).toBeTruthy();
	});

	it('disables all inputs when disabled prop is true', () => {
		render(EntityForm, {
			fields: [{ key: 'name', label: 'Name', type: 'text' }] as FieldDef[],
			value: { name: '' },
			disabled: true
		});
		expect((screen.getByLabelText('Name') as HTMLInputElement).disabled).toBe(true);
	});

	it('renders number input with min/max/step attributes', () => {
		render(EntityForm, {
			fields: [{ key: 'age', label: 'Age', type: 'number', min: 0, max: 120, step: 1 }] as FieldDef[],
			value: { age: 0 }
		});
		const input = screen.getByLabelText('Age') as HTMLInputElement;
		expect(input.type).toBe('number');
		expect(input.min).toBe('0');
		expect(input.max).toBe('120');
		expect(input.step).toBe('1');
	});

	it('calls onsubmit when the form is submitted', async () => {
		let submitted = false;
		render(EntityForm, {
			fields: [{ key: 'name', label: 'Name', type: 'text' }] as FieldDef[],
			value: { name: '' },
			onsubmit: () => { submitted = true; }
		});
		const form = document.querySelector('form') as HTMLFormElement;
		expect(form).toBeTruthy();
		await fireEvent.submit(form);
		expect(submitted).toBe(true);
	});
});
