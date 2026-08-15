<script lang="ts">
	import { t } from '$lib/i18n';
	import type { GeoSpaceOption } from '$lib/types';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';

	/**
	 * DestinationSelector — the first UI input on every V2 geo-aware tool page
	 * (§§90–97, §§99–101). The destination country is the primary data axis:
	 * all downstream API calls pass the selected space ID as `?dest=`.
	 *
	 * Ref: .jeos/skills/geo-aware-tool-pattern.md, ADR-088.
	 *      Rule 16 — the country list comes from the backend API, never hardcoded.
	 */
	let {
		destinations,
		selectedId = $bindable(),
		onchange
	}: {
		destinations: GeoSpaceOption[];
		selectedId: string;
		onchange?: (id: string) => void;
	} = $props();

	function fetchDestinations(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		const filtered = query === ''
			? destinations
			: destinations.filter((d) => d.name.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((d) => ({ id: d.id, label: d.name })));
	}

	function fetchDestinationsInitial(): Promise<AutocompleteItem[]> {
		return fetchDestinations('');
	}

	function getDestinationLabel(id: string): string {
		return destinations.find((d) => d.id === id)?.name ?? id;
	}
</script>

<div class="flex items-center gap-3">
	<label for="dest-selector" class="text-sm font-medium text-text whitespace-nowrap">
		{t('geo.destination', 'Destination:')}
	</label>
	<Autocomplete
		id="dest-selector"
		placeholder={t('geo.select_destination', 'Select a country…')}
		emptyMessage={t('geo.no_destinations', 'No destinations found')}
		fetcher={fetchDestinations}
		initialFetcher={fetchDestinationsInitial}
		selectedId={selectedId || null}
		selectedLabel={selectedId ? getDestinationLabel(selectedId) : null}
		onSelect={(item) => {
			selectedId = item.id;
			onchange?.(item.id);
		}}
		onClear={() => {
			selectedId = '';
			onchange?.('');
		}}
	/>
</div>
