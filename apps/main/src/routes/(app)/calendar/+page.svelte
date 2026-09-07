<script lang="ts">
	// Ref: japaconnect-engineering/sections/86-community-calendar.md
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import {
		CalendarDays,
		Plus,
		ExternalLink,
		Bookmark,
		BookmarkCheck,
		Bell,
		BellOff,
		RefreshCw,
		AlertTriangle,
		Loader2,
		X,
			Repeat
	} from 'lucide-svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { ListPageBar, UrlTabs, type UrlTab, FilterToggle, PageAction } from '$lib/components/ui';
	import { staticAutocompleteFetcher } from '$lib/autocomplete';
	import type { CalendarEntry, CalendarCategory } from '$lib/types';
	import type { PageData } from './$types';
import { safeJsonLdScript } from '$lib/utils/safeHtml';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('calendar.list_title', 'Community Calendar — JapaConnect'));
	const description = $derived(
		t(
			'calendar.list_description',
			'Verified dates for visa windows, embassy closures, deadlines, and exams.'
		)
	);

	// BUG-009: categories come from the API (calendar.categories system_setting),
	// not a hardcoded array (Rule 16). The server load fetches GET /calendar/categories.
	const categories: { value: CalendarCategory; label: string }[] = $derived(
		(data.categories ?? []).map((c: { value: string; label: string }) => ({
			value: c.value as CalendarCategory,
			label: c.label
		}))
	);

	const views = $derived<UrlTab[]>([
		{ value: 'list', label: t('calendar.view_list', 'List') },
		{ value: 'month', label: t('calendar.view_month', 'Month') },
		{ value: 'week', label: t('calendar.view_week', 'Week') }
	]);

	let showFilters = $state(false);

	// Local filter inputs seeded from URL params.
	let destInput = $state(untrack(() => data.filters.dest));
	let categoryInput = $state(untrack(() => data.filters.category));
	let fromInput = $state(untrack(() => data.filters.from));
	let toInput = $state(untrack(() => data.filters.to));

	$effect(() => {
		destInput = data.filters.dest;
		categoryInput = data.filters.category;
		fromInput = data.filters.from;
		toInput = data.filters.to;
	});

	const categoryItems = $derived<AutocompleteItem[]>(
		categories.map((c) => ({ id: c.value, label: c.label }))
	);
	const categoryFetcher = $derived(staticAutocompleteFetcher(categoryItems));
	const selectedCategoryLabel = $derived(
		categoryInput ? (categoryItems.find((c) => c.id === categoryInput)?.label ?? categoryInput) : null
	);

	const authed = $derived(data.authed);
	const currentView = $derived(data.filters.view);

	// Client-side save/unsave state.
	let savedIds = $state<Set<string>>(new Set());
	let savingId = $state<string | null>(null);

	// Client-side subscribe state.
	let subscribed = $state(false);
	let subscribeLoading = $state(false);

	function buildParams(overrides: Record<string, string> = {}): URLSearchParams {
		const params = new URLSearchParams();
		const view = overrides.view ?? currentView;
		const dest = overrides.dest ?? destInput;
		const category = overrides.category ?? categoryInput;
		const from = overrides.from ?? fromInput;
		const to = overrides.to ?? toInput;
		if (view && view !== 'list') params.set('view', view);
		else if (view === 'list') params.set('view', 'list');
		if (dest) params.set('dest', dest);
		if (category) params.set('category', category);
		if (from) params.set('from', from);
		if (to) params.set('to', to);
		return params;
	}

	function navigate(params: URLSearchParams) {
		goto(`/calendar${params.toString() ? `?${params.toString()}` : ''}`, { keepFocus: true });
	}

	function setView(value: string) {
		navigate(buildParams({ view: value }));
	}

	function applyFilters(e?: SubmitEvent) {
		e?.preventDefault();
		navigate(buildParams());
	}

	function clearFilters() {
		destInput = '';
		categoryInput = '';
		fromInput = '';
		toInput = '';
		navigate(new URLSearchParams({ view: currentView }));
	}

	const hasActiveFilters = $derived(
		!!data.filters.dest || !!data.filters.category || !!data.filters.from || !!data.filters.to
	);

	$effect(() => {
		if (hasActiveFilters) showFilters = true;
	});

	async function toggleSave(entry: CalendarEntry) {
		if (!authed) return;
		savingId = entry.id;
		try {
			if (savedIds.has(entry.id)) {
				await api.delete(`/calendar/${entry.id}/save`);
				savedIds.delete(entry.id);
				savedIds = new Set(savedIds);
			} else {
				await api.post(`/calendar/${entry.id}/save`);
				savedIds.add(entry.id);
				savedIds = new Set(savedIds);
			}
		} finally {
			savingId = null;
		}
	}

	async function toggleSubscribe() {
		if (!authed) return;
		subscribeLoading = true;
		try {
			if (subscribed) {
				await api.delete('/calendar/subscribe', {
					destination_space_id: destInput || undefined,
					category: categoryInput || undefined
				});
				subscribed = false;
			} else {
				await api.post('/calendar/subscribe', {
					destination_space_id: destInput || undefined,
					category: categoryInput || undefined
				});
				subscribed = true;
			}
		} finally {
			subscribeLoading = false;
		}
	}

	function categoryLabel(cat: CalendarCategory): string {
		return categories.find((c) => c.value === cat)?.label ?? cat;
	}

	// BUG-007: check staleness via status === 'unverified' (§86 Staleness
	// Management). Entries flagged by the nightly staleness_check task get
	// status='unverified' and should display a warning.
	function isStale(entry: CalendarEntry): boolean {
		return entry.status === 'unverified';
	}

	function isUnverified(entry: CalendarEntry): boolean {
		return entry.status === 'pending' || isStale(entry);
	}

	function fmtDate(iso: string): string {
		try {
			return new Date(iso).toLocaleString(undefined, {
				year: 'numeric',
				month: 'short',
				day: 'numeric',
				hour: '2-digit',
				minute: '2-digit'
			});
		} catch {
			return iso;
		}
	}

	function fmtDay(iso: string): string {
		try {
			return new Date(iso).toLocaleDateString(undefined, {
				year: 'numeric',
				month: 'short',
				day: 'numeric'
			});
		} catch {
			return iso;
		}
	}

	// Month view: group entries by their occurs_at day.
	// BUG-019: use $derived.by() so dependencies are tracked when the
	// function body runs, not when the function is created.
	const monthGroups = $derived.by(() => {
		const groups = new Map<string, CalendarEntry[]>();
		for (const e of data.entries) {
			const key = fmtDay(e.occurs_at);
			const arr = groups.get(key) ?? [];
			arr.push(e);
			groups.set(key, arr);
		}
		return Array.from(groups.entries()).sort((a, b) => a[0].localeCompare(b[0]));
	});

	const jsonLd = $derived(
		data.entries.length > 0
			? JSON.stringify({
					'@context': 'https://schema.org',
					'@type': 'ItemList',
					name: t('calendar.heading', 'Community Calendar'),
					itemListElement: data.entries.map((e, i) => ({
						'@type': 'ListItem',
						position: i + 1,
						item: {
							'@type': 'Event',
							name: e.title,
							startDate: e.occurs_at,
							...(e.ends_at ? { endDate: e.ends_at } : {}),
							eventStatus: e.status === 'published' ? 'https://schema.org/EventScheduled' : 'https://schema.org/EventPostponed',
							url: `${page.url.origin}/calendar/${e.id}`
						}
					}))
				})
			: ''
	);

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';

	function fetchDestinationCountries(q: string): Promise<AutocompleteItem[]> {
		const allLabel = t('calendar.filter_dest_all', 'All countries');
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
		destInput ? (data.countries ?? []).find((c) => c.id === destInput)?.name ?? null : null
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
	{#if jsonLd}
		{@html safeJsonLdScript(jsonLd)}
	{/if}
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<h1 class="sr-only">{t('calendar.heading', 'Community Calendar')}</h1>
	<ListPageBar>
		<UrlTabs
			tabs={views}
			value={currentView}
			onValueChange={(v) => setView(v)}
			class="!border-b-0 !bg-transparent"
		/>
		{#snippet actions()}
			<FilterToggle bind:show={showFilters} active={hasActiveFilters} controls="calendar-filter-panel" />
			{#if authed}
				<button
					type="button"
					onclick={toggleSubscribe}
					disabled={subscribeLoading}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
				>
					{#if subscribeLoading}
						<Loader2 size={14} class="animate-spin" />
					{:else if subscribed}
						<BellOff size={14} />
					{:else}
						<Bell size={14} />
					{/if}
					{subscribed ? t('calendar.unsubscribe', 'Unsubscribe') : t('calendar.subscribe', 'Subscribe')}
				</button>
				<PageAction
					href="/calendar/new"
					icon={Plus}
					label={t('calendar.submit_event', 'Submit Event')}
				/>
			{:else}
				<PageAction
					href="/login?redirect=/calendar/new"
					icon={Plus}
					label={t('calendar.submit_event', 'Submit Event')}
				/>
			{/if}
		{/snippet}
	</ListPageBar>

	<!-- Content -->
	<div class="p-4">
		{#if showFilters}
			<!-- Filter panel -->
			<div id="calendar-filter-panel" class="mb-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
		<form onsubmit={applyFilters} class="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
			<Autocomplete
				placeholder={t('calendar.filter_dest_all', 'All countries')}
				ariaLabel={t('calendar.filter_dest_all', 'All countries')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={fetchDestinationCountries}
				initialFetcher={fetchDestinationCountriesInitial}
				selectedId={destInput || null}
				selectedLabel={selectedDestinationLabel}
				onSelect={(item) => { destInput = item.id; applyFilters(); }}
				onClear={() => { destInput = ''; applyFilters(); }}
			/>
			<Autocomplete
				placeholder={t('calendar.filter_category', 'All categories')}
				ariaLabel={t('calendar.filter_category', 'All categories')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={categoryFetcher}
				initialFetcher={categoryFetcher}
				selectedId={categoryInput || null}
				selectedLabel={selectedCategoryLabel}
				onSelect={(item) => { categoryInput = item.id; applyFilters(); }}
				onClear={() => { categoryInput = ''; applyFilters(); }}
			/>
			<div class="flex gap-2">
				<input
					type="date"
					placeholder={t('calendar.filter_from', 'From')}
					value={fromInput}
					onchange={(e) => { fromInput = e.currentTarget.value; applyFilters(); }}
					class={inputClass}
					aria-label={t('calendar.filter_from', 'From')}
				/>
				<input
					type="date"
					placeholder={t('calendar.filter_to', 'To')}
					value={toInput}
					onchange={(e) => { toInput = e.currentTarget.value; applyFilters(); }}
					class={inputClass}
					aria-label={t('calendar.filter_to', 'To')}
				/>
			</div>
		</form>
		{#if hasActiveFilters}
			<div class="mt-2">
				<button
					type="button"
					onclick={clearFilters}
					class="inline-flex items-center gap-1 text-xs text-[var(--color-muted)] underline hover:text-[var(--color-text)]"
				>
					<X size={12} /> {t('common.clear_filters', 'Clear filters')}
				</button>
			</div>
		{/if}
			</div>
		{/if}

		{#if data.loadError}
			<EmptyState
				icon={CalendarDays}
				title={t('calendar.load_error', 'Could not load calendar')}
				body={t('calendar.load_error_body', 'Please try again later.')}
			/>
		{:else if data.entries.length === 0}
			<EmptyState
				icon={CalendarDays}
				title={t('calendar.empty', 'No calendar entries')}
				body={t('calendar.empty_body', 'No upcoming dates match your filters.')}
				action={authed
					? { label: t('calendar.submit_event', 'Submit Event'), href: '/calendar/new' }
					: { label: t('calendar.submit_event', 'Submit Event'), href: '/login?redirect=/calendar/new' }}
			/>
		{:else if currentView === 'month'}
			<!-- Month view: simple calendar grid grouped by date -->
			<div class="space-y-3">
				{#each monthGroups as [day, dayEntries] (day)}
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
						<div class="border-b border-[var(--color-border)] px-4 py-2 text-sm font-semibold text-[var(--color-text)]">
							{day}
						</div>
						<div class="divide-y divide-[var(--color-border)]">
							{#each dayEntries as entry (entry.id)}
								{@render entryRow(entry)}
							{/each}
						</div>
					</div>
				{/each}
			</div>
		{:else if currentView === 'week'}
			<!-- Week view: same chronological grouping as month, simpler label -->
			<div class="space-y-3">
				{#each monthGroups as [day, dayEntries] (day)}
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
						<div class="border-b border-[var(--color-border)] px-4 py-2 text-sm font-semibold text-[var(--color-text)]">
							{day}
						</div>
						<div class="divide-y divide-[var(--color-border)]">
							{#each dayEntries as entry (entry.id)}
								{@render entryRow(entry)}
							{/each}
						</div>
					</div>
				{/each}
			</div>
		{:else}
			<!-- List view: chronological list -->
			<div class="divide-y divide-[var(--color-border)] rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
				{#each data.entries as entry (entry.id)}
					{@render entryRow(entry)}
				{/each}
			</div>
		{/if}
	</div>
</div>

{#snippet entryRow(entry: CalendarEntry)}
	<div class="flex items-start gap-3 px-4 py-3">
		<a href="/calendar/{entry.id}" class="min-w-0 flex-1">
			<div class="flex items-start justify-between gap-2">
				<h2 class="text-sm font-semibold text-[var(--color-text)] hover:text-[var(--color-primary)]">
					{entry.title}
				</h2>
				<span class="shrink-0 rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs font-medium text-[var(--color-text-subtle)]">
					{categoryLabel(entry.category as CalendarCategory)}
				</span>
			</div>

			<div class="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-[var(--color-muted)]">
				<span class="inline-flex items-center gap-1">
					<CalendarDays size={12} />
					{fmtDate(entry.occurs_at)}
				</span>
				{#if entry.ends_at}
					<span class="inline-flex items-center gap-1">
						→ {fmtDate(entry.ends_at)}
					</span>
				{/if}

				{#if entry.is_recurring}
					<span class="inline-flex items-center gap-1 text-[var(--color-primary)]">
						<Repeat size={12} />
						{entry.recurrence_pattern ?? t('calendar.recurring', 'Recurring')}
					</span>
				{/if}
			</div>

			{#if isUnverified(entry)}
				<p class="mt-1.5 inline-flex items-center gap-1 text-xs text-amber-600">
					<AlertTriangle size={12} />
					{isStale(entry)
						? t('calendar.stale_warning', 'Entry may be stale — verify with the official source.')
						: t('calendar.unverified_warning', 'Unverified — confirm with the official source.')}
				</p>
			{/if}
		</a>

		<div class="flex shrink-0 items-center gap-2">
			<a
				href={entry.source_url}
				target="_blank"
				rel="noopener noreferrer nofollow"
				class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-2 py-1 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
				aria-label={t('calendar.source', 'Official source')}
			>
				<ExternalLink size={12} /> {t('calendar.source', 'Source')}
			</a>
			{#if authed}
				<button
					type="button"
					onclick={() => toggleSave(entry)}
					disabled={savingId === entry.id}
					class="rounded-[var(--radius)] border border-[var(--color-border)] p-1.5 text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					aria-label={savedIds.has(entry.id) ? t('common.saved', 'Saved') : t('common.save', 'Save')}
				>
					{#if savingId === entry.id}
						<Loader2 size={14} class="animate-spin" />
					{:else if savedIds.has(entry.id)}
						<BookmarkCheck size={14} class="text-[var(--color-primary)]" />
					{:else}
						<Bookmark size={14} />
					{/if}
				</button>
			{/if}
		</div>
	</div>
{/snippet}
