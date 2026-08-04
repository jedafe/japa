<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Trophy, Lock, Medal } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { goto } from '$app/navigation';
	import type { LeaderboardEntry } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('leaderboard.title', 'Leaderboard — JapaConnect'));
	const description = $derived(
		t('leaderboard.description', 'Top community members by points.')
	);

	const entries = $derived<LeaderboardEntry[]>(data.entries);
	const disabled = $derived(data.disabled);
	const activeScope = $derived(data.scope);
	const activePeriod = $derived(data.period);

	// Phase 34.05 — scope and period selectors. The backend validates these
	// (global | space:{id}) and (weekly | monthly | all_time).
	const scopeOptions = $derived([
		{ value: 'global', label: t('leaderboard.scope_global', 'Global') }
	]);
	const periodOptions = $derived([
		{ value: 'all_time', label: t('leaderboard.period_all_time', 'All time') },
		{ value: 'monthly', label: t('leaderboard.period_monthly', 'Monthly') },
		{ value: 'weekly', label: t('leaderboard.period_weekly', 'Weekly') }
	]);

	function selectScope(scope: string) {
		const params = new URLSearchParams();
		params.set('scope', scope);
		params.set('period', activePeriod);
		goto(`/leaderboard?${params.toString()}`, { keepFocus: true, noScroll: true });
	}

	function selectPeriod(period: string) {
		const params = new URLSearchParams();
		params.set('scope', activeScope);
		params.set('period', period);
		goto(`/leaderboard?${params.toString()}`, { keepFocus: true, noScroll: true });
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

<div class="mx-auto w-full max-w-2xl">
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Trophy size={20} /> {t('leaderboard.heading', 'Leaderboard')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">{description}</p>
	</div>

	{#if disabled}
		<div class="p-4">
			<EmptyState
				icon={Lock}
				title={t('tools.disabled_title', 'This tool is currently unavailable')}
				body={t('tools.disabled_body', 'Check back soon or contact support.')}
			/>
		</div>
	{:else}
		<!-- Phase 34.05 — scope and period selectors -->
		<div class="flex flex-wrap items-center justify-between gap-2 border-b border-[var(--color-border)] px-4 py-2">
			<div class="flex gap-1">
				{#each scopeOptions as opt (opt.value)}
					<button
						type="button"
						onclick={() => selectScope(opt.value)}
						class="rounded-full px-3 py-1 text-xs font-medium transition-colors {activeScope === opt.value ? 'bg-[var(--color-primary)] text-white' : 'border border-[var(--color-border)] text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]'}"
					>
						{opt.label}
					</button>
				{/each}
			</div>
			<div class="flex gap-1">
				{#each periodOptions as opt (opt.value)}
					<button
						type="button"
						onclick={() => selectPeriod(opt.value)}
						class="rounded-full px-3 py-1 text-xs font-medium transition-colors {activePeriod === opt.value ? 'bg-[var(--color-primary)] text-white' : 'border border-[var(--color-border)] text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]'}"
					>
						{opt.label}
					</button>
				{/each}
			</div>
		</div>

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
										<a href="/u/{entry.username}" class="text-[var(--color-text)] hover:underline">
											@{entry.username}
										</a>
									</td>
									<td class="px-3 py-2 text-right font-medium text-[var(--color-text)]">
										{entry.points}
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
