<script lang="ts">
	import type { StatsProps } from '../types';

	interface Props {
		props: StatsProps;
	}
	let { props }: Props = $props();
</script>

<section class="mx-auto max-w-4xl px-6 py-10">
	<div class="grid grid-cols-2 gap-6 sm:grid-cols-4">
		{#each props.items as item, i (i)}
			<div class="text-center">
				<div class="text-3xl font-bold text-[var(--color-text)] [font-variant-numeric:tabular-nums]">
					{item.prefix ?? ''}{item.value}{item.suffix ?? ''}
				</div>
				<div class="mt-1 text-sm text-[var(--color-text-subtle)]">{item.label}</div>
			</div>
		{/each}
	</div>
</section>
