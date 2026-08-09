<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import {
		PlaneLanding,
		MapPin,
		CalendarDays,
		Flag,
		ExternalLink,
		ClipboardList,
		Lock
	} from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton, SelectField } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import type {
		ArrivalChecklistSelectorEntry,
		ArrivalChecklistItem,
		GeoSpaceOption
	} from '$lib/types';
	import type { PageData } from './$types';

	/**
	 * Arrival Checklist tool — §92, ADR-088.
	 * Destination selector (built from the selector data) → optional visa
	 * category sub-selector → checklist items grouped by phase, sorted by
	 * sort_order. No auth needed for viewing (V1 — no progress tracking).
	 */
	let { data }: { data: PageData } = $props();

	const title = $derived(t('arrival_checklist.title', 'Arrival Checklist'));
	const description = $derived(
		t(
			'arrival_checklist.subtitle',
			'Everything you need to do after you land — phased by time period.'
		)
	);
	const disabled = $derived(data.disabled);
	const authed = $derived(data.authed);

	let selectorEntries = $state<ArrivalChecklistSelectorEntry[]>(
		untrack(() => data.selector)
	);
	let destinations = $state<GeoSpaceOption[]>(untrack(() => data.destinations));
	let items = $state<ArrivalChecklistItem[]>(untrack(() => data.items));
	let selectedDestId = $state<string>(untrack(() => data.initialDestId ?? ''));
	let selectedVisaCategory = $state<string>('');
	let loadingItems = $state(false);

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
		history.replaceState({}, '', url);
		loadItems();
	}

	function onVisaCategoryChange(v: string) {
		selectedVisaCategory = v;
		loadItems();
	}
</script>

<svelte:head>
	<title>{title} — JapaConnect Tools</title>
	<meta
		name="description"
		content={t(
			'arrival_checklist.seo_description',
			'A phased checklist of tasks to complete after arriving in your destination country — registration, banking, and more.'
		)}
	/>
	<link rel="canonical" href={page.url.href} />
	<meta property="og:title" content="{title} — JapaConnect Tools" />
	<meta
		property="og:description"
		content={t(
			'arrival_checklist.seo_description',
			'A phased checklist of tasks to complete after arriving in your destination country — registration, banking, and more.'
		)}
	/>
	<meta property="og:url" content={page.url.href} />
	<meta property="og:type" content="website" />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content="{title} — JapaConnect Tools" />
	<meta
		name="twitter:description"
		content={t(
			'arrival_checklist.seo_description',
			'A phased checklist of tasks to complete after arriving in your destination country — registration, banking, and more.'
		)}
	/>
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<PlaneLanding size={20} /> {title}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">{description}</p>
	</div>

	{#if disabled}
		<div class="p-4">
			<EmptyState
				icon={Lock}
				title={t('tools.disabled_title', 'This tool is currently unavailable')}
				body={t('tools.disabled_body', 'Check back soon or contact support.')}
			/>
		</div>
	{:else if destinations.length === 0}
		<div class="p-4">
			<EmptyState
				icon={PlaneLanding}
				title={t('arrival_checklist.no_destinations', 'No checklists available')}
				body={t(
					'arrival_checklist.no_destinations_desc',
					'Arrival checklist guides haven’t been published yet. Please check back later.'
				)}
			/>
		</div>
	{:else}
		<!-- Destination selector (built from selector data — geo-aware pattern) -->
		<div class="border-b border-[var(--color-border)] px-4 py-3">
			<label
				for="ac-dest"
				class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]"
			>
				{t('geo.destination', 'Destination:')}
			</label>
			<div class="mt-2">
				<SelectField
					id="ac-dest"
					value={selectedDestId}
					options={destinations.map((d) => ({ value: d.id, label: d.name }))}
					placeholder={t('geo.select_destination', 'Select a country…')}
					onValueChange={(v) => onDestChange(v)}
				/>
			</div>
		</div>

		{#if showVisaCategorySelector}
			<!-- Visa category sub-selector (only when multiple categories exist) -->
			<div class="border-b border-[var(--color-border)] px-4 py-3">
				<label
					for="ac-visa"
					class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]"
				>
					{t('arrival_checklist.visa_category', 'Visa category:')}
				</label>
				<div class="mt-2">
					<SelectField
						id="ac-visa"
						value={selectedVisaCategory}
						options={visaCategories.map((cat) => ({ value: cat, label: cat }))}
						placeholder={t('arrival_checklist.all_categories', 'All categories')}
						onValueChange={(v) => onVisaCategoryChange(v)}
					/>
				</div>
			</div>
		{/if}

		{#if !selectedDestId}
			<div class="p-4">
				<EmptyState
					icon={MapPin}
					title={t('arrival_checklist.select_destination', 'Select a destination country')}
					body={t(
						'arrival_checklist.select_destination_desc',
						'Choose a country above to see your arrival checklist.'
					)}
				/>
			</div>
		{:else}
			<div class="p-4">
				{#if !authed}
					<div
						class="mb-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-xs text-[var(--color-muted)]"
					>
						{t('arrival_checklist.anon_hint', 'Sign in to save your progress.')}
					</div>
				{/if}

				{#if loadingItems}
					<LoadingSkeleton variant="lines" count={5} />
				{:else if items.length === 0}
					<EmptyState
						icon={ClipboardList}
						title={t('arrival_checklist.no_items', 'No checklist items')}
						body={t(
							'arrival_checklist.no_items_desc',
							'No arrival checklist items have been published for this destination yet.'
						)}
					/>
				{:else}
					<!-- Phased checklist -->
					<div class="space-y-8">
						{#each phasedItems as group (group.phase)}
							<div>
								<div class="flex items-center gap-2 mb-3">
									<div
										class="flex items-center justify-center w-8 h-8 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)]"
									>
										<CalendarDays size={18} />
									</div>
									<h2 class="text-lg font-semibold text-[var(--color-text)]">{group.phase}</h2>
									<span class="text-xs text-[var(--color-muted)]">({group.items.length})</span>
								</div>
								<div class="space-y-2">
									{#each group.items as item (item.id)}
										<div
											class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 {item.is_flagged
											? 'border-yellow-300 bg-yellow-50/30'
											: ''}"
										>
											<div class="flex items-start gap-3">
												{#if item.is_flagged}
													<Flag
														size={16}
														class="text-yellow-500 flex-shrink-0 mt-0.5"
													/>
												{:else}
													<div class="w-4 flex-shrink-0 mt-0.5"></div>
												{/if}
												<div class="min-w-0 flex-1">
													<h3 class="text-sm font-medium text-[var(--color-text)]">{item.title}</h3>
													{#if item.description}
														<p class="mt-1 text-sm text-[var(--color-muted)]">{item.description}</p>
													{/if}
													{#if item.official_link}
														<a
															href={item.official_link}
															target="_blank"
															rel="noopener noreferrer"
															class="mt-2 inline-flex items-center gap-1 text-xs text-[var(--color-primary)] hover:underline"
														>
															<ExternalLink size={12} />
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
			</div>
		{/if}
	{/if}
</div>
