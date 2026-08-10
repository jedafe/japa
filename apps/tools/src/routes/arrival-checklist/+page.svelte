<script lang="ts">
	import { untrack } from 'svelte';
	import { replaceState } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { buildSeoMeta } from '$lib/seo';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import LoadingSkeleton from '$lib/components/ui/LoadingSkeleton.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import DestinationSelector from '$lib/components/tools/DestinationSelector.svelte';
	import type {
		ArrivalChecklistSelectorEntry,
		ArrivalChecklistItem,
		GeoSpaceOption
	} from '$lib/types';

	/**
	 * Arrival Checklist tool — §92, ADR-088.
	 * Destination selector (built from the selector data) → optional visa
	 * category sub-selector → checklist items grouped by phase, sorted by
	 * sort_order. No auth needed for viewing (V1 — no progress tracking).
	 */
	let { data } = $props();

	let selectorEntries = $state<ArrivalChecklistSelectorEntry[]>(
		untrack(() => data.selectorEntries)
	);
	let destinations = $state<GeoSpaceOption[]>(untrack(() => data.destinations));
	let items = $state<ArrivalChecklistItem[]>(untrack(() => data.items));
	let selectedDestId = $state<string>(untrack(() => data.initialDestId ?? ''));
	let selectedVisaCategory = $state<string>('');
	let loadingItems = $state(false);

	const seo = $derived(
		buildSeoMeta({
			title: t('arrival_checklist.seo_title', 'Arrival Checklist — JapaConnect Tools'),
			description: t(
				'arrival_checklist.seo_description',
				'A phased checklist of tasks to complete after arriving in your destination country — registration, banking, and more.'
			),
			path: '/arrival-checklist',
			origin: page.url.origin
		})
	);

	// Visa categories available for the selected destination (from selector).
	// Only shown if there's more than one category for the destination.
	const visaCategories = $derived(
		selectedDestId
			? selectorEntries
					.filter((e) => e.destination_space_id === selectedDestId && e.visa_category)
					.map((e) => e.visa_category as string)
			: []
	);

	const showVisaCategorySelector = $derived(visaCategories.length > 1);

	// Group items by phase, sorted by sort_order within each phase.
	const phasedItems = $derived.by(() => {
		const phases = new Map<string, ArrivalChecklistItem[]>();
		for (const item of items) {
			const arr = phases.get(item.phase) ?? [];
			arr.push(item);
			phases.set(item.phase, arr);
		}
		// Sort items within each phase by sort_order
		const result: { phase: string; items: ArrivalChecklistItem[] }[] = [];
		for (const [phase, phaseItems] of phases) {
			result.push({
				phase,
				items: phaseItems.sort((a, b) => a.sort_order - b.sort_order)
			});
		}
		return result;
	});

	async function loadItems() {
		if (!selectedDestId) {
			items = [];
			return;
		}
		// Find the selector entry for this dest (+ optional visa category)
		// and use its guide_id to fetch items via the correct endpoint.
		const entry = selectorEntries.find(
			(e) =>
				e.destination_space_id === selectedDestId &&
				(!selectedVisaCategory || e.visa_category === selectedVisaCategory)
		);
		if (!entry?.guide_id) {
			items = [];
			return;
		}
		loadingItems = true;
		const res = await api.get<ArrivalChecklistItem[]>(`/arrival-checklist/${entry.guide_id}`);
		loadingItems = false;
		if (!res.error) items = res.data ?? [];
		else items = [];
	}

	function onDestChange(id: string) {
		selectedDestId = id;
		selectedVisaCategory = '';
		const url = new URL(page.url);
		if (id) url.searchParams.set('dest', id);
		else url.searchParams.delete('dest');
		replaceState(url, {});
		loadItems();
	}

	function onVisaCategoryChange(e: Event) {
		selectedVisaCategory = (e.currentTarget as HTMLSelectElement).value;
		loadItems();
	}
</script>

<svelte:head>
	<title>{seo.title}</title>
	<meta name="description" content={seo.description} />
	<link rel="canonical" href={seo.canonical} />
	<meta property="og:title" content={seo.title} />
	<meta property="og:description" content={seo.description} />
	<meta property="og:url" content={seo.canonical} />
	<meta property="og:type" content="website" />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={seo.title} />
	<meta name="twitter:description" content={seo.description} />
</svelte:head>

