<script lang="ts">
	import { untrack } from 'svelte';
	// Ref: japaconnect-engineering/sections/87-japa-stage-profiles.md
	// Japa Stage directory. SSR list of public member profiles from
	// GET /members/by-stage with ?stage filter and pagination.
	import { t } from '$lib/i18n';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import PaginationControls from '$lib/components/ui/PaginationControls.svelte';
	import RelativeTime from '$lib/components/ui/RelativeTime.svelte';
	import { Compass, Filter, Users } from 'lucide-svelte';
	import type { JapaStage, MemberStage } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// The 7-stage progression (schema-defined enum — Rule 16: not hardcoded
	// data, just the enum vocabulary used for the legend/stepper labels).
	const STAGES: JapaStage[] = [
		'considering',
		'planning',
		'preparing',
		'applied',
		'approved',
		'relocated',
		'settled'
	];

	// Per-stage badge colours (CSS variables only).
	const STAGE_COLORS: Record<JapaStage, string> = {
		considering: 'var(--color-muted)',
		planning: '#3b82f6',
		preparing: '#8b5cf6',
		applied: '#f59e0b',
		approved: '#10b981',
		relocated: '#06b6d4',
		settled: 'var(--color-primary)'
	};

	function stageLabel(s: JapaStage): string {
		return t(`japa_stage.${s}`, s.charAt(0).toUpperCase() + s.slice(1));
	}

	// Local filter state, seeded from SSR-passed filters.
	let stageFilter = $state(untrack(() => data.filters.stage));
	let destFilter = $state(untrack(() => data.filters.dest));

	// Keep local filter state in sync when navigating between pages/filters.
	$effect(() => {
		stageFilter = data.filters.stage;
		destFilter = data.filters.dest;
	});

	function applyFilters() {
		const params = new URLSearchParams();
		if (stageFilter) params.set('stage', stageFilter);
		if (destFilter) params.set('dest', destFilter);
		goto(`/japa-stage${params.toString() ? `?${params.toString()}` : ''}`);
	}

	function clearFilters() {
		goto('/japa-stage');
	}

	function changePage(p: number) {
		const params = new URLSearchParams();
		if (data.filters.stage) params.set('stage', data.filters.stage);
		if (data.filters.dest) params.set('dest', data.filters.dest);
		params.set('page', String(p));
		goto(`/japa-stage?${params.toString()}`);
	}

	const title = $derived(t('japa_stage.directory_title', 'Japa Stage — Find Members by Stage'));
	const description = $derived(
		t(
			'japa_stage.directory_description',
			'Discover Nigerian diaspora members by where they are in their japa journey.'
		)
	);

	const hasFilters = $derived(!!data.filters.stage || !!data.filters.dest);
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
		<div class="flex items-center gap-2">
			<Compass size={18} class="text-[var(--color-primary)]" />
			<h1 class="text-lg font-semibold text-[var(--color-text)]">
				{t('japa_stage.heading', 'Japa Stage — Find Members by Stage')}
			</h1>
		</div>
		{#if data.authed}
			<a
				href="/japa-stage/me"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				{t('japa_stage.my_stage', 'My Stage')}
			</a>
		{:else}
			<a
				href="/login?redirect=/japa-stage/me"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
			>
				{t('japa_stage.set_my_stage', 'Set my stage')}
			</a>
		{/if}
	</div>

	<div class="p-3">
		<!-- Stage legend / progression stepper -->
		<div class="mb-4 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
			<p class="mb-2 text-xs font-medium uppercase tracking-wide text-[var(--color-text-subtle)]">
				{t('japa_stage.progression', 'Stage progression')}
			</p>
			<ol class="flex flex-wrap items-center gap-1">
				{#each STAGES as s, i (s)}
					<li class="flex items-center gap-1">
						<a
							href={`/japa-stage?stage=${s}`}
							class="inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-medium transition-colors hover:bg-[var(--color-surface-subtle)]"
							style={`border-color: ${STAGE_COLORS[s]}40; color: ${STAGE_COLORS[s]};`}
						>
							<span
								class="inline-block h-2 w-2 rounded-full"
								style={`background-color: ${STAGE_COLORS[s]};`}
							></span>
							{stageLabel(s)}
						</a>
						{#if i < STAGES.length - 1}
							<span class="text-[var(--color-muted)]" aria-hidden="true">→</span>
						{/if}
					</li>
				{/each}
			</ol>
		</div>

		<!-- Filter bar -->
		<div class="mb-4 flex flex-wrap items-end gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
			<div class="flex flex-col gap-1">
				<label for="filter-stage" class="text-xs font-medium text-[var(--color-text-subtle)]">
					{t('japa_stage.filter_stage', 'Stage')}
				</label>
				<select
					id="filter-stage"
					bind:value={stageFilter}
					class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
				>
					<option value="">{t('japa_stage.all_stages', 'All stages')}</option>
					{#each STAGES as s (s)}
						<option value={s}>{stageLabel(s)}</option>
					{/each}
				</select>
			</div>
			<div class="flex flex-col gap-1">
				<label for="filter-dest" class="text-xs font-medium text-[var(--color-text-subtle)]">
					{t('japa_stage.filter_dest', 'Destination')}
				</label>
				<select
					id="filter-dest"
					bind:value={destFilter}
					class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
				>
					<option value="">{t('japa_stage.all_destinations', 'All destinations')}</option>
					{#each data.countries as c (c.id)}
						<option value={c.id}>{c.name}</option>
					{/each}
				</select>
			</div>
			<button
				type="button"
				onclick={applyFilters}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				<Filter size={14} /> {t('common.apply', 'Apply')}
			</button>
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
		<a
			href="/u/{m.username}"
			class="flex items-start gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-colors hover:bg-[var(--color-surface-subtle)]"
		>
			<img
				src={m.avatar_url ?? `https://api.dicebear.com/7.x/initials/svg?seed=${m.username}`}
				alt={m.username}
				class="h-10 w-10 shrink-0 rounded-full object-cover"
				loading="lazy"
			/>
			<div class="min-w-0 flex-1">
				<p class="truncate text-sm font-semibold text-[var(--color-text)]">@{m.username}</p>
				<span
					class="mt-1 inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium"
					style={`background-color: ${STAGE_COLORS[m.stage as JapaStage]}1a; color: ${STAGE_COLORS[m.stage as JapaStage]};`}
				>
					<span
						class="inline-block h-1.5 w-1.5 rounded-full"
						style={`background-color: ${STAGE_COLORS[m.stage as JapaStage]};`}
					></span>
					{stageLabel(m.stage as JapaStage)}
				</span>
				<div class="mt-1.5 flex flex-col gap-0.5 text-xs text-[var(--color-text-subtle)]">
					{#if m.destination_space_name}
						<span>{t('japa_stage.dest', 'Destination')}: {m.destination_space_name}</span>
					{/if}
					{#if m.updated_at}
						<span>{t('japa_stage.updated', 'Updated')} <RelativeTime iso={m.updated_at} /></span>
					{/if}
				</div>
			</div>
		</a>
	</li>
{/snippet}
