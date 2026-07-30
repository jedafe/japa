<script lang="ts">
	import { t } from '$lib/i18n';
	import Autocomplete from './Autocomplete.svelte';
	import type { AutocompleteItem } from './Autocomplete.svelte';
	import type { MeSpace } from '$lib/types';
	import { searchSpaces } from '$lib/api/create';

	interface Props {
		spaces: MeSpace[];
		selectedId: string | null;
		onSelect: (id: string) => void;
		onfocus?: () => void;
	}

	let { spaces, selectedId, onSelect, onfocus }: Props = $props();

	// The selected space object — used to display the label without a re-fetch.
	const selectedSpace = $derived(spaces.find((s) => s.id === selectedId) ?? null);

	// Debounced fetcher — calls GET /me/spaces?q=… via the API client.
	async function fetcher(q: string): Promise<AutocompleteItem[]> {
		const res = await searchSpaces(q);
		return (res.data ?? []).map((s) => ({
			id: s.id,
			label: s.name,
			sublabel: s.is_primary ? '★' : undefined
		}));
	}

	// Initial suggestions — show up to 5 spaces (primary first) when the field is focused.
	async function initialFetcher(): Promise<AutocompleteItem[]> {
		return spaces
			.slice()
			.sort((a, b) => (a.is_primary ? -1 : b.is_primary ? 1 : 0))
			.slice(0, 5)
			.map((s) => ({
				id: s.id,
				label: s.name,
				sublabel: s.is_primary ? '★' : undefined
			}));
	}

	function handleSelect(item: AutocompleteItem) {
		onSelect(item.id);
	}

	function handleClear() {
		onSelect('');
	}
</script>

<Autocomplete
	placeholder={t('create.placeholder.space_search')}
	emptyMessage={t('create.placeholder.space_no_results')}
	{fetcher}
	{initialFetcher}
	{selectedId}
	selectedLabel={selectedSpace?.name ?? null}
	selectedBadge={selectedSpace?.is_primary ? '★' : null}
	onSelect={handleSelect}
	onClear={handleClear}
	{onfocus}
/>