<div class="mx-auto max-w-4xl px-4 py-8">
	{#if !data.enabled}
		<ToolDisabled tool={data.tool} />
	{:else}
		<div class="mb-8">
			<h1 class="text-2xl sm:text-3xl font-bold text-text">
				{t('arrival_checklist.title', 'Arrival Checklist')}
			</h1>
			<p class="mt-2 text-text-subtle">
				{t('arrival_checklist.subtitle', 'Everything you need to do after you land — phased by time period.')}
			</p>
		</div>

		{#if destinations.length === 0}
			<EmptyState
				icon="plane-landing"
				title={t('arrival_checklist.no_destinations', 'No checklists available')}
				description={t('arrival_checklist.no_destinations_desc', 'Arrival checklist guides haven’t been published yet. Please check back later.')}
			/>
		{:else}
			<!-- Destination selector (built from selector data — geo-aware pattern) -->
			<div class="mb-4">
				<DestinationSelector {destinations} bind:selectedId={selectedDestId} onchange={onDestChange} />
			</div>

			{#if showVisaCategorySelector}
				<!-- Visa category sub-selector (only when multiple categories exist) -->
				<div class="mb-6 flex items-center gap-3">
					<label for="visa-cat" class="text-sm font-medium text-text whitespace-nowrap">
						{t('arrival_checklist.visa_category', 'Visa category:')}
					</label>
					<select
						id="visa-cat"
						value={selectedVisaCategory}
						onchange={onVisaCategoryChange}
						class="flex-1 px-4 py-2 bg-surface border border-border rounded-lg text-text focus:border-primary focus:outline-none"
					>
						<option value="">{t('arrival_checklist.all_categories', 'All categories')}</option>
						{#each visaCategories as cat (cat)}
							<option value={cat}>{cat}</option>
						{/each}
					</select>
				</div>
			{/if}

			{#if !selectedDestId}
				<EmptyState
					icon="map-pin"
					title={t('arrival_checklist.select_destination', 'Select a destination country')}
					description={t('arrival_checklist.select_destination_desc', 'Choose a country above to see your arrival checklist.')}
				/>
			{:else if loadingItems}
				<div class="space-y-4">
					<LoadingSkeleton height="40px" />
					<LoadingSkeleton height="100px" />
					<LoadingSkeleton height="100px" />
				</div>
			{:else if items.length === 0}
				<EmptyState
					icon="clipboard-list"
					title={t('arrival_checklist.no_items', 'No checklist items')}
					description={t('arrival_checklist.no_items_desc', 'No arrival checklist items have been published for this destination yet.')}
				/>
			{:else}
				<!-- Phased checklist -->
				<div class="space-y-8">
					{#each phasedItems as group (group.phase)}
						<div>
							<div class="flex items-center gap-2 mb-3">
								<div
									class="flex items-center justify-center w-8 h-8 rounded-lg bg-[var(--tool-accent)]/10 text-[var(--tool-accent)]"
								>
									<Icon name="calendar-days" size={18} />
								</div>
								<h2 class="text-lg font-semibold text-text">{group.phase}</h2>
								<span class="text-xs text-muted">({group.items.length})</span>
							</div>
							<div class="space-y-2">
								{#each group.items as item (item.id)}
									<div
										class="p-4 bg-surface border border-border rounded-lg {item.is_flagged ? 'border-yellow-300 bg-yellow-50/30' : ''}"
									>
										<div class="flex items-start gap-3">
											{#if item.is_flagged}
												<Icon name="flag" size={16} class="text-yellow-500 flex-shrink-0 mt-0.5" />
											{:else}
												<div class="w-4 flex-shrink-0 mt-0.5"></div>
											{/if}
											<div class="min-w-0 flex-1">
												<h3 class="text-sm font-medium text-text">{item.title}</h3>
												{#if item.description}
													<p class="mt-1 text-sm text-text-subtle">{item.description}</p>
												{/if}
												{#if item.official_link}
													<a
														href={item.official_link}
														target="_blank"
														rel="noopener noreferrer"
														class="mt-2 inline-flex items-center gap-1 text-xs text-[var(--tool-accent)] hover:underline"
													>
														<Icon name="external-link" size={12} />
														{t('arrival_checklist.official_link', 'Official guidance')}
													</a>
												{/if}
											</div>
										</div>
									</div>
								{/each}
							</div>
						</div>
					{/each}
				</div>
			{/if}
		{/if}
	{/if}
</div>
