<script lang="ts">
	// Ref: japaconnect-engineering/sections/88-ask-an-expat.md
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { MessageSquare, Plus, Circle, CircleSlash, Loader2, Filter } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { i18n } from '$lib/i18n';
	import { relativeTime } from '$lib/feed/time';
	import { EmptyState, LoadingSkeleton } from '$lib/components/ui';
	import type { AMAThread, Space } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let sessions = $state<AMAThread[]>(untrack(() => data.sessions));
	let atEnd = $state(false);
	let loading = $state(false);
	let sentinel = $state<HTMLElement | null>(null);

	// Filter state (BUG-006: dest/profession filter UI).
	let destFilter = $state(page.url.searchParams.get('dest') ?? '');
	let professionFilter = $state(page.url.searchParams.get('profession') ?? '');

	// Build a country ID → name lookup (BUG-007: display country name not raw ID).
	const countryMap = $derived.by(() => {
		const m = new Map<string, string>();
		for (const c of data.countries ?? []) {
			m.set(c.id, c.name);
		}
		return m;
	});

	function countryName(id: string | undefined): string {
		if (!id) return '';
		return countryMap.get(id) ?? id;
	}

	$effect(() => {
		const seed = data.sessions;
		untrack(() => {
			sessions = seed;
			atEnd = seed.length < data.pageSize;
			loading = false;
		});
	});

	const title = $derived(t('ama.list_title', 'Ask an Expat — Live AMAs — JapaConnect'));
	const description = $derived(
		t('ama.list_description', 'Live Ask Me Anything sessions with experienced members of the diaspora.')
	);

	// Gate the "Start an AMA" button on the `forum.ama.create` RBAC node
	// (§26 RBAC wiring, §88). The backend resolves the caller's effective
	// permission set on /me and surfaces it as `permissions`; a custom role
	// granted `forum.ama.create` now sees the button without the frontend
	// hardcoding role names.
	const canStart = $derived(
		!!page.data.user && (page.data.user.permissions?.includes('forum.ama.create') ?? false)
	);

	function cursor(): string | null {
		const last = sessions[sessions.length - 1];
		return last ? last.created_at : null;
	}

	$effect(() => {
		const el = sentinel;
		if (!el || atEnd) return;
		const obs = new IntersectionObserver(
			(entries) => {
				if (entries[0]?.isIntersecting) loadMore();
			},
			{ rootMargin: '600px' }
		);
		obs.observe(el);
		return () => obs.disconnect();
	});

	async function loadMore() {
		if (loading || atEnd) return;
		const c = cursor();
		if (!c) return;
		loading = true;
		const params = new URLSearchParams({
			page_size: String(data.pageSize),
			cursor_created_at: c
		});
		if (destFilter) params.set('dest', destFilter);
		if (professionFilter) params.set('profession', professionFilter);
		const res = await api.get<AMAThread[]>(`/forum/ama?${params.toString()}`);
		if (res.error) {
			loading = false;
			return;
		}
		const batch = res.data ?? [];
		sessions = [...sessions, ...batch];
		atEnd = batch.length < data.pageSize;
		loading = false;
	}

	function applyFilters() {
		const params = new URLSearchParams();
		if (destFilter) params.set('dest', destFilter);
		if (professionFilter) params.set('profession', professionFilter);
		const qs = params.toString();
		goto(`/ama${qs ? `?${qs}` : ''}`, { invalidateAll: true });
	}

	function amaConfig(thread: AMAThread): {
		closes_at?: string;
		host_profession?: string;
		host_dest_country_id?: string;
		host_years_abroad?: number | null;
	} | null {
		return (thread.ama_config as {
			closes_at?: string;
			host_profession?: string;
			host_dest_country_id?: string;
			host_years_abroad?: number | null;
		}) ?? null;
	}

	function isLive(thread: AMAThread): boolean {
		const cfg = amaConfig(thread);
		if (!cfg?.closes_at) return true;
		return new Date(cfg.closes_at).getTime() > Date.now();
	}

	function fetchDestinationCountries(q: string): Promise<AutocompleteItem[]> {
		const allLabel = t('ama.filter_all', 'All countries');
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

<div class="mx-auto w-full max-w-full">
	<!-- Header -->
	<div class="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="text-lg font-semibold text-[var(--color-text)]">
			{t('ama.heading', 'Ask an Expat — Live AMAs')}
		</h1>
		{#if canStart}
			<a
				href="/ama/new"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				<Plus size={14} /> {t('ama.start', 'Start an AMA')}
			</a>
		{/if}
	</div>

	<!-- Filters (BUG-006: dest/profession filter UI per §88 Geo Awareness) -->
	<div class="flex flex-wrap items-end gap-3 border-b border-[var(--color-border)] px-4 py-3">
		<div class="flex flex-col gap-1">
			<label for="dest-filter" class="text-xs font-medium text-[var(--color-text-subtle)]">
				{t('ama.filter_destination', 'Destination')}
			</label>
			<Autocomplete
				placeholder={t('ama.filter_all', 'All countries')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={fetchDestinationCountries}
				initialFetcher={fetchDestinationCountriesInitial}
				selectedId={destFilter || null}
				selectedLabel={selectedDestinationLabel}
				onSelect={(item) => (destFilter = item.id)}
				onClear={() => (destFilter = '')}
			/>
		</div>
		<div class="flex flex-col gap-1">
			<label for="profession-filter" class="text-xs font-medium text-[var(--color-text-subtle)]">
				{t('ama.filter_profession', 'Profession')}
			</label>
			<input
				id="profession-filter"
				type="text"
				value={professionFilter}
				oninput={(e) => (professionFilter = e.currentTarget.value)}
				onkeydown={(e) => { if (e.key === 'Enter') applyFilters(); }}
				placeholder={t('ama.filter_profession_placeholder', 'e.g. Nurse')}
				class="w-40 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-1.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
			/>
		</div>
		<button
			type="button"
			onclick={applyFilters}
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
		>
			<Filter size={14} /> {t('ama.filter_apply', 'Filter')}
		</button>
	</div>

	<!-- List -->
	<div class="p-3">
		{#if data.loadError}
			<EmptyState
				icon={MessageSquare}
				title={t('ama.load_error_title', 'Failed to load AMAs')}
				body={t('ama.load_error_body', 'Please try again later.')}
			/>
		{:else if sessions.length === 0}
			<EmptyState
				icon={MessageSquare}
				title={t('ama.empty_title', 'No AMAs yet')}
				body={t('ama.empty_body', 'Check back soon for live Ask Me Anything sessions with experienced expats.')}
				action={canStart ? { label: t('ama.start', 'Start an AMA'), href: '/ama/new' } : undefined}
			/>
		{:else}
			<div class="space-y-3">
				{#each sessions as session (session.id)}
					{@const cfg = amaConfig(session)}
					{@const live = isLive(session)}
					<a
						href="/ama/{session.id}"
						class={`block rounded-[var(--radius-card)] border p-4 transition-colors hover:border-[var(--color-primary)]/40 ${live ? 'border-[var(--color-primary)]/30 bg-[var(--color-primary)]/[0.03]' : 'border-[var(--color-border)] bg-[var(--color-surface)]'}`}
					>
						<div class="flex flex-wrap items-center gap-2">
							<span class="rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-xs font-semibold text-[var(--color-primary)]">
								{t('ama.badge', 'AMA')}
							</span>
							{#if live}
								<span class="inline-flex items-center gap-0.5 rounded-full bg-green-500/10 px-2 py-0.5 text-xs font-semibold text-green-600">
									<Circle size={10} fill="currentColor" /> {t('ama.live', 'Live')}
								</span>
							{:else}
								<span class="inline-flex items-center gap-0.5 rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs font-semibold text-[var(--color-text-subtle)]">
									<CircleSlash size={11} /> {t('ama.closed', 'Closed')}
								</span>
							{/if}

						</div>

						<h2 class="mt-2 text-base font-semibold text-[var(--color-text)]">{session.title}</h2>

						<div class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-[var(--color-text-subtle)]">
							<div class="flex items-center gap-1.5">
								{#if session.author_avatar}
									<img src={session.author_avatar} alt="" class="h-5 w-5 rounded-full object-cover" />
								{/if}
								<span class="font-medium text-[var(--color-text)]">
									{session.author_name || t('ama.host', 'Host')}
								</span>
							</div>
							{#if cfg?.host_profession}
								<span>{cfg.host_profession}</span>
							{/if}
							{#if cfg?.host_dest_country_id}
								<span>{t('ama.in_country', 'in')} {countryName(cfg.host_dest_country_id)}</span>
							{/if}
							{#if cfg?.host_years_abroad != null}
								<span>{cfg.host_years_abroad} {t('ama.years_abroad', 'yrs abroad')}</span>
							{/if}
							{#if cfg?.closes_at}
								<span class={live ? 'text-green-600' : ''}>
									{live ? t('ama.closes_in', 'closes') : t('ama.closed_at', 'closed')} {relativeTime(cfg.closes_at, new Date(), i18n.locale)}
								</span>
							{/if}
						</div>
					</a>
				{/each}
			</div>

			{#if !atEnd}
				<div bind:this={sentinel} class="h-8" aria-hidden="true"></div>
				{#if loading}<LoadingSkeleton variant="lines" count={2} class="p-4" />{/if}
			{:else}
				<p class="py-6 text-center text-xs text-[var(--color-muted)]">
					{t('feed.end', "You're all caught up")}
				</p>
			{/if}
		{/if}
	</div>
</div>
