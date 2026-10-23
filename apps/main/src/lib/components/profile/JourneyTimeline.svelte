<script lang="ts">
	// §87 — Japa journey milestone timeline for the profile Overview tab.
	// Ordered stage list derived from the member's public japa stage:
	// earlier stages render as completed, the current stage in progress,
	// later stages upcoming. Only the current stage carries a date — the
	// API exposes a single `updated_at`, so past milestones have no date.
	import { Check, CircleDot, CircleDashed } from 'lucide-svelte';
	import { t, i18n } from '$lib/i18n';
	import { journeySteps } from './journey';
	import type { PublicJapaStage } from '$lib/types';

	let { japaStage }: { japaStage: PublicJapaStage | null } = $props();

	const steps = $derived(journeySteps(japaStage));

	const STATUS_KEY = {
		done: 'profile.journey.done',
		current: 'profile.journey.current',
		next: 'profile.journey.next'
	} as const;

	function chipClass(status: 'done' | 'current' | 'next'): string {
		switch (status) {
			case 'done':
				return 'bg-[var(--color-primary)] text-white';
			case 'current':
				return 'bg-[var(--color-primary)]/10 text-[var(--color-primary)]';
			default:
				return 'bg-[var(--color-surface-subtle)] text-[var(--color-muted)]';
		}
	}

	const updatedDate = $derived(
		japaStage?.updated_at
			? new Date(japaStage.updated_at).toLocaleDateString(i18n.locale, { year: 'numeric', month: 'short' })
			: ''
	);
</script>

{#if steps.length > 0}
	<ol class="relative">
		{#each steps as step, i (step.stage)}
			{@const isLast = i === steps.length - 1}
			<li class="relative flex gap-3 pb-5 {isLast ? 'pb-0' : ''}">
				{#if !isLast}
					<span
						class="absolute start-[15px] top-8 bottom-0 w-px {step.status === 'done'
							? 'bg-[var(--color-primary)]'
							: 'bg-[var(--color-border)]'}"
					></span>
				{/if}
				<span
					class="relative z-10 grid h-8 w-8 shrink-0 place-items-center rounded-full {step.status ===
					'done'
						? 'bg-[var(--color-primary)] text-white'
						: step.status === 'current'
							? 'bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
							: 'bg-[var(--color-surface-subtle)] text-[var(--color-muted)]'}"
				>
					{#if step.status === 'done'}
						<Check size={14} />
					{:else if step.status === 'current'}
						<CircleDot size={14} />
					{:else}
						<CircleDashed size={14} />
					{/if}
				</span>
				<div class="min-w-0 flex-1">
					<div class="flex items-center justify-between gap-2">
						<p class="text-sm font-medium {step.status === 'next' ? 'text-[var(--color-muted)]' : 'text-[var(--color-text)]'}">
							{t(`japa_stage.${step.stage}`, step.stage)}
						</p>
						<span class="shrink-0 text-xs text-[var(--color-muted)]">
							{#if step.status === 'current' && updatedDate}
								{updatedDate}
							{:else if step.status === 'next'}
								{t('profile.journey.tbd', 'TBD')}
							{/if}
						</span>
					</div>
					<span class="mt-1 inline-flex rounded-full px-2 py-0.5 text-[10px] font-medium {chipClass(step.status)}">
						{t(STATUS_KEY[step.status], step.status)}
					</span>
				</div>
			</li>
		{/each}
	</ol>
{/if}
