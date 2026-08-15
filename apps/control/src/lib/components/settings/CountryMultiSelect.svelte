<script lang="ts">
	// CountryMultiSelect — a chip-based multi-select for ISO 3166-1 alpha-2
	// country codes. The setting value is stored as a CSV string (e.g.
	// "NG,GH,ZA,KE,EG"). Country names are sourced from the spaces API
	// (GET /api/v1/spaces?level=country) so the country list is never
	// hardcoded (Rule 16 — backend is source of truth).
	//
	// Used by geo_personalization.origin_markets and .destination_markets
	// (ADR-064 / Phase 38.03 — config-driven archetype market lists).
	//
	// Ref: japaconnect-engineering/adr/ADR-064-geo-aware-personalization.md
	// Ref: japaconnect-engineering/conventions/system-settings-catalog.md
	import { t } from '$lib/stores/i18n.svelte';
	import { api } from '$lib/api';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import { X } from 'lucide-svelte';

	interface CountrySpace {
		geo_code: string;
		name: string;
	}

	let {
		value,
		disabled = false,
		onchange
	}: {
		value: string;
		disabled?: boolean;
		onchange: (next: string) => void;
	} = $props();

	// Country list fetched once from the spaces API. Keyed by geo_code for
	// O(1) name lookups when rendering chips.
	let countries = $state<Map<string, string>>(new Map());
	let loaded = $state(false);

	$effect(() => {
		if (!loaded) loadCountries();
	});

	async function loadCountries() {
		// Fetch all country-level spaces. The API caps page_size at 100, so
		// we request the max and accept that ~195 countries may require
		// multiple pages. For now 100 covers the vast majority of configured
		// markets; if more are needed the backend can raise the cap.
		// The api() helper already unwraps the response .data field, so
		// res.data is the array of spaces directly.
		const res = await api<CountrySpace[]>(
			'/api/v1/spaces?level=country&page_size=100'
		);
		if (res.ok && res.data) {
			const map = new Map<string, string>();
			for (const s of res.data) {
				if (s.geo_code) map.set(s.geo_code.toUpperCase(), s.name);
			}
			countries = map;
		}
		loaded = true;
	}

	// Parse the CSV value into an array of ISO codes. Handles JSON-quoted
	// values (e.g. '"NG,GH,ZA"') that the backend may return.
	function parseCsv(v: string): string[] {
		if (!v) return [];
		let raw = v.trim();
		// Strip JSON string quoting if present.
		if (raw.startsWith('"') && raw.endsWith('"')) {
			raw = raw.slice(1, -1);
		}
		return raw
			.split(',')
			.map((s) => s.trim().toUpperCase())
			.filter(Boolean);
	}

	// selectedCodes is derived from the current value prop so external
	// resets (e.g. "Reset to default") are immediately reflected.
	let selectedCodes = $derived(parseCsv(value));

	function emit(next: string[]) {
		onchange(next.join(','));
	}

	function addCountry(code: string) {
		const upper = code.toUpperCase();
		if (!selectedCodes.includes(upper)) {
			emit([...selectedCodes, upper]);
		}
	}

	function removeCountry(code: string) {
		emit(selectedCodes.filter((c) => c !== code));
	}

	// Autocomplete fetchers — filter the loaded country list client-side.
	// Searches both the ISO code and the country name.
	function fetchCountries(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase().trim();
		const entries = [...countries.entries()];
		const filtered = query === ''
			? entries
			: entries.filter(
					([code, name]) =>
						code.toLowerCase().includes(query) ||
						name.toLowerCase().includes(query)
				);
		// Show "CODE — Name" in the dropdown for clarity.
		return Promise.resolve(
			filtered
				.filter(([code]) => !selectedCodes.includes(code))
				.slice(0, 50)
				.map(([code, name]) => ({
					id: code,
					label: `${code} — ${name}`
				}))
		);
	}

	function fetchCountriesInitial(): Promise<AutocompleteItem[]> {
		return fetchCountries('');
	}

	// countryLabel returns the display name for a code, falling back to the
	// raw code if the country list hasn't loaded yet or the code is unknown.
	function countryLabel(code: string): string {
		return countries.get(code) ?? code;
	}
</script>

<div class="w-64">
	{#if selectedCodes.length > 0}
		<div class="mb-1 flex flex-wrap gap-1">
			{#each selectedCodes as code}
				<span
					class="bg-accent text-accent-foreground inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px]"
					title={countryLabel(code)}
				>
					<span class="font-mono font-semibold">{code}</span>
					<span class="text-muted-foreground">{countryLabel(code)}</span>
					<button
						class="hover:text-destructive"
						disabled={disabled}
						onclick={() => removeCountry(code)}
						aria-label={t('common.remove', 'Remove') + ' ' + code}
					>
						<X size={11} />
					</button>
				</span>
			{/each}
		</div>
	{/if}
	{#if !loaded}
		<div class="flex items-center gap-2 rounded-lg border border-border-input bg-background px-3 py-1.5 text-sm text-muted-foreground">
			<span class="inline-block size-3.5 animate-spin rounded-full border-2 border-border-input border-t-accent-foreground"></span>
			{t('common.loading', 'Loading...')}
		</div>
	{:else}
		<Autocomplete
			placeholder={t('settings.country_add_placeholder', 'Add country...')}
			emptyMessage={t('settings.no_results', 'No results found')}
			fetcher={fetchCountries}
			initialFetcher={fetchCountriesInitial}
			selectedId={null}
			onSelect={(item) => addCountry(item.id)}
		/>
	{/if}
</div>
