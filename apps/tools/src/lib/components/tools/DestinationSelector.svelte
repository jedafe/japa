<script lang="ts">
	import { t } from '$lib/i18n';
	import type { GeoSpaceOption } from '$lib/types';

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

	function handleChange(e: Event) {
		const value = (e.currentTarget as HTMLSelectElement).value;
		selectedId = value;
		onchange?.(value);
	}
</script>

<div class="flex items-center gap-3">
	<label for="dest-selector" class="text-sm font-medium text-text whitespace-nowrap">
		{t('geo.destination', 'Destination:')}
	</label>
	<select
		id="dest-selector"
		value={selectedId}
		onchange={handleChange}
		class="flex-1 px-4 py-2 bg-surface border border-border rounded-lg text-text focus:border-primary focus:outline-none"
	>
		<option value="">{t('geo.select_destination', 'Select a country…')}</option>
		{#each destinations as dest (dest.id)}
			<option value={dest.id}>{dest.name}</option>
		{/each}
	</select>
</div>
