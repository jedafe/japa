export { default as EntityForm } from './entity-form.svelte';
export { default as EntityPage } from './entity-page.svelte';
export type { EntityPageConfig, EntityPageController } from './entity-page.svelte';

export type FieldDef =
	| {
			key: string;
			label: string;
			type: 'text' | 'textarea' | 'number' | 'email' | 'password' | 'date' | 'datetime-local' | 'url' | 'checkbox' | 'color';
			placeholder?: string;
			required?: boolean;
			min?: number;
			max?: number;
			step?: number;
			help?: string;
			class?: string;
			readonlyInEdit?: boolean;
	  }
	| {
			key: string;
			label: string;
			type: 'select';
			options: Array<{ value: string; label: string }>;
			placeholder?: string;
			required?: boolean;
			help?: string;
			class?: string;
			readonlyInEdit?: boolean;
	  };
