<script lang="ts">
	// WebhookEventMultiSelect — a chip-based multi-select for webhook event
	// types. The event catalog is sourced from the backend
	// (GET /api/v1/control/webhook-events) so it is never hardcoded (Rule 16).
	//
	// Uses the shared Autocomplete.svelte component for a live debounced
	// search over the catalog (client-side filter, debounced by the
	// Autocomplete component). Mirrors the CountryMultiSelect pattern.
	import { t } from '$lib/stores/i18n.svelte';
	import { api } from '$lib/api';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import { X } from 'lucide-svelte';

	interface WebhookEventDef {
		key: string;
		label: string;
		description: string;
	}

	let {
		value,
		onchange
	}: {
		value: string[];
		onchange: (next: string[]) => void;
	} = $props();

	// Event catalog fetched once from the backend. Keyed by event key for
	// O(1) label lookups when rendering chips.
	let events = $state<Map<string, WebhookEventDef>>(new Map());
	let loaded = $state(false);

	$effect(() => {
		if (!loaded) loadEvents();
	});

	async function loadEvents() {
		const res = await api<WebhookEventDef[]>('/control/webhook-events');
		if (res.ok && Array.isArray(res.data)) {
			const map = new Map<string, WebhookEventDef>();
			for (const e of res.data) {
				if (e.key) map.set(e.key, e);
			}
			events = map;
		}
		loaded = true;
	}

	// selectedKeys is derived from the current value prop so external
	// resets (e.g. modal close) are immediately reflected.
	let selectedKeys = $derived(
		Array.isArray(value) ? value.filter((k) => typeof k === 'string') : []
	);

	function emit(next: string[]) {
		onchange(next);
	}

	function addEvent(key: string) {
		if (!selectedKeys.includes(key)) {
			emit([...selectedKeys, key]);
		}
	}

	function removeEvent(key: string) {
		emit(selectedKeys.filter((k) => k !== key));
	}

	// Autocomplete fetcher — filter the loaded event catalog client-side.
	// Searches both the event key and the label.
	function fetchEvents(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase().trim();
		const entries = [...events.values()];
		const filtered = query === ''
			? entries
			: entries.filter(
					(e) =>
						e.key.toLowerCase().includes(query) ||
						e.label.toLowerCase().includes(query)
				);
		return Promise.resolve(
			filtered
				.filter((e) => !selectedKeys.includes(e.key))
				.slice(0, 50)
				.map((e) => ({
					id: e.key,
					label: e.label,
					sublabel: e.key
				}))
		);
	}

	function fetchEventsInitial(): Promise<AutocompleteItem[]> {
		return fetchEvents('');
	}

	function eventLabel(key: string): string {
		return events.get(key)?.label ?? key;
	}
</script>

<div class="w-full">
	{#if selectedKeys.length > 0}
		<div class="mb-2 flex flex-wrap gap-1.5">
			{#each selectedKeys as key (key)}
				<span
					class="bg-muted text-muted-foreground inline-flex items-center gap-1 rounded-5px px-2 py-0.5 font-mono text-xs"
					title={events.get(key)?.description ?? key}
				>
					{eventLabel(key)}
					<button
						type="button"
						class="text-muted-foreground hover:text-foreground"
						onclick={() => removeEvent(key)}
						aria-label={t('common.remove', 'Remove') + ' ' + key}
					>
						<X size={11} />
					</button>
				</span>
			{/each}
		</div>
	{/if}
	{#if !loaded}
		<div class="border-border-input bg-background text-muted-foreground flex items-center gap-2 rounded-9px border px-3 py-2 text-sm">
			<span class="border-border-input border-t-accent-foreground inline-block size-3.5 animate-spin rounded-full border-2"></span>
			{t('common.loading', 'Loading...')}
		</div>
	{:else}
		<Autocomplete
			placeholder={t('webhooks.search_events', 'Search events...')}
			emptyMessage={t('webhooks.no_events_found', 'No events found.')}
			fetcher={fetchEvents}
			initialFetcher={fetchEventsInitial}
			selectedId={null}
			onSelect={(item) => addEvent(item.id)}
		/>
	{/if}
</div>
