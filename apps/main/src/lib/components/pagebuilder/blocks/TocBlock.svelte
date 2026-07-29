<script lang="ts">
	import type { TocProps } from '../types';

	let { props }: { props: TocProps } = $props();
	const items = $derived(props.items ?? []);
</script>

{#if items.length > 0}
	<nav class="mx-auto w-full max-w-2xl px-4 py-6">
		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			{#if props.title}
				<h3 class="mb-3 text-sm font-medium text-[var(--color-text)]">{props.title}</h3>
			{/if}
			<ul class="space-y-1.5">
				{#each items as item (item.anchor)}
					<li>
						<a href={`#${item.anchor}`} class="block text-sm text-[var(--color-primary)] hover:underline">
							{item.label}
						</a>
					</li>
				{/each}
			</ul>
		</div>
	</nav>
{/if}
