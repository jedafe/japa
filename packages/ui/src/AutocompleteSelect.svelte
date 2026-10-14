<script lang="ts">
	// Drop-in replacement for a native <select>: the same value/options
	// contract, rendered through Autocomplete so every dropdown in the app
	// shares one searchable-list + selected-chip interaction.
	//
	// Differences from a bare Autocomplete worth noting:
	// - options is a static list — the fetchers are local filters, no API call.
	// - A '' option (e.g. "All types") survives selection: it maps to an
	//   internal sentinel so the chip still renders its label instead of
	//   falling back to the placeholder.
	// - `name` renders a hidden input so native <form> posts (FormData) keep
	//   working exactly like a named <select>.
	// - `clearable` shows the chip's clear X → value becomes ''.
	import Autocomplete, { type AutocompleteItem } from './Autocomplete.svelte';
	import { staticAutocompleteFetcher } from './autocomplete';

	interface Option {
		value: string;
		label: string;
		sublabel?: string;
	}

	interface Props {
		options: Option[];
		value?: string;
		/** Fires after value changes — replaces the native onchange handler. */
		onValueChange?: (v: string) => void;
		placeholder?: string;
		emptyMessage?: string;
		id?: string;
		/** Hidden-input name for native <form> submission (FormData). */
		name?: string;
		disabled?: boolean;
		/** Show the chip clear button — selecting nothing sets value=''. */
		clearable?: boolean;
		class?: string;
		inputClass?: string;
		ariaLabel?: string;
		/** Label used when `emptyMessage` is not provided. */
		noResultsLabel?: string;
		/** Accessible label for the chip's clear button (forwarded to Autocomplete). */
		clearLabel?: string;
		/** Accessible label for the loading spinner (forwarded to Autocomplete). */
		loadingLabel?: string;
		/** Open the dropdown above the field instead of below (forwarded to Autocomplete). */
		openUp?: boolean;
		/** Compact density for tight inline controls (forwarded to Autocomplete). */
		compact?: boolean;
	}

	let {
		options,
		value = $bindable(''),
		onValueChange,
		placeholder = '',
		emptyMessage,
		id,
		name,
		disabled = false,
		clearable = false,
		class: className = '',
		inputClass = '',
		ariaLabel,
		noResultsLabel = 'No results found',
		clearLabel,
		loadingLabel,
		openUp = false,
		compact = false
	}: Props = $props();

	// '' is not a usable Autocomplete item id (falsy = "no selection"), so
	// empty-string options get a sentinel id and map back to '' on commit.
	const NONE = '__none__';
	const hasNoneOption = $derived(options.some((o) => o.value === ''));

	// Dedupe by resolved id — two options mapping to the same id (e.g. an
	// API-provided "default" entry plus a locally prepended value:'' option)
	// would otherwise produce duplicate each-block keys and crash the render.
	const items = $derived<AutocompleteItem[]>(
		options
			.map((o) => ({
				id: o.value === '' ? NONE : o.value,
				label: o.label,
				sublabel: o.sublabel
			}))
			.filter((item, i, arr) => arr.findIndex((x) => x.id === item.id) === i)
	);
	const fetcher = $derived(staticAutocompleteFetcher(items));

	const selId = $derived(
		value === '' ? (hasNoneOption ? NONE : null) : value || null
	);
	// Fall back to the raw value so stale/legacy values still render instead
	// of an empty chip (a native <select> always shows *something*).
	const selLabel = $derived(items.find((i) => i.id === selId)?.label ?? (value || null));

	function commit(id: string) {
		const v = id === NONE ? '' : id;
		value = v;
		onValueChange?.(v);
	}
</script>

<Autocomplete
	{id}
	placeholder={placeholder || ' '}
	{ariaLabel}
	emptyMessage={emptyMessage ?? noResultsLabel}
	{fetcher}
	initialFetcher={fetcher}
	selectedId={selId}
	selectedLabel={selLabel}
	onSelect={(i) => commit(i.id)}
	onClear={clearable ? () => commit('') : undefined}
	{disabled}
	class={className}
	{inputClass}
	{clearLabel}
	{loadingLabel}
	{openUp}
	{compact}
/>
{#if name}
	<input type="hidden" {name} value={value ?? ''} />
{/if}
