<script lang="ts">
	import type { SliderProps } from '../types';
	import { onMount } from 'svelte';
	import { ChevronLeft, ChevronRight } from 'lucide-svelte';
	import { safeHref } from '$lib/utils/safeHtml';

	let { props }: { props: SliderProps } = $props();
	let current = $state(0);
	let timer: ReturnType<typeof setInterval> | null = null;

	const slides = $derived(props.slides ?? []);
	const intervalMs = $derived(props.interval_ms ?? 5000);
	// Validate every slide URL once: unsafe image/CTA targets fail closed.
	const safeSlides = $derived(
		slides.map((s) => ({
			...s,
			image_url: s.image_url ? safeHref(s.image_url) : '',
			cta_url: s.cta_url ? safeHref(s.cta_url) : ''
		}))
	);

	function next() {
		current = (current + 1) % slides.length;
	}
	function prev() {
		current = (current - 1 + slides.length) % slides.length;
	}

	onMount(() => {
		if (props.autoplay && slides.length > 1) {
			timer = setInterval(next, intervalMs);
		}
		return () => {
			if (timer) clearInterval(timer);
		};
	});
</script>

{#if safeSlides.length > 0}
	<div class="relative h-64 overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] sm:h-80">
		{#each safeSlides as slide, i (i)}
			<div
				class="absolute inset-0 transition-opacity duration-500 {i === current ? 'opacity-100' : 'opacity-0'}"
				aria-hidden={i !== current}
			>
				{#if slide.image_url}
					<img src={slide.image_url} alt={slide.title ?? ''} class="h-full w-full object-cover" />
				{/if}
				<div class="absolute inset-0 bg-gradient-to-t from-black/60 to-transparent"></div>
				<div class="absolute bottom-0 left-0 p-6 text-white">
					{#if slide.title}<h3 class="text-lg font-semibold">{slide.title}</h3>{/if}
					{#if slide.subtitle}<p class="mt-1 text-sm opacity-90">{slide.subtitle}</p>{/if}
					{#if slide.cta_text && slide.cta_url}
						<a href={slide.cta_url} class="mt-3 inline-block rounded-[var(--radius)] bg-white/20 px-4 py-2 text-sm font-medium backdrop-blur hover:bg-white/30">
							{slide.cta_text}
						</a>
					{/if}
				</div>
			</div>
		{/each}

		{#if safeSlides.length > 1}
			<button type="button" onclick={prev} class="absolute left-3 top-1/2 grid h-9 w-9 -translate-y-1/2 place-items-center rounded-full bg-white/20 text-white backdrop-blur hover:bg-white/30" aria-label="Previous">
				<ChevronLeft size={18} />
			</button>
			<button type="button" onclick={next} class="absolute right-3 top-1/2 grid h-9 w-9 -translate-y-1/2 place-items-center rounded-full bg-white/20 text-white backdrop-blur hover:bg-white/30" aria-label="Next">
				<ChevronRight size={18} />
			</button>
			<div class="absolute bottom-3 left-1/2 flex -translate-x-1/2 gap-1.5">
				{#each safeSlides as _, i (i)}
					<button type="button" onclick={() => (current = i)} class="h-1.5 rounded-full transition-all {i === current ? 'w-6 bg-white' : 'w-1.5 bg-white/50'}" aria-label="Go to slide {i + 1}"></button>
				{/each}
			</div>
		{/if}
	</div>
{/if}
