<script lang="ts">
	// Ref: japaconnect-engineering/sections/102-mentorship-matching.md
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import UserHoverCard from '$lib/components/ui/UserHoverCard.svelte';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { goto } from '$app/navigation';
	import { enhance } from '$app/forms';
	import { page } from '$app/state';
	import { GraduationCap, X, GitCompare, UserPlus, Loader2 } from 'lucide-svelte';
	import { EmptyState, ListPageBar, UrlTabs, type UrlTab, FilterToggle, PageAction } from '$lib/components/ui';
	import { staticAutocompleteFetcher, autocompleteItems } from '$lib/autocomplete';
	import type { Mentor } from '$lib/types';
	import type { PageData, ActionData } from './$types';

	let { data, form }: { data: PageData; form?: ActionData } = $props();

	const user = $derived(page.data.user);
	const mentors = $derived<Mentor[]>(data.mentors);
	const meta = $derived(data.meta);
	const totalPages = $derived(meta?.total_pages ?? 1);
	const currentPage = $derived(data.filters.page);
	// §102: if the viewer is already a mentor, the action links to their dashboard.
	const userIsMentor = $derived(!!user && mentors.some((m) => m.user_id === user.id));
	// §102: comparison UI is gated by the mentorship.comparison feature flag.
	const comparisonEnabled = $derived(page.data.features?.['mentorship.comparison'] === true);

	// Filter bar state — seeded from the server-provided filter values.
	let dest = $state(untrack(() => data.filters.dest));
	let areas = $state(untrack(() => data.filters.areas));
	let sort = $state(untrack(() => data.filters.sort));

	// Re-seed when the server payload changes (e.g. browser back/forward).
	$effect(() => {
		dest = data.filters.dest;
		areas = data.filters.areas;
		sort = data.filters.sort;
	});

	const sortOptions = [
		{ value: 'match', label: t('mentors.sort_match', 'Best match') },
		{ value: 'newest', label: t('mentors.sort_newest', 'Newest') },
		{ value: 'capacity', label: t('mentors.sort_capacity', 'Most capacity') }
	];

	const tabs = $derived<UrlTab[]>(sortOptions.map((o) => ({ value: o.value, label: o.label })));

	function selectSort(v: string) {
		const params = new URLSearchParams();
		if (data.filters.dest) params.set('dest', data.filters.dest);
		if (data.filters.areas) params.set('areas', data.filters.areas);
		if (v && v !== 'match') params.set('sort', v);
		const qs = params.toString();
		goto(`/mentors${qs ? `?${qs}` : ''}`);
	}

	let showFilters = $state(false);
	$effect(() => {
		if (hasFilters) showFilters = true;
	});

	const MAX_COMPARE = 4;
	let selectedIds = $state<Set<string>>(new Set());
	let savingJCE = $state(false);

	const canCompare = $derived(selectedIds.size >= 2 && selectedIds.size <= MAX_COMPARE);
	const compareHref = $derived(
		`/compare?type=mentors&preselect=${[...selectedIds].join(',')}`
	);

	const title = $derived(t('mentors.list_title', 'Find a Mentor — JapaConnect'));
	const description = $derived(
		t('mentors.list_description', 'Discover experienced diaspora mentors to guide your journey.')
	);

	function applyFilters(e?: SubmitEvent) {
		e?.preventDefault();
		const params = new URLSearchParams();
		if (dest.trim()) params.set('dest', dest.trim());
		if (areas.trim()) params.set('areas', areas.trim());
		if (data.filters.sort) params.set('sort', data.filters.sort);
		const qs = params.toString();
		goto(`/mentors${qs ? `?${qs}` : ''}`);
	}

	function clearFilters() {
		dest = '';
		areas = '';
		goto('/mentors');
	}

	const hasFilters = $derived(!!data.filters.dest || !!data.filters.areas);

	// areas is a free-text filter — suggest areas seen in the loaded mentors,
	// allow arbitrary input via allowFreeText.
	const areasFetcher = $derived(
		staticAutocompleteFetcher(
			autocompleteItems(mentors.flatMap((m) => m.mentor_areas ?? []))
		)
	);

	function toggleCompare(mentor: Mentor) {
		const next = new Set(selectedIds);
		if (next.has(mentor.user_id)) {
			next.delete(mentor.user_id);
		} else if (next.size < MAX_COMPARE) {
			next.add(mentor.user_id);
		}
		selectedIds = next;
	}

	function capacityPct(m: Mentor): number {
		const cap = m.mentor_capacity ?? 0;
		const active = m.active_mentee_count ?? 0;
		if (cap <= 0) return 0;
		return Math.min(100, Math.round((active / cap) * 100));
	}

	function capacityLabel(m: Mentor): string {
		const cap = m.mentor_capacity ?? 0;
		const active = m.active_mentee_count ?? 0;
		return `${active}/${cap}`;
	}

	function hasCapacity(m: Mentor): boolean {
		const cap = m.mentor_capacity ?? 0;
		const active = m.active_mentee_count ?? 0;
		return cap > 0 && active < cap;
	}

	function pageHref(p: number): string {
		const params = new URLSearchParams();
		if (dest.trim()) params.set('dest', dest.trim());
		if (areas.trim()) params.set('areas', areas.trim());
		if (sort) params.set('sort', sort);
		params.set('page', String(p));
		return `/mentors?${params.toString()}`;
	}

	function fetchDestinationCountries(q: string): Promise<AutocompleteItem[]> {
		const allLabel = t('mentors.filter_destination_placeholder', 'All countries');
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
		dest ? (data.countries ?? []).find((c) => c.id === dest)?.name ?? null : null
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<h1 class="sr-only">{t('mentors.heading', 'Find a Mentor')}</h1>
	<ListPageBar>
		<UrlTabs
			{tabs}
			value={data.filters.sort || 'match'}
			onValueChange={(v) => selectSort(v)}
			class="!border-b-0 !bg-transparent"
		/>
		{#snippet actions()}
			<FilterToggle bind:show={showFilters} active={hasFilters} controls="mentors-filter-panel" />
			{#if user}
				<PageAction
					href="/mentors/me"
					icon={UserPlus}
					label={userIsMentor
						? t('mentors.my_mentorship', 'My Mentorship')
						: t('mentors.become_mentor', 'Become a Mentor')}
				/>
			{/if}
		{/snippet}
	</ListPageBar>

	<!-- Content -->
	<div class="p-4">
		{#if showFilters}
			<!-- Filter panel -->
			<div id="mentors-filter-panel" class="mb-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
	<form onsubmit={applyFilters}>
		<div class="flex flex-wrap items-center gap-3">
			<div class="w-52">
				<Autocomplete
					placeholder={t('mentors.filter_destination', 'Destination')}
					ariaLabel={t('mentors.filter_destination', 'Destination')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={fetchDestinationCountries}
					initialFetcher={fetchDestinationCountriesInitial}
					selectedId={dest || null}
					selectedLabel={selectedDestinationLabel}
					onSelect={(item) => { dest = item.id; applyFilters(); }}
					onClear={() => { dest = ''; applyFilters(); }}
				/>
			</div>

			<div class="w-52">
				<Autocomplete
					placeholder={t('mentors.filter_areas', 'Areas')}
					ariaLabel={t('mentors.filter_areas', 'Areas')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={areasFetcher}
					initialFetcher={areasFetcher}
					selectedId={areas || null}
					selectedLabel={areas || null}
					allowFreeText
					onSelect={(item) => { areas = item.id; applyFilters(); }}
					onClear={() => { areas = ''; applyFilters(); }}
				/>
			</div>

			<div class="flex items-center gap-2">
				{#if hasFilters}
					<button
						type="button"
						onclick={clearFilters}
						class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						<X size={14} /> {t('common.clear', 'Clear')}
					</button>
				{/if}
			</div>
		</div>
	</form>
			</div>
		{/if}

		{#if data.loadError}
			<EmptyState
				icon={GraduationCap}
				title={t('mentors.load_error', 'Could not load mentors')}
				body={t('mentors.load_error_body', 'Please try again later.')}
			/>
		{:else if mentors.length === 0}
			<EmptyState
				icon={GraduationCap}
				title={t('mentors.empty_title', 'No mentors found')}
				body={t('mentors.empty_body', 'Try adjusting your filters, or check back soon.')}
				action={user
					? {
							label: userIsMentor
								? t('mentors.my_mentorship', 'My Mentorship')
								: t('mentors.become_mentor', 'Become a Mentor'),
							href: '/mentors/me'
						}
					: { label: t('common.login', 'Log in'), href: '/login?redirect=/mentors' }}
			/>
		{:else}
			<div class="space-y-3">
				{#each mentors as mentor (mentor.user_id)}
					{@const selected = selectedIds.has(mentor.user_id)}
					<div
						class="rounded-[var(--radius-card)] border p-4 transition-colors {selected
							? 'border-[var(--color-primary)] bg-[var(--color-primary)]/5'
							: 'border-[var(--color-border)] bg-[var(--color-surface)]'}"
					>
						<div class="flex items-start gap-3">
							<UserHoverCard
									id={mentor.user_id}
									slug={mentor.username}
									name={mentor.display_name ?? mentor.username}
									avatar={mentor.avatar_url}
									class="shrink-0"
								>
									{#snippet trigger()}
										{#if mentor.avatar_url}
											<img
												src={mentor.avatar_url}
												alt={mentor.username}
												class="h-12 w-12 rounded-full object-cover"
											/>
										{:else}
											<div class="grid h-12 w-12 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-muted)]">
												<GraduationCap size={20} />
											</div>
										{/if}
									{/snippet}
								</UserHoverCard>


							<div class="min-w-0 flex-1">
								<div class="flex flex-wrap items-center gap-2">
									<UserHoverCard
										id={mentor.user_id}
										slug={mentor.username}
										name={mentor.display_name ?? mentor.username}
										avatar={mentor.avatar_url}
										variant="row"
										class="text-sm font-semibold text-[var(--color-text)] hover:text-[var(--color-primary)]"
									>
										{#snippet trigger()}
											{mentor.username}
										{/snippet}
									</UserHoverCard>
									{#if hasCapacity(mentor)}
										<span class="inline-flex items-center rounded-full bg-green-500/10 px-2 py-0.5 text-[10px] font-medium text-green-600">
											{t('mentors.spots_open', 'Spots open')}
										</span>
									{/if}
								</div>

								{#if mentor.mentor_bio}
									<p class="mt-1 line-clamp-2 text-sm text-[var(--color-text-subtle)]">
										{mentor.mentor_bio}
									</p>
								{/if}

								{#if mentor.mentor_areas && mentor.mentor_areas.length > 0}
									<div class="mt-2 flex flex-wrap gap-1.5">
										{#each mentor.mentor_areas as area (area)}
											<span class="inline-flex items-center rounded-full border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-2 py-0.5 text-[11px] font-medium text-[var(--color-text-subtle)]">
												{area}
											</span>
										{/each}
									</div>
								{/if}

								<!-- Capacity indicator -->
								<div class="mt-3 flex items-center gap-2">
									<div class="h-1.5 w-28 overflow-hidden rounded-full bg-[var(--color-surface-subtle)]">
										<div
											class="h-full rounded-full bg-[var(--color-primary)]"
											style="width: {capacityPct(mentor)}%"
										></div>
									</div>
									<span class="text-xs text-[var(--color-muted)]">
										{t('mentors.capacity', 'Mentees')} {capacityLabel(mentor)}
									</span>
								</div>
							</div>

							<div class="flex shrink-0 flex-col items-end gap-2">
								<a
									href="/mentors/{mentor.username}"
									class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
								>
									{t('mentors.request', 'Request Mentorship')}
								</a>
								{#if comparisonEnabled}
									<label class="inline-flex items-center gap-1.5 text-xs text-[var(--color-text-subtle)]">
										<input
											type="checkbox"
											checked={selected}
											onchange={() => toggleCompare(mentor)}
											disabled={!selected && selectedIds.size >= MAX_COMPARE}
											class="h-4 w-4 rounded border-[var(--color-border)]"
										/>
										{t('mentors.compare_select', 'Compare')}
									</label>
								{/if}
							</div>
						</div>
					</div>
				{/each}
			</div>

			<!-- Compare bar -->
			{#if comparisonEnabled && selectedIds.size > 0}
				<div class="sticky bottom-3 mt-4 flex flex-wrap items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2.5 shadow-sm">
					<span class="text-sm text-[var(--color-text-subtle)]">
						{selectedIds.size}/{MAX_COMPARE} {t('mentors.compare_selected', 'selected')}
					</span>
					<div class="flex items-center gap-2">
						<button
							type="button"
							onclick={() => (selectedIds = new Set())}
							class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							{t('common.clear', 'Clear')}
						</button>
						<a
							href={compareHref}
							class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] {canCompare ? '' : 'pointer-events-none opacity-50'}"
						>
							<GitCompare size={14} /> {t('mentors.compare', 'Compare')}
						</a>
						<form
							method="POST"
							use:enhance={() => {
								savingJCE = true;
								return async ({ update }) => {
									savingJCE = false;
									await update();
								};
							}}
							class="contents"
							>
							{#each [...selectedIds] as id (id)}
								<input type="hidden" name="mentor_id" value={id} />
							{/each}
							<button
								type="submit"
								disabled={!canCompare || !data.authed}
								title={data.authed
									? t('mentors.open_in_jce_hint', 'Open in JCE comparison')
									: t('mentors.sign_in_to_compare', 'Sign in to compare')}
								class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50 disabled:cursor-not-allowed"
							>
								{#if savingJCE}
									<Loader2 size={14} class="animate-spin" />
								{:else}
									<GitCompare size={14} />
								{/if}
								{t('mentors.open_in_jce', 'Compare in JCE')}
							</button>
						</form>
					</div>
					{#if form?.error}
						<div class="mt-2 w-full rounded-[var(--radius)] border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300">
							{form.message || t('mentors.jce_error', 'Could not create comparison')}
						</div>
					{/if}
				</div>
			{/if}

			<!-- Pagination -->
			{#if totalPages > 1}
				<nav class="mt-6 flex items-center justify-center gap-2" aria-label={t('common.pagination', 'Pagination')}>
					{#if currentPage > 1}
						<a
							href={pageHref(currentPage - 1)}
							class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							{t('common.prev', 'Previous')}
						</a>
					{/if}
					<span class="text-sm text-[var(--color-muted)]">
						{currentPage} / {totalPages}
					</span>
					{#if currentPage < totalPages}
						<a
							href={pageHref(currentPage + 1)}
							class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							{t('common.next', 'Next')}
						</a>
					{/if}
				</nav>
			{/if}
		{/if}
	</div>
</div>
