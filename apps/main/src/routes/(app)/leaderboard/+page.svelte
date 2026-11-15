<script lang="ts">
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import { t } from '#lib/i18n/index.js';
	import UserHoverCard from '#lib/components/ui/UserHoverCard.svelte';
	import VerifiedBadge from '#lib/components/ui/VerifiedBadge.svelte';
	import { page } from '$app/state';
	import { Trophy, Lock, Medal, Filter } from 'lucide-svelte';
	import { EmptyState, ListPageBar, Modal, SelectField, UrlTabs, type UrlTab } from '#lib/components/ui/index.js';
	import { goto } from '$app/navigation';
	import type { LeaderboardEntry } from '#lib/types/index.js';
	import type { PageData } from './$types';

import { ogCardUrl } from '#lib/seo/og.js';
	let { data }: { data: PageData } = $props();

	const title = $derived(t('leaderboard.title', 'Leaderboard — {site}'));
	const description = $derived(
		t('leaderboard.description', 'Top community members by points.')
	);

	const entries = $derived<LeaderboardEntry[]>(data.entries);
	const disabled = $derived(data.disabled);
	const activeScope = $derived(data.scope);
	const activePeriod = $derived(data.period);
	const hasActiveFilters = $derived(activeScope !== 'global');
	let filtersModalOpen = $state(false);

	// Phase 34.05 — scope and period selectors. The backend validates these
	// (global | space:{uuid}) and (weekly | monthly | all_time).
	const periodTabs = $derived<UrlTab[]>([
		{ value: 'all_time', label: t('leaderboard.period_all_time', 'All time') },
		{ value: 'monthly', label: t('leaderboard.period_monthly', 'Monthly') },
		{ value: 'weekly', label: t('leaderboard.period_weekly', 'Weekly') }
	]);

	// Per-space scope options: Global plus every space the member belongs to.
	const scopeOptions = $derived([
		{ value: 'global', label: t('leaderboard.scope_global', 'Global') },
		...(data.spaces ?? []).map((s) => ({ value: `space:${s.id}`, label: s.name }))
	]);

	function navigate(scope: string, period: string) {
		const params = new URLSearchParams();
		params.set('scope', scope);
		params.set('period', period);
		goto(`/leaderboard?${params.toString()}`, { reset: false });
	}

	function selectPeriod(period: string) {
		navigate(activeScope, period);
	}

	function selectScope(scope: string) {
		if (scope) navigate(scope, activePeriod);
	}

	const medalColor = (rank: number): string => {
		if (rank === 1) return 'text-[var(--color-warning)]';
		if (rank === 2) return 'text-[var(--color-muted)]';
		if (rank === 3) return 'text-[var(--color-info)]';
		return 'text-[var(--color-muted)]';
	};
</script>

<SeoHead
	ogImage={ogCardUrl(page.url.origin, 'leaderboard')} title={title} description={description} ogTitle={title} />

<div class="mx-auto w-full max-w-2xl" style="--page-gutter: 1rem">
	<h1 class="sr-only">{t('leaderboard.heading', 'Leaderboard')}</h1>
	<ListPageBar>
		{#if !disabled}
			<UrlTabs
				tabs={periodTabs}
				value={activePeriod}
				onValueChange={(v) => selectPeriod(v)}
				class="!border-b-0 !bg-transparent"
			/>
		{:else}
			<span class="flex items-center px-4 text-sm font-semibold text-[var(--color-text)]" style="min-height: var(--nav-height)">
				{t('leaderboard.heading', 'Leaderboard')}
			</span>
		{/if}
		{#snippet actions()}
			{#if !disabled && scopeOptions.length > 1}
				<button
					type="button"
					onclick={() => (filtersModalOpen = true)}
					class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					{t('common.filters', 'Filters')}
					{#if hasActiveFilters}
						<span class="ms-0.5 h-2 w-2 rounded-full bg-[var(--color-primary)]" aria-hidden="true"></span>
					{/if}
					<Filter size={16} />
				</button>
			{/if}
		{/snippet}
	</ListPageBar>

	{#if !disabled && scopeOptions.length > 1}
		<Modal bind:open={filtersModalOpen} title={t('common.filters', 'Filters')} icon={Filter} size="lg">
			{#snippet children()}
				<SelectField
					value={activeScope}
					options={scopeOptions}
					ariaLabel={t('leaderboard.scope_label', 'Scope')}
					onValueChange={selectScope}
				/>
			{/snippet}
		</Modal>
	{/if}

	{#if disabled}
		<div class="p-4">
			<EmptyState
				icon={Lock}
				title={t('tools.disabled_title', 'This tool is currently unavailable')}
				body={t('tools.disabled_body', 'Check back soon or contact support.')}
			/>
		</div>
	{:else}
		{#if entries.length === 0}
			<div class="p-4">
				<EmptyState icon={Trophy} title={t('leaderboard.empty', 'No entries yet')} />
			</div>
		{:else}
			<div class="p-4">
				<div class="overflow-x-auto rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
					<table class="w-full text-sm">
						<thead>
							<tr class="border-b border-[var(--color-border)] text-start text-xs text-[var(--color-muted)]">
								<th class="px-3 py-2 font-medium">{t('leaderboard.rank', 'Rank')}</th>
								<th class="px-3 py-2 font-medium">{t('leaderboard.user', 'User')}</th>
								<th class="px-3 py-2 text-end font-medium">{t('leaderboard.points', 'Points')}</th>
							</tr>
						</thead>
						<tbody>
							{#each entries as entry (entry.user_id)}
								<tr class="border-b border-[var(--color-border)] last:border-0">
									<td class="px-3 py-2">
										<span class="flex items-center gap-1.5 {medalColor(entry.rank)}">
											{#if entry.rank <= 3}
												<Medal size={14} />
											{/if}
											<span class="font-medium">{entry.rank}</span>
										</span>
									</td>
									<td class="px-3 py-2">
										<UserHoverCard
											id={entry.user_id}
											slug={entry.username}
											name={entry.display_name ?? entry.username}
											avatar={entry.avatar_url}
											verified={entry.verified}
											verificationType={entry.verification_type}
											class="inline-flex w-fit min-w-0 max-w-full items-center gap-2 text-start text-[var(--color-text)] hover:underline"
										>
											{#snippet trigger()}
												{#if entry.avatar_url}
													<img
														src={entry.avatar_url}
														alt=""
														class="h-6 w-6 shrink-0 rounded-full object-cover"
													/>
												{/if}
												<span class="min-w-0 truncate">
													{#if entry.display_name}
														<span class="inline-flex items-center gap-1 font-medium">{entry.display_name}<VerifiedBadge verified={entry.verified} type={entry.verification_type} scope="user" size={13} /></span>
														<span class="text-[var(--color-muted)]">@{entry.username}</span>
													{:else}
														@{entry.username}
													{/if}
												</span>
											{/snippet}
										</UserHoverCard>
									</td>
									<td class="px-3 py-2 text-end font-medium text-[var(--color-text)]">
										{entry.points.toLocaleString()}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</div>
		{/if}
	{/if}
</div>
