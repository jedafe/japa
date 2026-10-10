<script lang="ts">
	// Hero trust chips — the listing's selling_points[] rendered as a row
	// of icon + title (+ optional description) chips under the rating
	// (§114 2026-10-10 mockup hero). Icons resolve through the fixed
	// marketplaceIcon whitelist.
	import { marketplaceIcon } from './marketplaceIcon';
	import type { ServiceIconEntry } from '$lib/types';

	interface Props {
		points: ServiceIconEntry[];
	}
	let { points }: Props = $props();
</script>

{#if points.length > 0}
	<ul class="flex flex-wrap items-stretch gap-x-8 gap-y-4">
		{#each points as point, i (i)}
			{@const Icon = marketplaceIcon(point.icon)}
			<li class="flex items-start gap-3">
				<span
					class="grid h-9 w-9 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/10"
				>
					<Icon size={16} class="text-[var(--color-primary)]" aria-hidden="true" />
				</span>
				<span class="min-w-0">
					<span class="block text-sm font-semibold text-[var(--color-text)]">{point.title}</span>
					{#if point.description}
						<span class="block text-xs text-[var(--color-muted)]">{point.description}</span>
					{/if}
				</span>
			</li>
		{/each}
	</ul>
{/if}
