<script lang="ts">
	// MultiAutocomplete — a multi-value picker built on the shared
	// Autocomplete: each pick is appended as a removable chip instead of
	// replacing a single selection. Use for fields whose value is a list
	// (e.g. intake months, tag ids).
	//
	// The component is controlled: `values` is the selected id list and
	// `onchange` fires with the new list on every add/remove. Labels for
	// chips come from `labels` plus any item picked during the session.
	import Autocomplete, { type AutocompleteItem } from './Autocomplete.svelte';
	import { X } from 'lucide-svelte';

	interface Props {
		/** Placeholder text for the search input. */
		placeholder: string;
		/** Empty-state message when no results match. */
		emptyMessage: string;
		/** Fetcher — called with the debounced query string. */
		fetcher: (q: string) => Promise<AutocompleteItem[]>;
		/** Optional fetcher for initial suggestions when focused with empty query. */
		initialFetcher?: () => Promise<AutocompleteItem[]>;
		/** Currently selected item ids. */
		values: string[];
		/** id → label map for chips not picked this session. */
		labels?: Record<string, string>;
		/** Fires with the new id list on every add/remove. */
		onchange?: (values: string[]) => void;
		/** Allow committing a typed value not in the option list. */
		allowFreeText?: boolean;
		/** Input id (for label association). */
		id?: string;
		/** Disable interaction. */
		disabled?: boolean;
		/** Extra classes on the outer wrapper. */
		class?: string;
		/** Accessible label for the clear-selection button (forwarded). */
		clearLabel?: string;
		/** Accessible label shown while loading (forwarded). */
		loadingLabel?: string;
		/** Accessible label for a chip's remove button. */
		removeLabel?: string;
	}

	let {
		placeholder,
		emptyMessage,
		fetcher,
		initialFetcher,
		values = [],
		labels = {},
		onchange,
		allowFreeText = false,
		id,
		disabled = false,
		class: wrapperClass = '',
		clearLabel = 'Clear',
		loadingLabel = 'Loading...',
		removeLabel = 'Remove'
	}: Props = $props();

	// Labels learned from picks this session — covers chips whose label is
	// not in the caller-provided map (e.g. an id seeded from a row).
	let learnedLabels = $state<Record<string, string>>({});
	const chipLabel = (id: string) => labels[id] ?? learnedLabels[id] ?? id;

	const filteredFetcher = async (q: string) =>
		(await fetcher(q)).filter((i) => !values.includes(i.id));
	const filteredInitial = async () =>
		initialFetcher ? (await initialFetcher()).filter((i) => !values.includes(i.id)) : [];

	function add(item: AutocompleteItem) {
		if (disabled || values.includes(item.id)) return;
		learnedLabels[item.id] = item.label;
		onchange?.([...values, item.id]);
	}

	function remove(id: string) {
		if (disabled) return;
		onchange?.(values.filter((v) => v !== id));
	}
</script>

<div class="multi-autocomplete {wrapperClass}">
	{#if values.length > 0}
		<div class="multi-autocomplete-chips">
			{#each values as v (v)}
				<span class="multi-autocomplete-chip">
					<span class="multi-autocomplete-chip-label">{chipLabel(v)}</span>
					<button
						type="button"
						class="multi-autocomplete-chip-remove"
						onclick={() => remove(v)}
						aria-label={removeLabel}
						{disabled}
					>
						<X size={11} />
					</button>
				</span>
			{/each}
		</div>
	{/if}
	<Autocomplete
		{id}
		{placeholder}
		{emptyMessage}
		fetcher={filteredFetcher}
		initialFetcher={initialFetcher ? filteredInitial : undefined}
		selectedId={null}
		selectedLabel={null}
		onSelect={add}
		{allowFreeText}
		{disabled}
		{clearLabel}
		{loadingLabel}
	/>
</div>

<style>
	.multi-autocomplete {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
	}
	.multi-autocomplete-chips {
		display: flex;
		flex-wrap: wrap;
		gap: 0.375rem;
	}
	.multi-autocomplete-chip {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		padding: 0.125rem 0.375rem 0.125rem 0.5rem;
		border-radius: 9999px;
		font-size: 0.75rem;
		line-height: 1.4;
		background: color-mix(in srgb, var(--color-primary, #3b82f6) 14%, transparent);
		color: var(--color-foreground, inherit);
	}
	.multi-autocomplete-chip-label {
		max-width: 16rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.multi-autocomplete-chip-remove {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		padding: 0.125rem;
		border-radius: 9999px;
		opacity: 0.6;
		cursor: pointer;
	}
	.multi-autocomplete-chip-remove:hover:not(:disabled) {
		opacity: 1;
	}
	.multi-autocomplete-chip-remove:disabled {
		cursor: not-allowed;
	}
</style>
