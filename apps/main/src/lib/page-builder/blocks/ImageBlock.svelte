<script lang="ts">
	import { imgproxy } from '$lib/utils/imgproxy';

	interface ImageItem {
		url: string;
		alt: string;
	}

	interface Props {
		images?: ImageItem[];
		caption?: string;
		layout?: 'single' | 'grid' | 'carousel';
		readOnly?: boolean;
	}

	let {
		images = [],
		caption = '',
		layout = 'single',
		readOnly = false,
	}: Props = $props();

	let scrollIndex = $state(0);

	function scrollNext() {
		if (images.length === 0) return;
		scrollIndex = (scrollIndex + 1) % images.length;
	}

	function scrollPrev() {
		if (images.length === 0) return;
		scrollIndex = (scrollIndex - 1 + images.length) % images.length;
	}

	let currentImage = $derived(images[scrollIndex] ?? null);
	let wrapperClass = $derived(
		`block-image${!readOnly ? ' ring-2 ring-blue-400/40 hover:ring-blue-400/60' : ''}`
	);
</script>

<div class={wrapperClass}>
	{#if images.length === 0}
		<div class="flex h-48 items-center justify-center rounded-lg border-2 border-dashed border-gray-300 bg-gray-50 text-sm text-gray-400 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-500">
			No images added
		</div>
	{:else if layout === 'single'}
		<figure class="overflow-hidden rounded-lg">
			<img
				src={imgproxy(images[0].url, { width: 1200, format: 'webp', quality: 85 })}
				alt={images[0].alt}
				class="h-auto w-full object-cover"
				loading="lazy"
			/>
			{#if caption}
				<figcaption class="mt-2 text-center text-sm text-gray-500 dark:text-gray-400">
					{caption}
				</figcaption>
			{/if}
		</figure>
	{:else if layout === 'grid'}
		<figure>
			<div class="grid gap-3 {images.length === 2 ? 'grid-cols-2' : images.length === 3 ? 'grid-cols-3' : 'grid-cols-2 md:grid-cols-4'}">
				{#each images.slice(0, 4) as img}
					<img
						src={imgproxy(img.url, { width: 600, format: 'webp', quality: 80 })}
						alt={img.alt}
						class="aspect-square h-full w-full rounded-lg object-cover"
						loading="lazy"
					/>
				{/each}
			</div>
			{#if caption}
				<figcaption class="mt-2 text-center text-sm text-gray-500 dark:text-gray-400">
					{caption}
				</figcaption>
			{/if}
		</figure>
	{:else if layout === 'carousel' && currentImage}
		<figure class="relative overflow-hidden rounded-lg">
			<img
				src={imgproxy(currentImage.url, { width: 1200, format: 'webp', quality: 85 })}
				alt={currentImage.alt}
				class="h-auto w-full object-cover"
			/>

			{#if images.length > 1}
				<button
					type="button"
					class="absolute left-3 top-1/2 -translate-y-1/2 rounded-full bg-black/50 p-2 text-white transition-colors hover:bg-black/70"
					aria-label="Previous image"
					onclick={scrollPrev}
				>
					<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M15 19l-7-7 7-7" />
					</svg>
				</button>
				<button
					type="button"
					class="absolute right-3 top-1/2 -translate-y-1/2 rounded-full bg-black/50 p-2 text-white transition-colors hover:bg-black/70"
					aria-label="Next image"
					onclick={scrollNext}
				>
					<svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
					</svg>
				</button>

				<div class="absolute bottom-3 left-1/2 flex -translate-x-1/2 gap-1.5">
					{#each images as _, i}
						<span
							class="h-2 w-2 rounded-full transition-colors {i === scrollIndex ? 'bg-white' : 'bg-white/40'}"
						></span>
					{/each}
				</div>
			{/if}

			{#if caption}
				<figcaption class="mt-2 text-center text-sm text-gray-500 dark:text-gray-400">
					{caption}
				</figcaption>
			{/if}
		</figure>
	{/if}
</div>
