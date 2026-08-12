<script lang="ts">
	// WidgetPicker — wraps the admin Autocomplete with a live-debounced search
	// fetcher that queries /admin/widgets. Used by the Page Builder's widget
	// block content editor so the user can search for a widget by name instead
	// of pasting a UUID.
	import Autocomplete, { type AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import { apiRaw } from '$lib/api';

	interface WidgetListItem {
		id: string;
		name: string;
		widget_type: string;
		is_active: boolean;
	}

	interface Props {
		/** Currently selected widget UUID, or null/empty. */
		selectedId: string | null;
		/** Label for the currently selected widget (its name). */
		selectedLabel?: string | null;
		/** Callback when a widget is selected. */
		onSelect: (item: AutocompleteItem) => void;
		/** Callback when the selection is cleared. */
		onClear?: () => void;
	}

	let { selectedId, selectedLabel, onSelect, onClear }: Props = $props();

	// Fetch widgets matching the query string via the admin API.
	// Uses apiRaw so we get the raw { data, total } envelope.
	async function fetcher(q: string): Promise<AutocompleteItem[]> {
		const params = new URLSearchParams();
		params.set('page_size', '20');
		params.set('page_offset', '0');
		if (q) params.set('q', q);
		const result = await apiRaw<{ data?: WidgetListItem[]; total?: number }>(
			`/admin/widgets?${params.toString()}`
		);
		if (!result.ok || !result.data?.data) return [];
		return result.data.data.map((w) => ({
			id: w.id,
			label: w.name,
			sublabel: `${w.widget_type}${w.is_active ? '' : ' · inactive'}`,
		}));
	}

	// Initial suggestions when the field is focused with no query.
	async function initialFetcher(): Promise<AutocompleteItem[]> {
		return fetcher('');
	}
</script>

<Autocomplete
	placeholder="Search widgets by name…"
	emptyMessage="No widgets found"
	{fetcher}
	{initialFetcher}
	{selectedId}
	{selectedLabel}
	{onSelect}
	{onClear}
/>
