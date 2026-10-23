<script lang="ts">
	import { untrack } from 'svelte';
	// Ref: japaconnect-engineering/sections/87-japa-stage-profiles.md
	// Japa Stage directory. SSR list of public member profiles from
	// GET /members/by-stage with ?stage filter and pagination.
	import { t } from '$lib/i18n';
	import UserHoverCard from '$lib/components/ui/UserHoverCard.svelte';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import PaginationControls from '$lib/components/ui/PaginationControls.svelte';
	import RelativeTime from '$lib/components/ui/RelativeTime.svelte';
	import { Compass, Users } from 'lucide-svelte';
	import { ListPageBar, UrlTabs, type UrlTab, FilterToggle, PageAction } from '$lib/components/ui';
	import { japaStageColors, japaStageLabel, japaStageOptions, japaStagesForIntent, japaIntentLabel, japaIntentOptions } from '$lib/enumOptions';
	import type { JapaStage, JapaIntent, MemberStage } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const intentOpts = $derived(japaIntentOptions(t, data.intents));

	// Local filter state, seeded from SSR-passed filters.
	let destFilter = $state(untrack(() => data.filters.dest));
	let intentFilter = $state(untrack(() => data.filters.intent));

	// Keep local filter state in sync when navigating between pages/filters.
	$effect(() => {
		destFilter = data.filters.dest;
		intentFilter = data.filters.intent;
	});

	function applyFilters() {
		const params = new URLSearchParams();
		if (data.filters.stage) params.set('stage', data.filters.stage);
		if (intentFilter) params.set('intent', intentFilter);
		if (destFilter) params.set('dest', destFilter);
		goto(`/japa-stage${params.toString() ? `?${params.toString()}` : ''}`);
	}

	function clearFilters() {
		goto('/japa-stage');
	}

	function selectStage(v: string) {
		const params = new URLSearchParams();
		if (v !== 'all') params.set('stage', v);
		if (data.filters.intent) params.set('intent', data.filters.intent);
		if (data.filters.dest) params.set('dest', data.filters.dest);
		goto(`/japa-stage${params.toString() ? `?${params.toString()}` : ''}`);
	}

	function changePage(p: number) {
		const params = new URLSearchParams();
		if (data.filters.stage) params.set('stage', data.filters.stage);
		if (data.filters.intent) params.set('intent', data.filters.intent);
		if (data.filters.dest) params.set('dest', data.filters.dest);
		params.set('page', String(p));
		goto(`/japa-stage?${params.toString()}`);
	}

	const title = $derived(t('japa_stage.directory_title', 'Japa Stage — Find Members by Stage'));
	const description = $derived(
		t(
			'japa_stage.directory_description',
			'Discover diaspora members by where they are in their japa journey.'
		)
	);

	const hasFilters = $derived(!!data.filters.dest || !!data.filters.intent);

	// Stage tabs narrow to the selected journey class — a stage outside the
	// class's applicable set can never match (ADR-122 class-scoped terminals).
	const stageOpts = $derived(
		japaStageOptions(t, japaStagesForIntent(data.stages, (data.filters.intent || intentFilter) as JapaIntent | ''))
	);

	const tabs = $derived<UrlTab[]>([
		{ value: 'all', label: t('japa_stage.all_stages', 'All stages') },
		...stageOpts.map((o) => ({ value: o.value, label: o.label }))
	]);

	let showFilters = $state(false);
	$effect(() => {
		if (hasFilters) showFilters = true;
	});

	function fetchDestinationCountries(q: string): Promise<AutocompleteItem[]> {
		const allLabel = t('japa_stage.all_destinations', 'All destinations');
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
</svelte:head>

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 48rem)/2))">
	<h1 class="sr-only">{t('japa_stage.heading', 'Japa Stage — Find Members by Stage')}</h1>
	<ListPageBar>
		<UrlTabs
			{tabs}
			value={data.filters.stage || 'all'}
			onValueChange={(v) => selectStage(v)}
			class="!border-b-0 !bg-transparent"
		/>
		{#snippet actions()}
			<FilterToggle bind:show={showFilters} active={hasFilters} controls="japa-stage-filter-panel" />
			<PageAction
				href={data.authed ? '/japa-stage/me' : '/login?redirect=/japa-stage/me'}
				icon={Compass}
				label={data.authed
					? t('japa_stage.my_stage', 'My Stage')
					: t('japa_stage.set_my_stage', 'Set my stage')}
			/>
		{/snippet}
	</ListPageBar>

	<div class="mx-auto w-full max-w-3xl p-4">
		{#if showFilters}
		<!-- Filter panel -->
		<div id="japa-stage-filter-panel" class="mb-4 flex flex-wrap items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
			<div class="w-44">
				<label for="intent-filter" class="sr-only">{t('japa_intent.filter', 'Journey type')}</label>
				<AutocompleteSelect
					id="intent-filter"
					value={intentFilter}
					options={[
						{ value: '', label: t('japa_intent.all', 'All journey types') },
						...intentOpts
					]}
					onValueChange={(v) => {
						intentFilter = v;
						applyFilters();
					}}
				/>
			</div>
			<div class="w-52">
				<Autocomplete
					placeholder={t('japa_stage.filter_dest', 'Destination')}
					ariaLabel={t('japa_stage.filter_dest', 'Destination')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={fetchDestinationCountries}
					initialFetcher={fetchDestinationCountriesInitial}
					selectedId={destFilter || null}
					selectedLabel={selectedDestinationLabel}
					onSelect={(item) => { destFilter = item.id; applyFilters(); }}
					onClear={() => { destFilter = ''; applyFilters(); }}
				/>
			</div>
			{#if hasFilters}
				<button
					type="button"
					onclick={clearFilters}
					class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
				>
					{t('common.clear', 'Clear')}
				</button>
			{/if}
		</div>
		{/if}

		<!-- Content -->
		{#if data.loadError}
			<EmptyState
				icon={Users}
				title={t('japa_stage.load_error', 'Could not load members')}
				body={t('japa_stage.load_error_body', 'Please try again later.')}
			/>
		{:else if data.members.length === 0}
			<EmptyState
				icon={Users}
				title={t('japa_stage.empty', 'No members found')}
				body={t('japa_stage.empty_body', 'Try adjusting your filters.')}
				action={data.authed
					? { label: t('japa_stage.set_my_stage', 'Set my stage'), href: '/japa-stage/me' }
					: undefined}
			/>
		{:else}
			<ul class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
				{#each data.members as m (m.user_id)}
					{@render memberCard(m)}
				{/each}
			</ul>

			{#if data.totalPages > 1}
				<div class="mt-4 flex justify-center">
					<PaginationControls
						page={data.currentPage}
						totalPages={data.totalPages}
						onPageChange={changePage}
					/>
				</div>
			{/if}
		{/if}
	</div>
</div>

{#snippet memberCard(m: MemberStage)}
	<li>
		<UserHoverCard
			id={m.user_id}
			slug={m.username}
			name={m.display_name ?? m.username ?? ''}
			avatar={m.avatar_url}
			class="inline-flex w-full min-w-0 max-w-full items-start gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 text-start transition-colors hover:bg-[var(--color-surface-subtle)]"
		>
			{#snippet trigger()}
				<span class="grid h-10 w-10 shrink-0 place-items-center overflow-hidden rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-muted)]">
					{#if m.avatar_url}
						<img
							src={m.avatar_url}
							alt={m.username}
							class="h-full w-full object-cover"
							loading="lazy"
						/>
					{:else}
						<Users size={20} />
					{/if}
				</span>
				<div class="min-w-0 flex-1">
					<p class="truncate text-sm font-semibold text-[var(--color-text)]">@{m.username}</p>
					<div class="mt-1 flex flex-wrap items-center gap-1.5">
					{#if m.stage}
						<span
							class="inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium"
							style={`background-color: ${japaStageColors[m.stage as JapaStage]}1a; color: ${japaStageColors[m.stage as JapaStage]};`}
						>
							<span
								class="inline-block h-1.5 w-1.5 rounded-full"
								style={`background-color: ${japaStageColors[m.stage as JapaStage]};`}
							></span>
							{japaStageLabel(t, m.stage as JapaStage)}
						</span>
					{/if}
					{#if m.intent}
						<span
							class="inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium"
							style="background-color: var(--color-surface-subtle); color: var(--color-text-subtle);"
						>
							{japaIntentLabel(t, m.intent as JapaIntent)}
						</span>
					{/if}
				</div>
				<div class="mt-1.5 flex flex-col gap-0.5 text-xs text-[var(--color-text-subtle)]">
					{#if m.destination_space_name}
						<span>{t('japa_stage.dest', 'Destination')}: {m.destination_space_name}</span>
					{/if}
					{#if m.updated_at}
						<span>{t('japa_stage.updated', 'Updated')} <RelativeTime iso={m.updated_at} /></span>
					{/if}
				</div>
			</div>
		{/snippet}
		</UserHoverCard>
	</li>
{/snippet}
