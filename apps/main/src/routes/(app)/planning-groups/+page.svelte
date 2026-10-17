<script lang="ts">
	import { untrack } from 'svelte';
	// Ref: japaconnect-engineering/sections/82-japa-planning-groups.md
	import { t } from '$lib/i18n';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { Users, Plus, Plane, Calendar, Lock, Unlock } from 'lucide-svelte';
	import { EmptyState, ListPageBar, UrlTabs, type UrlTab, FilterToggle, PageAction } from '$lib/components/ui';
	import { staticAutocompleteFetcher, autocompleteItems } from '$lib/autocomplete';
	import type { PageData } from './$types';
import { safeJsonLdScript } from '$lib/utils/safeHtml';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('planning_groups.title', 'Planning Groups — {site}'));
	const description = $derived(
		t(
			'planning_groups.description',
			'Join or create groups to plan your japa journey with others heading to the same destination.'
		)
	);

	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'CollectionPage',
			name: 'Planning Groups',
			description,
			url: page.url.href
		})
	);

	let destFilter = $state(untrack(() => data.filters.dest));
	let visaFilter = $state(untrack(() => data.filters.visa));

	const filterHref = $derived(() => {
		const params = new URLSearchParams();
		if (destFilter.trim()) params.set('dest', destFilter.trim());
		if (visaFilter.trim()) params.set('visa', visaFilter.trim());
		const qs = params.toString();
		return qs ? `/planning-groups?${qs}` : '/planning-groups';
	});

	function applyFilters(e?: SubmitEvent) {
		e?.preventDefault();
		goto(filterHref());
	}

	// Join-policy axis is a client-side filter over the loaded list (the
	// server does not take an is_open_join param).
	let view = $state('all');
	const tabs = $derived<UrlTab[]>([
		{ value: 'all', label: t('common.all', 'All') },
		{ value: 'open', label: t('planning_groups.open_join', 'Open to join') },
		{ value: 'closed', label: t('planning_groups.closed_join', 'Invite only') }
	]);
	const visibleGroups = $derived(
		view === 'all'
			? data.groups
			: data.groups.filter((g) => (view === 'open' ? g.is_open_join : !g.is_open_join))
	);

	// visa is a free-text server filter — suggest categories observed in the
	// loaded groups, and allow arbitrary input via allowFreeText.
	const visaFetcher = $derived(
		staticAutocompleteFetcher(autocompleteItems(data.groups.map((g) => g.visa_category)))
	);

	const hasFilters = $derived(!!data.filters.dest || !!data.filters.visa);
	let showFilters = $state(false);
	$effect(() => {
		if (hasFilters) showFilters = true;
	});

	function fetchDestinationCountries(q: string): Promise<AutocompleteItem[]> {
		const allLabel = t('planning_groups.filter_destination_all', 'All countries');
		const query = q.toLowerCase().trim();
		const allItem: AutocompleteItem = { id: '', label: allLabel };
		if (query === '') {
			return Promise.resolve([allItem, ...(data.countries ?? []).map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }))]);
		}
		const matchesAll = allLabel.toLowerCase().includes(query);
		const filtered = (data.countries ?? []).filter(
			(c) => c.name.toLowerCase().includes(query) || c.geo_code.toLowerCase().includes(query)
		);
		const items = filtered.map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }));
		return Promise.resolve(matchesAll ? [allItem, ...items] : items);
	}
	function fetchDestinationCountriesInitial(): Promise<AutocompleteItem[]> {
		return fetchDestinationCountries('');
	}
	const selectedDestinationLabel = $derived(
		destFilter ? (data.countries ?? []).find((c) => c.id === destFilter)?.name ?? null : null
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
	{@html safeJsonLdScript(jsonLd)}
</svelte:head>

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 48rem)/2))">
	<h1 class="sr-only">{t('planning_groups.heading', 'Planning Groups')}</h1>
	<ListPageBar>
		<UrlTabs {tabs} bind:value={view} class="!border-b-0 !bg-transparent" />
		{#snippet actions()}
			<FilterToggle bind:show={showFilters} active={hasFilters} controls="planning-groups-filter-panel" />
			<PageAction
				href={data.authed ? '/planning-groups/new' : '/login?redirect=/planning-groups'}
				icon={Plus}
				label={t('planning_groups.create', 'Create Group')}
			/>
		{/snippet}
	</ListPageBar>

	<div class="mx-auto w-full max-w-3xl p-4">
		{#if showFilters}
			<!-- Filter panel -->
			<div id="planning-groups-filter-panel" class="mb-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
		<form onsubmit={applyFilters} class="flex flex-wrap items-center gap-3">
			<div class="w-52">
				<Autocomplete
					placeholder={t('planning_groups.filter_destination', 'Destination')}
					ariaLabel={t('planning_groups.filter_destination', 'Destination')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={fetchDestinationCountries}
					initialFetcher={fetchDestinationCountriesInitial}
					selectedId={destFilter || null}
					selectedLabel={selectedDestinationLabel}
					onSelect={(item) => { destFilter = item.id; applyFilters(); }}
					onClear={() => { destFilter = ''; applyFilters(); }}
				/>
			</div>
			<div class="w-52">
				<Autocomplete
					placeholder={t('planning_groups.filter_visa', 'Visa category')}
					ariaLabel={t('planning_groups.filter_visa', 'Visa category')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={visaFetcher}
					initialFetcher={visaFetcher}
					selectedId={visaFilter || null}
					selectedLabel={visaFilter || null}
					allowFreeText
					onSelect={(item) => { visaFilter = item.id; applyFilters(); }}
					onClear={() => { visaFilter = ''; applyFilters(); }}
				/>
			</div>
			{#if data.filters.dest || data.filters.visa}
				<a
					href="/planning-groups"
					class="text-sm text-[var(--color-muted)] hover:text-[var(--color-text)]"
				>
					{t('common.clear', 'Clear')}
				</a>
			{/if}
		</form>
			</div>
		{/if}

		{#if data.loadError}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">
				{t('planning_groups.load_error', 'Failed to load planning groups. Please try again later.')}
			</p>
		{:else if data.groups.length === 0}
			<EmptyState
				icon={Users}
				title={t('planning_groups.empty', 'No planning groups yet.')}
				body={t(
					'planning_groups.empty_body',
					'Be the first to create a group for your destination and visa category.'
				)}
				action={data.authed ? { label: t('planning_groups.create', 'Create Group'), href: '/planning-groups/new' } : undefined}
			/>
		{:else}
			{#if visibleGroups.length === 0}
				<p class="py-8 text-center text-sm text-[var(--color-muted)]">
					{t('planning_groups.empty_filtered', 'No groups match this tab.')}
				</p>
			{/if}
			<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
				{#each visibleGroups as group (group.id)}
					<a
						href="/planning-groups/{group.id}"
						class="group flex flex-col gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-primary)]/40 hover:bg-[var(--color-surface-subtle)]"
					>
						<div class="flex items-start justify-between gap-2">
							<h2
								class="truncate text-sm font-semibold text-[var(--color-text)] group-hover:text-[var(--color-primary)]"
							>
								{group.name}
							</h2>
							<span
								class="inline-flex shrink-0 items-center gap-1 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-xs font-medium text-[var(--color-text-subtle)]"
								title={group.is_open_join
									? t('planning_groups.open_join', 'Open to join')
									: t('planning_groups.closed_join', 'Invite only')}
							>
								{#if group.is_open_join}
									<Unlock size={12} />
								{:else}
									<Lock size={12} />
								{/if}
							</span>
						</div>

						{#if group.description}
							<p class="line-clamp-2 text-xs text-[var(--color-text-subtle)]">{group.description}</p>
						{/if}

						<div class="mt-auto flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-[var(--color-muted)]">
							{#if group.visa_category}
								<span class="inline-flex items-center gap-1">
									<Plane size={12} /> {group.visa_category}
								</span>
							{/if}
							{#if group.target_departure_window}
								<span class="inline-flex items-center gap-1">
									<Calendar size={12} /> {group.target_departure_window}
								</span>
							{/if}
						</div>

						<div class="flex items-center justify-between border-t border-[var(--color-border)] pt-3 text-xs text-[var(--color-muted)]">
							<span class="inline-flex items-center gap-1">
								<Users size={12} />
								{group.member_count ?? 0}/{group.max_members ?? 0}
							</span>
							{#if group.visa_category}
								<span>{group.visa_category}</span>
							{/if}
						</div>
					</a>
				{/each}
			</div>
		{/if}
	</div>
</div>
