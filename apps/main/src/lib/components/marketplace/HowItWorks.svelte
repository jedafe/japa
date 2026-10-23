<script lang="ts">
	// "How it works" — the mockup's horizontal step rail: a numbered badge
	// + icon circle per step with a dashed connector line between them
	// (§114 2026-10-10). steps[].icon is an optional kebab-case Lucide
	// name resolved through the whitelist; absent icons get the fallback.
	// Rendered only when the listing carries steps — callers gate on
	// steps.length.
	import { marketplaceIcon } from './marketplaceIcon';
	import { t } from '$lib/i18n';
	import type { ServiceStep } from '$lib/types';

	interface Props {
		steps: ServiceStep[];
	}
	let { steps }: Props = $props();
</script>

<section aria-labelledby="howitworks-heading">
	<h2 id="howitworks-heading" class="text-xl font-bold text-[var(--color-text)]">
		{t('services.steps_heading', 'How it works')}
	</h2>
	<ol class="mt-6 grid gap-8 sm:grid-cols-2 lg:flex lg:items-start lg:gap-4">
		{#each steps as step, i (i)}
			{@const Icon = marketplaceIcon(step.icon)}
			<li class="relative flex min-w-0 flex-1 flex-col items-center text-center">
				{#if i > 0}
					<!-- dashed connector — spans the gap to the previous step's
					     icon on lg+, hidden while the rail stacks. -->
					<span
						class="absolute end-[calc(50%+2.75rem)] top-8 hidden w-[calc(100%-5.5rem)] border-t-2 border-dashed border-[var(--color-border)] lg:block"
						aria-hidden="true"
					></span>
				{/if}
				<span
					class="relative grid h-16 w-16 place-items-center rounded-full bg-[var(--color-primary)]/5 ring-1 ring-[var(--color-primary)]/15"
				>
					<Icon size={22} class="text-[var(--color-primary)]" aria-hidden="true" />
					<span
						class="absolute -bottom-1 grid h-6 w-6 place-items-center rounded-full bg-[var(--color-primary)] text-[11px] font-bold text-[var(--color-primary-contrast)]"
						aria-hidden="true"
					>
						{i + 1}
					</span>
				</span>
				<p class="mt-4 text-sm font-semibold text-[var(--color-text)]">{step.title}</p>
				{#if step.description}
					<p class="mt-1 max-w-56 text-xs leading-relaxed text-[var(--color-muted)]">
						{step.description}
					</p>
				{/if}
			</li>
		{/each}
	</ol>
</section>
