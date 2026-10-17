<script lang="ts">
	import { t } from '$lib/i18n';
	import UserHoverCard from '$lib/components/ui/UserHoverCard.svelte';
	import { page } from '$app/state';
	import { Trophy, Lock, Medal } from 'lucide-svelte';
	import { EmptyState, ListPageBar, SelectField, UrlTabs, type UrlTab } from '$lib/components/ui';
	import { goto } from '$app/navigation';
	import type { LeaderboardEntry } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('leaderboard.title', 'Leaderboard — {site}'));
	const description = $derived(
		t('leaderboard.description', 'Top community members by points.')
	);

	const entries = $derived<LeaderboardEntry[]>(data.entries);
	const disabled = $derived(data.disabled);
	const activeScope = $derived(data.scope);
	const activePeriod = $derived(data.period);

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
		goto(`/leaderboard?${params.toString()}`, { keepFocus: true, noScroll: true });
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

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

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
				<SelectField
					value={activeScope}
					options={scopeOptions}
					onValueChange={selectScope}
					class="w-40 !py-1.5 text-xs"
				/>
			{/if}
		{/snippet}
	</ListPageBar>

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
				<div class="overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
					<table class="w-full text-sm">
						<thead>
							<tr class="border-b border-[var(--color-border)] text-left text-xs text-[var(--color-muted)]">
								<th class="px-3 py-2 font-medium">{t('leaderboard.rank', 'Rank')}</th>
								<th class="px-3 py-2 font-medium">{t('leaderboard.user', 'User')}</th>
								<th class="px-3 py-2 text-right font-medium">{t('leaderboard.points', 'Points')}</th>
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
											class="inline-flex w-fit min-w-0 max-w-full items-center gap-2 text-left text-[var(--color-text)] hover:underline"
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
														<span class="font-medium">{entry.display_name}</span>
														<span class="text-[var(--color-muted)]">@{entry.username}</span>
													{:else}
														@{entry.username}
													{/if}
												</span>
											{/snippet}
										</UserHoverCard>
									</td>
									<td class="px-3 py-2 text-right font-medium text-[var(--color-text)]">
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
