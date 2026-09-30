<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Trophy, Lock, Calendar, Gift, Award, CheckCircle2, Target } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { RelativeTime } from '$lib/components/ui';
	import type { Challenge } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('challenges.title', 'Challenges — JapaConnect'));
	const description = $derived(
		t('challenges.description', 'Compete in community challenges and earn rewards.')
	);

	const challenges = $derived<Challenge[]>(data.challenges);
	const disabled = $derived(data.disabled);
	const authed = $derived(data.authed);
	const myProgress = $derived<Record<string, { target_count: number; current_count: number; completed: boolean }>>(
		Object.fromEntries((data.myProgress ?? []).map((p) => [p.id, p]))
	);

	// Badge id → name map so a reward_badge_id renders as the badge's name.
	const badgeNames = $derived<Record<string, string>>(
		Object.fromEntries((data.badges ?? []).map((b) => [b.id, b.name]))
	);

	function rewardBadgeName(ch: Challenge): string {
		return (ch.reward_badge_id && badgeNames[ch.reward_badge_id]) || t('challenges.reward_badge', 'Badge reward');
	}

	// Progress percentage for a challenge — 0 if no progress data yet.
	function progressPct(ch: Challenge): number {
		const p = myProgress[ch.id];
		if (!p || p.target_count === 0) return 0;
		return Math.min(100, Math.round((p.current_count / p.target_count) * 100));
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Trophy size={20} /> {t('challenges.heading', 'Challenges')}
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
	{:else if challenges.length === 0}
		<div class="p-4">
			<EmptyState
				icon={Trophy}
				title={t('challenges.empty', 'No active challenges')}
				body={t('challenges.empty_body', 'Check back soon for new challenges.')}
			/>
		</div>
	{:else}
		<div class="p-4">
			<div class="space-y-3">
				{#each challenges as ch (ch.id)}
					{@const prog = myProgress[ch.id]}
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-start justify-between gap-3">
							<div class="min-w-0 flex-1">
								<div class="flex items-center gap-2">
									<h3 class="text-sm font-medium text-[var(--color-text)]">{ch.name}</h3>
									{#if prog?.completed}
										<CheckCircle2 size={14} class="shrink-0 text-[var(--color-success)]" />
									{/if}
								</div>
								{#if ch.description}
									<p class="mt-1 text-xs text-[var(--color-muted)]">{ch.description}</p>
								{/if}
								<div class="mt-2 flex flex-wrap gap-3 text-xs text-[var(--color-muted)]">
									<span class="flex items-center gap-1">
										<Calendar size={12} />
										<RelativeTime iso={ch.starts_at} />
										{#if ch.ends_at}
											→ <RelativeTime iso={ch.ends_at} />
										{:else}
											→ {t('challenges.ongoing', 'Ongoing')}
										{/if}
									</span>
									{#if ch.target_count}
										<span class="flex items-center gap-1">
											<Target size={12} />
											{t('challenges.goal', 'Goal: {n}').replace('{n}', String(ch.target_count))}
										</span>
									{/if}
								</div>
							</div>
							<div class="flex shrink-0 flex-col items-end gap-1">
								{#if ch.reward_points > 0}
									<span class="flex items-center gap-1 rounded-full bg-[var(--color-surface-subtle)] px-2.5 py-1 text-xs font-medium text-[var(--color-text)]">
										<Gift size={12} class="text-[var(--color-primary)]" />
										{ch.reward_points} {t('gamification.points', 'pts')}
									</span>
								{/if}
								{#if ch.reward_badge_id}
									<span class="flex items-center gap-1 rounded-full bg-[var(--color-surface-subtle)] px-2.5 py-1 text-xs font-medium text-[var(--color-text)]">
										<Award size={12} class="text-[var(--color-primary)]" />
										{rewardBadgeName(ch)}
									</span>
								{/if}
							</div>
						</div>

						<!-- Phase 34.04 — progress bar for authenticated users -->
						{#if authed && prog}
							<div class="mt-3">
								<div class="flex items-center justify-between text-xs text-[var(--color-muted)]">
									<span>{t('challenges.progress', 'Progress')}</span>
									<span>{prog.current_count} / {prog.target_count} ({progressPct(ch)}%)</span>
								</div>
								<div class="mt-1 h-2 overflow-hidden rounded-full bg-[var(--color-surface-subtle)]">
									<div
										class="h-full transition-all {prog.completed ? 'bg-[var(--color-success)]' : 'bg-[var(--color-primary)]'}"
										style="width: {progressPct(ch)}%"
									></div>
								</div>
							</div>
						{:else if authed}
							<div class="mt-3">
								<p class="text-xs text-[var(--color-muted)]">{t('challenges.not_joined', 'Not started yet')}</p>
							</div>
						{/if}
					</div>
				{/each}
			</div>
		</div>
	{/if}
</div>
