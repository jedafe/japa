<script lang="ts">
	// Ref: japaconnect-engineering/sections/86-community-calendar.md
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
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
	import type { CalendarEntry, CalendarCategory } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('calendar.list_title', 'Community Calendar — JapaConnect'));
	const description = $derived(
		t(
			'calendar.list_description',
			'Verified dates for visa windows, embassy closures, deadlines, and exams.'
		)
	);

	// Schema-defined category enum (Rule 16 — no hardcoded data arrays).
	const categories: { value: CalendarCategory; label: string }[] = [
		{ value: 'visa_window', label: t('calendar.cat_visa_window', 'Visa window') },
		{ value: 'embassy_closure', label: t('calendar.cat_embassy_closure', 'Embassy closure') },
		{ value: 'application_deadline', label: t('calendar.cat_application_deadline', 'Application deadline') },
		{ value: 'exam_window', label: t('calendar.cat_exam_window', 'Exam window') },
		{ value: 'immigration_office', label: t('calendar.cat_immigration_office', 'Immigration office') },
		{ value: 'other', label: t('calendar.cat_other', 'Other') }
	];

	const views = [
		{ value: 'list', label: t('calendar.view_list', 'List') },
		{ value: 'month', label: t('calendar.view_month', 'Month') },
		{ value: 'week', label: t('calendar.view_week', 'Week') }
	];

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

	function applyFilters(e: SubmitEvent) {
		e.preventDefault();
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
		{@html `<script type="application/ld+json">${jsonLd}</script>`}
	{/if}
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<!-- Header -->
	<div class="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
		<div class="min-w-0">
			<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
				<CalendarDays size={20} /> {t('calendar.heading', 'Community Calendar')}
			</h1>
			<p class="mt-1 text-sm text-[var(--color-muted)]">{description}</p>
		</div>
		<div class="flex shrink-0 items-center gap-2">
			{#if authed}
				<button
					type="button"
					onclick={toggleSubscribe}
					disabled={subscribeLoading}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
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
				<a
					href="/calendar/new"
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
				>
					<Plus size={14} /> {t('calendar.submit_event', 'Submit Event')}
				</a>
			{:else}
				<a
					href="/login?redirect=/calendar/new"
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
				>
					<Plus size={14} /> {t('calendar.submit_event', 'Submit Event')}
				</a>
			{/if}
		</div>
	</div>

	<!-- View toggle -->
	<div class="flex items-center gap-1 border-b border-[var(--color-border)] px-4 py-2">
		<div class="flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] p-0.5">
			{#each views as v (v.value)}
				<button
					type="button"
					onclick={() => setView(v.value)}
					class="rounded-[calc(var(--radius)-2px)] px-3 py-1 text-xs font-medium transition-colors {currentView ===
					v.value
						? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)]'
						: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
					aria-pressed={currentView === v.value}
				>
					{v.label}
				</button>
			{/each}
		</div>
	</div>

	<!-- Filter bar -->
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<form onsubmit={applyFilters} class="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
			<select
				value={destInput}
				onchange={(e) => (destInput = e.currentTarget.value)}
				class={inputClass}
				aria-label={t('calendar.filter_dest', 'Destination country')}
			>
				<option value="">{t('calendar.filter_dest_all', 'All countries')}</option>
				{#each data.countries as c (c.id)}
					<option value={c.id}>{c.name}</option>
				{/each}
			</select>
			<select
				value={categoryInput}
				onchange={(e) => (categoryInput = e.currentTarget.value)}
				class={inputClass}
			>
				<option value="">{t('calendar.filter_category', 'All categories')}</option>
				{#each categories as cat (cat.value)}
					<option value={cat.value}>{cat.label}</option>
				{/each}
			</select>
			<div class="flex gap-2">
				<input
					type="date"
					placeholder={t('calendar.filter_from', 'From')}
					value={fromInput}
					oninput={(e) => (fromInput = e.currentTarget.value)}
					class={inputClass}
					aria-label={t('calendar.filter_from', 'From')}
				/>
				<input
					type="date"
					placeholder={t('calendar.filter_to', 'To')}
					value={toInput}
					oninput={(e) => (toInput = e.currentTarget.value)}
					class={inputClass}
					aria-label={t('calendar.filter_to', 'To')}
				/>
			</div>
			<button
				type="submit"
				class="shrink-0 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:opacity-90"
			>
				{t('common.filter', 'Filter')}
			</button>
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

	<!-- Content -->
	<div class="p-4">
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
