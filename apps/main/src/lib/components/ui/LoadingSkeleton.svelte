<script lang="ts">
	interface Props {
		/** 'lines' = stacked text bars; 'card' = avatar + lines block. */
		variant?: 'lines' | 'card';
		/** Number of rows (lines) or cards to render. */
		count?: number;
		class?: string;
	}

	let { variant = 'lines', count = 3, class: klass = '' }: Props = $props();

	const widths = ['92%', '78%', '64%', '85%', '70%'];
</script>

<!-- Distinct from EmptyState: this is a pulsing placeholder, never a message. -->
<div class={klass} aria-hidden="true" data-testid="loading-skeleton">
	{#if variant === 'lines'}
		<div class="space-y-3">
			{#each Array.from({ length: count }) as _, i (i)}
				<div
					class="h-4 rounded bg-[var(--color-surface-subtle)]"
					style="width: {widths[i % widths.length]}; animation: jc-skeleton 1.4s ease-in-out infinite; animation-delay: {i *
						0.12}s"
				></div>
			{/each}
		</div>
	{:else}
		<div class="space-y-4">
			{#each Array.from({ length: count }) as _, i (i)}
				<div
					class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
				>
					<div class="flex items-center gap-3">
						<div
							class="h-10 w-10 shrink-0 rounded-full bg-[var(--color-surface-subtle)]"
							style="animation: jc-skeleton 1.4s ease-in-out infinite"
						></div>
						<div class="flex-1 space-y-2">
							<div
								class="h-3 w-1/3 rounded bg-[var(--color-surface-subtle)]"
								style="animation: jc-skeleton 1.4s ease-in-out infinite"
							></div>
							<div
								class="h-3 w-1/4 rounded bg-[var(--color-surface-subtle)]"
								style="animation: jc-skeleton 1.4s ease-in-out infinite; animation-delay: 0.1s"
							></div>
						</div>
					</div>
					<div class="mt-4 space-y-2">
						<div
							class="h-3 w-full rounded bg-[var(--color-surface-subtle)]"
							style="animation: jc-skeleton 1.4s ease-in-out infinite; animation-delay: 0.15s"
						></div>
						<div
							class="h-3 w-4/5 rounded bg-[var(--color-surface-subtle)]"
							style="animation: jc-skeleton 1.4s ease-in-out infinite; animation-delay: 0.2s"
						></div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
