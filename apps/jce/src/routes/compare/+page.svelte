<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import {
		listComparisonTypes,
		getComparisonType,
		searchEntities,
		createQuickComparison,
		createGuestSelection,
		createQuickComparisonFromToken
	} from '$lib/api/jce';
	import type { ComparisonType, ResolvedItem, JCEComparisonSummary } from '$lib/types/jce';

	let { types, preselectedType, preselectedItems, compareToken, user, mainOrigin } = $props<{
		types: ComparisonType[];
		preselectedType: string;
		preselectedItems: ResolvedItem[];
		compareToken: string;
		user: { id: string } | null;
		mainOrigin: string;
	}>();

	let selectedType = $state<ComparisonType | null>(null);
	let query = $state('');
	let results = $state<ResolvedItem[]>([]);
	let selectedItems = $state<ResolvedItem[]>([]);
	let loading = $state(false);
	let error = $state('');
	let creating = $state(false);

	$effect.pre(() => {
		if (selectedItems.length === 0 && preselectedItems && preselectedItems.length > 0) {
			selectedItems = [...preselectedItems];
		}
	});

	const maxItems = $derived((selectedType?.config?.max_item_count as number) ?? 4);
	const itemRefTypes = $derived((selectedType?.config?.item_ref_types as string[]) ?? ['space']);

	onMount(() => {
		if (compareToken && user) {
			restoreFromToken();
		} else if (preselectedType) {
			loadType(preselectedType);
		} else if (types.length === 1) {
			selectType(types[0]);
		}
	});

	async function loadTypeThenRestore() {
		if (!selectedType && preselectedType) {
			const t = await getComparisonType(preselectedType);
			if (t) selectedType = t;
		}
	}

	async function loadType(slug: string) {
		const t = await getComparisonType(slug);
		if (t) selectType(t);
	}

	async function selectType(t: ComparisonType) {
		selectedType = t;
		// Add URL query param without full reload.
		const url = new URL($page.url);
		url.searchParams.set('type', t.slug);
		goto(url, { replaceState: true, keepFocus: true });
		if (query) await doSearch();
	}

	async function doSearch() {
		if (!selectedType || !query.trim()) {
			results = [];
			return;
		}
		loading = true;
		error = '';
		try {
			const entityType = itemRefTypes[0] ?? 'space';
			results = await searchEntities(entityType, query, {});
		} catch (e) {
			error = e instanceof Error ? e.message : 'Search failed';
		} finally {
			loading = false;
		}
	}

	function addItem(item: ResolvedItem) {
		if (selectedItems.some((i) => i.id === item.id && i.type === item.type)) return;
		if (selectedItems.length >= maxItems) return;
		selectedItems = [...selectedItems, item];
	}

	function removeItem(idx: number) {
		selectedItems = selectedItems.filter((_, i) => i !== idx);
	}

	async function create() {
		if (!selectedType || selectedItems.length < 2) return;
		const type = selectedType;
		const refType = itemRefTypes[0] ?? 'space';
		creating = true;
		error = '';
		try {
			const title = `Compare ${selectedItems.map((i) => i.title).join(' vs ')}`;
			const input = {
				title,
				type_id: type.id,
				depth: 'quick' as const,
				status: 'draft' as const,
				visibility: 'private' as const,
				items: selectedItems.map((i) => ({
					title: i.title,
					description: i.description ?? '',
					type_id: type.id,
					item_ref: { type: refType, id: i.id, title: i.title, slug: i.metadata?.slug as string }
				})),
				dimensions: []
			};
			const cmp = await createQuickComparison(input);
			if (cmp?.slug) {
				goto(`/compare/${cmp.slug}`);
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not create comparison';
		} finally {
			creating = false;
		}
	}

	async function saveGuestSelectionAndLogin() {
		if (!selectedType || selectedItems.length < 2) return;
		const type = selectedType;
		const refType = itemRefTypes[0] ?? 'space';
		const itemRefs = selectedItems.map((i) => ({
			type: refType,
			id: i.id,
			title: i.title,
			slug: i.metadata?.slug
		}));
		const token = await createGuestSelection({ type_slug: type.slug, item_refs: itemRefs });
		if (token) {
			const returnUrl = encodeURIComponent(`/compare?type=${encodeURIComponent(type.slug)}&compare_token=${token}`);
			window.location.href = `${mainOrigin}/login?redirect=${returnUrl}`;
		}
	}

	async function restoreFromToken() {
		if (!user || !compareToken) return;
		await loadTypeThenRestore();
		if (!selectedType) return;
		const type = selectedType;
		creating = true;
		try {
			const cmp = await createQuickComparisonFromToken(compareToken, type.id);
			if (cmp?.slug) goto(`/compare/${cmp.slug}`);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not restore selection';
		} finally {
			creating = false;
		}
	}

	$effect(() => {
		if (query.length > 1) {
			const t = setTimeout(doSearch, 250);
			return () => clearTimeout(t);
		}
		results = [];
	});
</script>

<div class="max-w-4xl mx-auto p-6 space-y-8">
	<h1 class="text-3xl font-bold">Compare</h1>

	{#if error}
		<div class="rounded-lg bg-red-500/10 text-red-500 p-4" role="alert">{error}</div>
	{/if}

	{#if compareToken && !user}
		<p class="text-sm text-slate-400">
			Your selections are saved. <a href="{mainOrigin}/login?redirect={encodeURIComponent($page.url.pathname + $page.url.search)}" class="underline">Log in</a> to create the comparison.
		</p>
	{/if}

	<section class="space-y-4">
		<h2 class="text-lg font-semibold">1. Choose a comparison type</h2>
		<div class="flex flex-wrap gap-3">
			{#each types as t (t.id)}
				<button
					type="button"
					class="px-4 py-2 rounded-lg border border-slate-700 text-sm hover:bg-slate-800 focus:ring-2 focus:ring-blue-500"
					class:bg-slate-800={selectedType?.id === t.id}
					onclick={() => selectType(t)}
				>
					{t.name}
				</button>
			{/each}
		</div>
	</section>

	{#if selectedType}
		<section class="space-y-4">
			<h2 class="text-lg font-semibold">2. Pick {selectedType.item_label ?? 'items'} (up to {maxItems})</h2>

			<div class="relative">
				<input
					type="text"
					bind:value={query}
					placeholder="Search {selectedType.name}..."
					class="w-full rounded-lg bg-slate-900 border border-slate-700 px-4 py-2"
				/>
				{#if loading}
					<span class="absolute right-4 top-2 text-slate-400 text-sm">Searching...</span>
				{/if}
			</div>

			{#if results.length > 0}
				<ul class="divide-y divide-slate-800 border border-slate-700 rounded-lg">
					{#each results as item (item.id)}
						<li class="p-3 flex items-center justify-between hover:bg-slate-800/50">
							<div>
								<p class="font-medium">{item.title}</p>
								{#if item.description}
									<p class="text-sm text-slate-400">{item.description}</p>
								{/if}
							</div>
							<button
								type="button"
								class="text-sm px-3 py-1 rounded bg-blue-600 hover:bg-blue-500 disabled:opacity-50"
								disabled={selectedItems.length >= maxItems}
								onclick={() => addItem(item)}
							>
								Add
							</button>
						</li>
					{/each}
				</ul>
			{/if}

			{#if selectedItems.length > 0}
				<div class="space-y-2">
					<h3 class="text-sm font-semibold text-slate-300">Selected</h3>
					<ul class="flex flex-wrap gap-2">
						{#each selectedItems as item, idx (item.id)}
							<li class="px-3 py-1 rounded-full bg-slate-800 border border-slate-700 flex items-center gap-2">
								<span class="text-sm">{item.title}</span>
								<button
									type="button"
									class="text-slate-400 hover:text-white"
									onclick={() => removeItem(idx)}
								>
									&times;
								</button>
							</li>
						{/each}
					</ul>
				</div>
			{/if}
		</section>

		{#if selectedItems.length >= 2}
			<section class="space-y-4">
				<h2 class="text-lg font-semibold">3. Create quick comparison</h2>
				<p class="text-sm text-slate-400">
					Quick comparisons are private drafts. You can promote them to a detailed comparison later.
				</p>
				{#if user}
					<button
						type="button"
						class="px-6 py-2 rounded-lg bg-green-600 hover:bg-green-500 disabled:opacity-50"
						disabled={creating}
						onclick={create}
					>
						{creating ? 'Creating...' : 'Compare now'}
					</button>
				{:else}
					<button
						type="button"
						class="px-6 py-2 rounded-lg bg-blue-600 hover:bg-blue-500"
						onclick={saveGuestSelectionAndLogin}
					>
						Log in to compare
					</button>
				{/if}
			</section>
		{/if}
	{/if}
</div>
