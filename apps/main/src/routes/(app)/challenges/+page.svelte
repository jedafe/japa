<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Trophy, Lock, Calendar, Gift } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { RelativeTime } from '$lib/components/ui';
	import type { Challenge } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('challenges.title', 'Challenges — JapaConnect'));
	const description = $derived(
		t('challenges.description', 'Compete in community challenges and earn rewards.')
	);

	const challenges = $state<Challenge[]>(data.challenges);
	const disabled = $state(data.disabled);
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
	<div class="border-b border-[var(--color-border)] px-4 py-3">
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
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-start justify-between gap-3">
							<div class="min-w-0 flex-1">
								<h3 class="text-sm font-medium text-[var(--color-text)]">{ch.name}</h3>
								{#if ch.description}
									<p class="mt-1 text-xs text-[var(--color-muted)]">{ch.description}</p>
								{/if}
								<div class="mt-2 flex flex-wrap gap-3 text-xs text-[var(--color-muted)]">
									<span class="flex items-center gap-1">
										<Calendar size={12} />
										<RelativeTime iso={ch.start_date} /> → <RelativeTime iso={ch.end_date} />
									</span>
								</div>
							</div>
							<span class="flex shrink-0 items-center gap-1 rounded-full bg-[var(--color-surface-subtle)] px-2.5 py-1 text-xs font-medium text-[var(--color-text)]">
								<Gift size={12} class="text-[var(--color-primary)]" />
								{ch.reward_points} {t('gamification.points', 'pts')}
							</span>
						</div>
					</div>
				{/each}
			</div>
		</div>
	{/if}
</div>
