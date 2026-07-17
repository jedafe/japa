<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	interface SlideItem {
		id: string;
		image_url: string;
		caption?: string;
		alt_text?: string;
	}

	interface Props {
		slides?: SlideItem[];
		autoplay?: boolean;
		interval_ms?: number;
		show_dots?: boolean;
		show_arrows?: boolean;
		height?: number;
		readOnly?: boolean;
	}

	let {
		slides = [],
		autoplay = false,
		interval_ms = 5000,
		show_dots = true,
		show_arrows = true,
		height = 400,
		readOnly = false,
	}: Props = $props();

	let currentIndex = $state(0);
	let intervalId = $state<number | null>(null);

	function next() {
		if (slides.length === 0) return;
		currentIndex = (currentIndex + 1) % slides.length;
	}

	function prev() {
		if (slides.length === 0) return;
		currentIndex = (currentIndex - 1 + slides.length) % slides.length;
	}

	function goTo(index: number) {
		currentIndex = index;
	}

	$effect(() => {
		if (autoplay && slides.length > 1) {
			intervalId = window.setInterval(next, interval_ms);
		}
		return () => {
			if (intervalId) clearInterval(intervalId);
		};
	});

	let wrapperClass = $derived(
		`block-slider${!readOnly ? ' ring-2 ring-blue-400/40 hover:ring-blue-400/60' : ''}`
	);
</script>

<div class={wrapperClass}>
	{#if slides.length === 0}
		<div class="flex h-32 items-center justify-center rounded-lg border-2 border-dashed border-gray-300 bg-gray-50 text-sm text-gray-400 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-500">
			Add slides to create a carousel
		</div>
	{:else}
		<div class="relative overflow-hidden rounded-lg" style="height: {height}px">
			{#each slides as slide, i}
				<div
					class="absolute inset-0 transition-opacity duration-500 {i === currentIndex ? 'opacity-100' : 'opacity-0'}"
				>
					<img
						src={slide.image_url}
						alt={slide.alt_text ?? slide.caption ?? ''}
						class="h-full w-full object-cover"
					/>
					{#if slide.caption}
						<div class="absolute bottom-0 left-0 right-0 bg-gradient-to-t from-black/60 to-transparent p-4">
							<p class="text-sm text-white">{slide.caption}</p>
						</div>
					{/if}
				</div>
			{/each}

			{#if show_arrows && slides.length > 1}
				<button
					class="absolute left-2 top-1/2 -translate-y-1/2 rounded-full bg-black/30 p-2 text-white backdrop-blur-sm hover:bg-black/50"
					onclick={prev}
					aria-label="Previous slide"
				>
					<svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
					</svg>
				</button>
				<button
					class="absolute right-2 top-1/2 -translate-y-1/2 rounded-full bg-black/30 p-2 text-white backdrop-blur-sm hover:bg-black/50"
					onclick={next}
					aria-label="Next slide"
				>
					<svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
					</svg>
				</button>
			{/if}

			{#if show_dots && slides.length > 1}
				<div class="absolute bottom-3 left-1/2 flex -translate-x-1/2 gap-2">
					{#each slides as _, i}
						<button
							class="h-2 w-2 rounded-full transition-colors {i === currentIndex ? 'bg-white' : 'bg-white/50'}"
							onclick={() => goTo(i)}
							aria-label="Go to slide {i + 1}"
						></button>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
</div>
