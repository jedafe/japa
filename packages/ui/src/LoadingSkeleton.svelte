<script lang="ts">
	// LoadingSkeleton — shared pulsing placeholder.
	// Modes:
	// - variant set ('lines' | 'card' | 'chart') → structured placeholder group.
	// - otherwise → a single bar sized by width/height/radius props or `class`.
	interface Props {
		/** 'lines' = stacked text bars; 'card' = avatar + lines block; 'chart' = bar chart placeholder. */
		variant?: 'lines' | 'card' | 'chart';
		/** Number of rows (lines) or cards to render. */
		count?: number;
		/** Single-bar width (single mode only). */
		width?: string;
		/** Single-bar height (single mode only). */
		height?: string;
		/** Single-bar border radius (single mode only). */
		radius?: string;
		class?: string;
	}

	let { variant, count = 3, width, height, radius, class: klass = '' }: Props = $props();

	const widths = ['92%', '78%', '64%', '85%', '70%'];
	const delays = [0, 0.1, 0.15, 0.2];
</script>

{#if variant}
	<!-- Structured variants — staggered pulse placeholders. -->
	<div class={klass} aria-hidden="true" data-testid="loading-skeleton">
		{#if variant === 'lines'}
			<div class="space-y-3">
				{#each Array.from({ length: count }) as _, i (i)}
					<div
						class="h-4 animate-pulse rounded bg-[var(--color-surface-subtle,var(--color-muted))]"
						style="width: {widths[i % widths.length]}; animation-delay: {i * 0.12}s"
					></div>
				{/each}
			</div>
		{:else if variant === 'card'}
			<div class="space-y-4">
				{#each Array.from({ length: count }) as _, i (i)}
					<div
						class="rounded-[var(--radius-card,var(--radius-md,8px))] border border-[var(--color-border)] bg-[var(--color-surface,var(--color-background))] p-4"
					>
						<div class="flex items-center gap-3">
							<div class="h-10 w-10 shrink-0 animate-pulse rounded-full bg-[var(--color-surface-subtle,var(--color-muted))]"></div>
							<div class="flex-1 space-y-2">
								<div class="h-3 w-1/3 animate-pulse rounded bg-[var(--color-surface-subtle,var(--color-muted))]"></div>
								<div class="h-3 w-1/4 animate-pulse rounded bg-[var(--color-surface-subtle,var(--color-muted))]"></div>
							</div>
						</div>
						<div class="mt-4 space-y-2">
							<div class="h-3 w-full animate-pulse rounded bg-[var(--color-surface-subtle,var(--color-muted))]"></div>
							<div class="h-3 w-4/5 animate-pulse rounded bg-[var(--color-surface-subtle,var(--color-muted))]"></div>
						</div>
					</div>
				{/each}
			</div>
		{:else}
			<!-- chart: pulsing bar placeholders -->
			<div class="flex h-full items-end justify-around gap-4 pb-8">
				{#each [60, 85, 45, 70] as h, i (i)}
					<div
						class="w-1/5 animate-pulse rounded-t bg-[var(--color-surface-subtle,var(--color-muted))]"
						style="height: {h}%; animation-delay: {delays[i]}s"
					></div>
				{/each}
			</div>
		{/if}
	</div>
{:else}
	<!-- Single bar — sized by class and/or explicit width/height/radius props. -->
	<div
		class="animate-pulse rounded-[var(--radius,var(--radius-md,4px))] bg-[var(--color-surface-subtle,var(--color-muted))] {klass}"
		style="{width ? `width: ${width};` : ''}{height ? `height: ${height};` : ''}{radius ? `border-radius: ${radius};` : ''}"
		aria-hidden="true"
		data-testid="loading-skeleton"
	></div>
{/if}
