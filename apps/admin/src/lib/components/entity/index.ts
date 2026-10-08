export { default as EntityForm } from './entity-form.svelte';
export { default as EntityPage } from './entity-page.svelte';
export type { EntityPageConfig, EntityPageController } from './entity-page.svelte';

// Per-row lock props shared by every FieldDef variant. `disabledWhen` is
// evaluated against the editing row (null on create — the predicate is never
// called there); when it returns true the control renders disabled and
// `lockedHelp`, if set, replaces `help` so the form can explain the lock.
type RowLockProps = {
	/** Returns true when the field must render disabled for the row being
	 *  edited (e.g. a status derived from a linked feature flag). */
	disabledWhen?: (editing: any) => boolean;
	/** Help text shown in place of `help` while the field is row-locked. */
	lockedHelp?: string;
};

export type FieldDef =
	| ({
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
			/** Omit the field from the edit form (still seeded into the update
			 *  body and rendered on create). For columns the update endpoint
			 *  cannot change — e.g. coupon_type is absent from UpdateCoupon's
			 *  SET clause (§55). */
			hideInEdit?: boolean;
	  } & RowLockProps)
	| ({
			key: string;
			label: string;
			type: 'select';
			options: Array<{ value: string; label: string }>;
			placeholder?: string;
			required?: boolean;
			help?: string;
			class?: string;
			readonlyInEdit?: boolean;
			hideInEdit?: boolean;
	  } & RowLockProps)
	| ({
			key: string;
			label: string;
			type: 'autocomplete';
			placeholder: string;
			emptyMessage: string;
			/** Fetcher — called with the debounced query string. */
			fetcher: (q: string) => Promise<{ id: string; label: string; sublabel?: string }[]>;
			/** Optional fetcher for initial suggestions when focused with empty query. */
			initialFetcher?: () => Promise<{ id: string; label: string; sublabel?: string }[]>;
			/** Resolves the currently selected id to a display label (avoids re-fetch on render). */
			getSelectedLabel: (id: string) => string;
			/** Drops the option matching the editing row's id — for
			 *  self-referential pickers (e.g. a parent select that must not
			 *  offer the row itself). */
			excludeSelf?: boolean;
			required?: boolean;
			help?: string;
			class?: string;
			readonlyInEdit?: boolean;
			hideInEdit?: boolean;
	  } & RowLockProps);
