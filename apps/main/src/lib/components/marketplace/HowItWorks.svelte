<script lang="ts">
	// "How it works" — numbered step rail from the listing's steps[] set
	// (§114; authored via the service create/edit repeaters). Rendered only
	// when the listing carries steps — callers gate on steps.length.
	// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md (§3.3)
	import { t } from '$lib/i18n';
	import type { ServiceStep } from '$lib/types';

	interface Props {
		steps: ServiceStep[];
	}
	let { steps }: Props = $props();
</script>

<section aria-labelledby="howitworks-heading">
	<h2
		id="howitworks-heading"
		class="text-sm font-semibold uppercase tracking-wide text-[var(--color-text-subtle)]"
	>
		{t('services.steps_heading', 'How it works')}
	</h2>
	<ol class="mt-4 space-y-4">
		{#each steps as step, i (i)}
			<li class="flex items-start gap-3">
				<span
					class="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/10 text-sm font-semibold text-[var(--color-primary)]"
					aria-hidden="true"
				>
					{i + 1}
				</span>
				<div class="min-w-0">
					<p class="text-sm font-semibold text-[var(--color-text)]">{step.title}</p>
					{#if step.description}
						<p class="mt-1 text-sm leading-relaxed text-[var(--color-text-subtle)]">
							{step.description}
						</p>
					{/if}
				</div>
			</li>
		{/each}
	</ol>
</section>
