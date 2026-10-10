<script lang="ts">
	import type { ImageGalleryProps } from '../types';
	import { safeHref } from '$lib/utils/safeHtml';
	let { props }: { props: ImageGalleryProps } = $props();
	const columns = $derived(props.columns ?? 3);
	const gridClass = $derived(
		columns === 2 ? 'grid-cols-2' : columns === 4 ? 'grid-cols-2 sm:grid-cols-4' : 'grid-cols-2 sm:grid-cols-3'
	);
	// Gallery images come from admin-authored props — validate each URL so
	// data:/javascript: payloads never reach <img src>. Unsafe entries are
	// skipped (fail closed).
	const safeImages = $derived(
		(props.images ?? [])
			.map((img) => ({ ...img, url: img.url ? safeHref(img.url) : '' }))
			.filter((img) => img.url)
	);
</script>

{#if safeImages.length}
	<div class="mx-auto w-full max-w-4xl px-4 py-6">
		<div class="grid {gridClass} gap-3">
			{#each safeImages as img, i (i)}
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
