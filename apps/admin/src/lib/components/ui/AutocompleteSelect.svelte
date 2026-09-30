<script lang="ts">
	// Drop-in replacement for a native <select>: the same value/options
	// contract, rendered through Autocomplete so every dropdown in the app
	// shares one searchable-list + selected-chip interaction.
	//
	// Differences from a bare Autocomplete worth noting:
	// - options is a static list — the fetchers are local filters, no API call.
	// - A '' option (e.g. "All") survives selection: it maps to an internal
	//   sentinel so the chip still renders its label instead of falling back
	//   to the placeholder.
	// - `name` renders a hidden input so native <form> posts (FormData) keep
	//   working exactly like a named <select>.
	// - `clearable` shows the chip's clear X → value becomes ''.
	import { t } from '$lib/stores/i18n.svelte';
	import Autocomplete, { type AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';

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
		ariaLabel
	}: Props = $props();

	// '' is not a usable Autocomplete item id (falsy = "no selection"), so
	// empty-string options get a sentinel id and map back to '' on commit.
	const NONE = '__none__';
	const hasNoneOption = $derived(options.some((o) => o.value === ''));

	const items = $derived<AutocompleteItem[]>(
		options.map((o) => ({
			id: o.value === '' ? NONE : o.value,
			label: o.label,
			sublabel: o.sublabel
		}))
	);
	// Local, case-insensitive substring match — no API call.
	const fetcher = $derived(
		(q: string): Promise<AutocompleteItem[]> =>
			Promise.resolve(
				q.trim()
					? items.filter((i) =>
							(i.label + ' ' + (i.sublabel ?? '') + ' ' + i.id)
								.toLowerCase()
								.includes(q.trim().toLowerCase())
						)
					: items
			)
	);

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
	emptyMessage={emptyMessage ?? t('common.no_results', 'No results found')}
	{fetcher}
	initialFetcher={() => fetcher('')}
	selectedId={selId}
	selectedLabel={selLabel}
	onSelect={(i) => commit(i.id)}
	onClear={clearable ? () => commit('') : undefined}
	{disabled}
	class={className}
	{inputClass}
/>
{#if name}
	<input type="hidden" {name} value={value ?? ''} />
{/if}
