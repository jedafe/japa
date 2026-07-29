<script lang="ts">
	import type { ImageGalleryProps } from '../types';
	let { props }: { props: ImageGalleryProps } = $props();
	const columns = $derived(props.columns ?? 3);
	const gridClass = $derived(
		columns === 2 ? 'grid-cols-2' : columns === 4 ? 'grid-cols-2 sm:grid-cols-4' : 'grid-cols-2 sm:grid-cols-3'
	);
</script>

{#if props.images?.length}
	<div class="mx-auto w-full max-w-4xl px-4 py-6">
		<div class="grid {gridClass} gap-3">
			{#each props.images as img, i (i)}
				<figure class="overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)]">
					<img src={img.url} alt={img.alt ?? ''} class="h-full w-full object-cover" loading="lazy" />
					{#if img.caption}
						<figcaption class="p-2 text-xs text-[var(--color-muted)]">{img.caption}</figcaption>
					{/if}
				</figure>
			{/each}
		</div>
	</div>
{/if}
