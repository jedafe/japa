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
		AlertTriangle,
		Loader2,
		X,
		ChevronLeft,
		ChevronRight,
		Repeat
	} from 'lucide-svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { ListPageBar, UrlTabs, type UrlTab, FilterToggle, PageAction } from '$lib/components/ui';
	import { staticAutocompleteFetcher } from '$lib/autocomplete';
	import type { CalendarEntry, CalendarCategory } from '$lib/types';
	import type { PageData } from './$types';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('calendar.list_title', 'Community Calendar — {site}'));
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
	const focus = $derived(data.filters.focus);

	// ── Save state ──────────────────────────────────────────────────────────
	// Entries fetched client-side (Load more) merge with the SSR page data.
	let extraEntries = $state<CalendarEntry[]>([]);
	const allEntries = $derived([...data.entries, ...extraEntries]);

	const baseSavedIds = $derived(new Set(allEntries.filter((e) => e.is_saved).map((e) => e.id)));
	let savedOverrides = $state<Record<string, boolean>>({});
	const savedIds = $derived.by(() => {
		const s = new Set(baseSavedIds);
		for (const [k, v] of Object.entries(savedOverrides)) {
			if (v) s.add(k);
			else s.delete(k);
		}
		return s;
	});
	let savingId = $state<string | null>(null);
	let actionError = $state('');

	$effect(() => {
		data.entries;
		savedOverrides = {};
		extraEntries = [];
	});

	// ── Subscribe state — seeded from the list response (meta.subscribed). ──
	let subscribed = $state(untrack(() => data.subscribed));
	let subscribeLoading = $state(false);

	$effect(() => {
		subscribed = data.subscribed;
	});

	function buildParams(overrides: Record<string, string> = {}): URLSearchParams {
		const params = new URLSearchParams();
		const view = overrides.view ?? currentView;
		const dest = overrides.dest ?? destInput;
		const category = overrides.category ?? categoryInput;
		const from = overrides.from ?? fromInput;
		const to = overrides.to ?? toInput;
		const focusV = overrides.focus ?? focus;
		if (view) params.set('view', view);
		if (dest) params.set('dest', dest);
		if (category) params.set('category', category);
		if (from && view === 'list') params.set('from', from);
		if (to && view === 'list') params.set('to', to);
		if (view !== 'list' && focusV) params.set('focus', focusV);
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
		actionError = '';
		const next = !savedIds.has(entry.id);
		const res = next
			? await api.post(`/calendar/${entry.id}/save`)
			: await api.delete(`/calendar/${entry.id}/save`);
		// Only apply the optimistic state when the API confirmed — the api
		// client never throws, so res.error must be checked explicitly.
		if (res.error) {
			actionError = res.error.message;
		} else {
			savedOverrides = { ...savedOverrides, [entry.id]: next };
		}
		savingId = null;
	}

	async function toggleSubscribe() {
		if (!authed || !data.filters.dest || subscribeLoading) return;
		subscribeLoading = true;
		actionError = '';
		const body = {
			destination_space_id: data.filters.dest,
			category: data.filters.category || undefined
		};
		const res = subscribed
			? await api.delete('/calendar/subscribe', body)
			: await api.post('/calendar/subscribe', body);
		if (res.error) {
			actionError = res.error.message;
		} else {
			subscribed = !subscribed;
		}
		subscribeLoading = false;
	}

	// ── Load more (offset pagination) ───────────────────────────────────────
	const hasMore = $derived(
		(data.meta?.total ?? 0) > allEntries.length && allEntries.length >= data.pageSize
	);
	let loadingMore = $state(false);

	async function loadMore() {
		if (loadingMore || !hasMore) return;
		loadingMore = true;
		const params = new URLSearchParams({ page_size: String(data.pageSize) });
		params.set('page_offset', String(allEntries.length));
		if (data.filters.dest) params.set('dest', data.filters.dest);
		if (data.filters.category) params.set('category', data.filters.category);
		if (data.rangeFrom) params.set('from', data.rangeFrom);
		if (data.rangeTo) params.set('to', data.rangeTo);
		const res = await api.get<CalendarEntry[]>(`/calendar?${params.toString()}`);
		loadingMore = false;
		if (res.data?.length) {
			extraEntries = [...extraEntries, ...res.data];
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
				weekday: 'short',
				year: 'numeric',
				month: 'short',
				day: 'numeric'
			});
		} catch {
			return iso;
		}
	}

	// ── Date helpers for month/week views ───────────────────────────────────
	function dayKey(iso: string): string {
		const d = new Date(iso);
		if (isNaN(d.getTime())) return '';
		const pad = (n: number) => String(n).padStart(2, '0');
		return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
	}

	// Entries indexed by local calendar day. Multi-day entries appear on every
	// covered day (span capped at 62 days to bound rendering).
	const entriesByDay = $derived.by(() => {
		const map = new Map<string, CalendarEntry[]>();
		for (const e of allEntries) {
			const start = new Date(e.occurs_at);
			if (isNaN(start.getTime())) continue;
			const end = e.ends_at ? new Date(e.ends_at) : start;
			const days = Math.min(
				62,
				Math.max(1, Math.round((end.getTime() - start.getTime()) / 86400000) + 1)
			);
			for (let i = 0; i < days; i++) {
				const d = new Date(start.getTime() + i * 86400000);
				const pad = (n: number) => String(n).padStart(2, '0');
				const key = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
				const arr = map.get(key) ?? [];
				arr.push(e);
				map.set(key, arr);
			}
		}
		return map;
	});

	// Sorted day list for grouped (month-list) rendering — chronological.
	const dayGroups = $derived(
		Array.from(entriesByDay.entries()).sort((a, b) => a[0].localeCompare(b[0]))
	);

	// ── Month grid model ────────────────────────────────────────────────────
	const monthLabel = $derived(
		focus
			? new Date(`${focus}-01T00:00:00`).toLocaleDateString(undefined, {
					year: 'numeric',
					month: 'long'
				})
			: ''
	);

	const monthWeeks = $derived.by(() => {
		if (currentView !== 'month' || !focus) return [];
		const [y, m] = focus.split('-').map(Number);
		const first = new Date(y, m - 1, 1);
		const daysInMonth = new Date(y, m, 0).getDate();
		const lead = (first.getDay() + 6) % 7; // Monday-first offset
		const cells: { key: string; day: number; inMonth: boolean; isToday: boolean }[] = [];
		const todayKey = dayKey(new Date().toISOString());
		for (let i = 0; i < lead; i++) {
			const d = new Date(y, m - 1, 1 - (lead - i));
			cells.push({ key: dayKey(d.toISOString()), day: d.getDate(), inMonth: false, isToday: dayKey(d.toISOString()) === todayKey });
		}
		for (let d = 1; d <= daysInMonth; d++) {
			const key = `${focus}-${String(d).padStart(2, '0')}`;
			cells.push({ key, day: d, inMonth: true, isToday: key === todayKey });
		}
		const trailing = (7 - (cells.length % 7)) % 7;
		for (let i = 1; i <= trailing; i++) {
			const d = new Date(y, m, i);
			cells.push({ key: dayKey(d.toISOString()), day: d.getDate(), inMonth: false, isToday: dayKey(d.toISOString()) === todayKey });
		}
		const weeks: typeof cells[] = [];
		for (let i = 0; i < cells.length; i += 7) weeks.push(cells.slice(i, i + 7));
		return weeks;
	});

	// ── Week model ──────────────────────────────────────────────────────────
	const weekDays = $derived.by(() => {
		if (currentView !== 'week' || !focus) return [];
		const start = new Date(`${focus}T00:00:00`);
		if (isNaN(start.getTime())) return [];
		const todayKey = dayKey(new Date().toISOString());
		return Array.from({ length: 7 }, (_, i) => {
			const d = new Date(start.getTime() + i * 86400000);
			const pad = (n: number) => String(n).padStart(2, '0');
			const key = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
			return {
				key,
				day: d.getDate(),
				name: d.toLocaleDateString(undefined, { weekday: 'short' }),
				isToday: key === todayKey
			};
		});
	});

	const weekLabel = $derived(
		weekDays.length === 7
			? `${weekDays[0].name} ${weekDays[0].day} – ${weekDays[6].name} ${weekDays[6].day}`
			: ''
	);

	function shiftFocus(dir: -1 | 1) {
		if (currentView === 'month' && focus) {
			const [y, m] = focus.split('-').map(Number);
			const d = new Date(y, m - 1 + dir, 1);
			const next = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`;
			navigate(buildParams({ focus: next }));
		} else if (currentView === 'week' && focus) {
			const d = new Date(`${focus}T00:00:00`);
			d.setDate(d.getDate() + dir * 7);
			const pad = (n: number) => String(n).padStart(2, '0');
			const next = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
			navigate(buildParams({ focus: next }));
		}
	}

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

	const dayNames = $derived(
		Array.from({ length: 7 }, (_, i) =>
			new Date(Date.UTC(2024, 0, 1 + i)).toLocaleDateString(undefined, { weekday: 'short' })
		)
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

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 48rem)/2))">
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
					disabled={subscribeLoading || !data.filters.dest}
					title={data.filters.dest ? undefined : t('calendar.subscribe_needs_dest', 'Select a destination country to subscribe')}
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
	<div class="mx-auto w-full max-w-3xl p-4">
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
			{#if currentView === 'list'}
			<div class="flex min-w-0 gap-2">
				<input
					type="date"
					placeholder={t('calendar.filter_from', 'From')}
					value={fromInput}
					onchange={(e) => { fromInput = e.currentTarget.value; applyFilters(); }}
					class="{inputClass} min-w-0 flex-1"
					aria-label={t('calendar.filter_from', 'From')}
				/>
				<input
					type="date"
					placeholder={t('calendar.filter_to', 'To')}
					value={toInput}
					onchange={(e) => { toInput = e.currentTarget.value; applyFilters(); }}
					class="{inputClass} min-w-0 flex-1"
					aria-label={t('calendar.filter_to', 'To')}
				/>
			</div>
			{/if}
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

		{#if actionError}
			<p class="mb-3 rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
				{actionError}
			</p>
		{/if}

		{#if data.loadError}
			<EmptyState
				icon={CalendarDays}
				title={t('calendar.load_error', 'Could not load calendar')}
				body={t('calendar.load_error_body', 'Please try again later.')}
			/>
		{:else if currentView === 'month'}
			<!-- Month view: real calendar grid for the focused month. -->
			<div class="mb-3 flex items-center justify-between">
				<button
					type="button"
					onclick={() => shiftFocus(-1)}
					class="grid h-8 w-8 place-items-center rounded-[var(--radius)] border border-[var(--color-border)] text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
					aria-label={t('calendar.prev_month', 'Previous month')}
				>
					<ChevronLeft size={16} />
				</button>
				<h2 class="text-base font-semibold text-[var(--color-text)]">{monthLabel}</h2>
				<button
					type="button"
					onclick={() => shiftFocus(1)}
					class="grid h-8 w-8 place-items-center rounded-[var(--radius)] border border-[var(--color-border)] text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
					aria-label={t('calendar.next_month', 'Next month')}
				>
					<ChevronRight size={16} />
				</button>
			</div>
			<div class="overflow-x-auto rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
				<div class="grid min-w-[560px] grid-cols-7 border-b border-[var(--color-border)]">
					{#each dayNames as d (d)}
						<div class="px-2 py-1.5 text-center text-xs font-medium text-[var(--color-muted)]">{d}</div>
					{/each}
				</div>
				{#each monthWeeks as week (week[0].key)}
					<div class="grid min-w-[560px] grid-cols-7 divide-x divide-[var(--color-border)] border-b border-[var(--color-border)] last:border-b-0">
						{#each week as cell (cell.key)}
							{@const dayEntries = entriesByDay.get(cell.key) ?? []}
							<div class="min-h-24 p-1.5 {cell.inMonth ? '' : 'bg-[var(--color-surface-subtle)]/50'}">
								<div class="mb-1 flex h-6 w-6 items-center justify-center rounded-full text-xs {cell.isToday ? 'bg-[var(--color-primary)] font-semibold text-[var(--color-primary-contrast)]' : cell.inMonth ? 'text-[var(--color-text)]' : 'text-[var(--color-muted)]'}">
									{cell.day}
								</div>
								<div class="space-y-0.5">
									{#each dayEntries.slice(0, 3) as entry (entry.id)}
										<a
											href="/calendar/{entry.id}"
											class="block truncate rounded px-1 py-0.5 text-[11px] font-medium transition-colors {isStale(entry) ? 'bg-amber-500/15 text-amber-600 hover:bg-amber-500/25' : 'bg-[var(--color-primary)]/10 text-[var(--color-primary)] hover:bg-[var(--color-primary)]/20'}"
											title={entry.title}
										>
											{entry.title}
										</a>
									{/each}
									{#if dayEntries.length > 3}
										<a
											href="/calendar?view=list&from={cell.key}&to={cell.key}{data.filters.dest ? `&dest=${data.filters.dest}` : ''}{data.filters.category ? `&category=${data.filters.category}` : ''}"
											class="block px-1 text-[11px] text-[var(--color-muted)] hover:text-[var(--color-text)]"
										>
											+{dayEntries.length - 3} {t('calendar.more', 'more')}
										</a>
									{/if}
								</div>
							</div>
						{/each}
					</div>
				{/each}
			</div>
		{:else if currentView === 'week'}
			<!-- Week view: 7-day strip for the focused week. -->
			<div class="mb-3 flex items-center justify-between">
				<button
					type="button"
					onclick={() => shiftFocus(-1)}
					class="grid h-8 w-8 place-items-center rounded-[var(--radius)] border border-[var(--color-border)] text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
					aria-label={t('calendar.prev_week', 'Previous week')}
				>
					<ChevronLeft size={16} />
				</button>
				<h2 class="text-base font-semibold text-[var(--color-text)]">{weekLabel}</h2>
				<button
					type="button"
					onclick={() => shiftFocus(1)}
					class="grid h-8 w-8 place-items-center rounded-[var(--radius)] border border-[var(--color-border)] text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
					aria-label={t('calendar.next_week', 'Next week')}
				>
					<ChevronRight size={16} />
				</button>
			</div>
			<div class="overflow-x-auto rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
				<div class="grid min-w-[560px] grid-cols-7 divide-x divide-[var(--color-border)]">
					{#each weekDays as cell (cell.key)}
						{@const dayEntries = entriesByDay.get(cell.key) ?? []}
						<div class="min-h-32 p-1.5">
							<div class="mb-1 text-center">
								<div class="text-xs text-[var(--color-muted)]">{cell.name}</div>
								<div class="mx-auto flex h-7 w-7 items-center justify-center rounded-full text-sm {cell.isToday ? 'bg-[var(--color-primary)] font-semibold text-[var(--color-primary-contrast)]' : 'text-[var(--color-text)]'}">
									{cell.day}
								</div>
							</div>
							<div class="space-y-1">
								{#each dayEntries as entry (entry.id)}
									<a
										href="/calendar/{entry.id}"
										class="block truncate rounded px-1.5 py-1 text-xs font-medium transition-colors {isStale(entry) ? 'bg-amber-500/15 text-amber-600 hover:bg-amber-500/25' : 'bg-[var(--color-primary)]/10 text-[var(--color-primary)] hover:bg-[var(--color-primary)]/20'}"
										title={entry.title}
									>
										{entry.title}
									</a>
								{/each}
								{#if dayEntries.length === 0}
									<div class="py-2 text-center text-[11px] text-[var(--color-muted)]">—</div>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			</div>
		{:else if allEntries.length === 0}
			<EmptyState
				icon={CalendarDays}
				title={t('calendar.empty', 'No calendar entries')}
				body={t('calendar.empty_body', 'No upcoming dates match your filters.')}
				action={authed
					? { label: t('calendar.submit_event', 'Submit Event'), href: '/calendar/new' }
					: { label: t('calendar.submit_event', 'Submit Event'), href: '/login?redirect=/calendar/new' }}
			/>
		{:else}
			<!-- List view: chronological list -->
			<div class="divide-y divide-[var(--color-border)] rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
				{#each allEntries as entry (entry.id)}
					{@render entryRow(entry)}
				{/each}
			</div>
			{#if hasMore}
				<div class="mt-3 flex justify-center">
					<button
						type="button"
						onclick={loadMore}
						disabled={loadingMore}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					>
						{#if loadingMore}
							<Loader2 size={14} class="animate-spin" />
						{/if}
						{t('calendar.load_more', 'Load more')}
					</button>
				</div>
			{/if}
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
				{#if entry.destination_space_name}
					<span class="inline-flex items-center gap-1">
						{entry.destination_space_name}
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
