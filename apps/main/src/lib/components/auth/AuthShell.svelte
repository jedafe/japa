<script lang="ts">
	// Ref: japaconnect-engineering/sections/02-authentication-sessions.md
	// ADR-058 4th exception: auth layout (not AppShell).
	// Full-page transparent carousel background + centered rounded card
	// with left brand panel (carousel) and right form panel.
	// Carousel slides configurable via appearance.auth_brand_slides.
	import type { Snippet } from 'svelte';
	import { Compass, ArrowLeft } from 'lucide-svelte';
	import { t } from '$lib/i18n/store.svelte';
	import ThemeLogo from '$lib/components/branding/ThemeLogo.svelte';
	import type { BrandSlide } from '$lib/auth/brandSlides';

	let {
		children,
		brandSlides = [],
		logoUrl = '',
		logoDarkUrl = ''
	}: {
		children: Snippet;
		brandSlides?: BrandSlide[];
		logoUrl?: string;
		logoDarkUrl?: string;
	} = $props();

	const wordmark = $derived(t('app.name'));

	// Carousel state — only used when brandSlides has >1 entry.
	let activeSlide = $state(0);
	const SLIDE_INTERVAL = 6000;

	// Minimal fallback in case the API is unreachable on first render.
	const fallbackSlide: BrandSlide = {
		image: '/brand/kigali-airplane-window.jpg',
		headline: t('auth.brand.slide1_headline'),
		subheading: t('auth.brand.slide1_subheading')
	};

	// Effective slides: configured slides from the API, or the single fallback.
	const slides = $derived(brandSlides.length > 0 ? brandSlides : [fallbackSlide]);
	const hasCarousel = $derived(slides.length > 1);

	// Auto-advance the carousel; pause when the tab is hidden.
	$effect(() => {
		if (!hasCarousel) return;
		const id = setInterval(() => {
			activeSlide = (activeSlide + 1) % slides.length;
		}, SLIDE_INTERVAL);
		const onVisibility = () => {
			if (document.hidden) clearInterval(id);
		};
		document.addEventListener('visibilitychange', onVisibility);
		return () => {
			clearInterval(id);
			document.removeEventListener('visibilitychange', onVisibility);
		};
	});
</script>

<div class="relative flex min-h-screen items-center justify-center overflow-hidden p-4">
	<!-- Full-page transparent carousel background (confirmed good) -->
	<div class="pointer-events-none absolute inset-0 z-0 overflow-hidden">
		{#each slides as slide, i (slide.image + i)}
			<img
				src={slide.image}
				alt=""
				aria-hidden="true"
				class="absolute inset-0 h-full w-full object-cover transition-opacity duration-1000"
				class:opacity-25={activeSlide === i}
				class:opacity-0={activeSlide !== i}
				draggable="false"
			/>
		{/each}
		<div class="absolute inset-0 bg-[var(--color-bg)]/70"></div>
	</div>

	<!-- Centered card: left brand panel + right form panel -->
	<div
		class="relative z-10 flex w-full max-w-5xl flex-col overflow-hidden rounded-3xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-2xl shadow-black/50 lg:flex-row"
		style="background: color-mix(in srgb, var(--color-surface) 95%, transparent)"
	>
		<!-- Left: brand carousel panel (50%) — full-bleed image cover -->
		<aside class="relative hidden lg:flex lg:w-[50%] flex-col self-stretch overflow-hidden" aria-hidden="true">
			<!-- Full-bleed carousel images covering the entire panel -->
			{#each slides as slide, i (slide.image + i)}
				<img
					src={slide.image}
					alt=""
					class="absolute inset-0 h-full w-full object-cover transition-opacity duration-700"
					class:opacity-100={activeSlide === i}
					class:opacity-0={activeSlide !== i}
					draggable="false"
				/>
			{/each}

			<!-- Dark overlay for overall legibility -->
			<div class="absolute inset-0 bg-black/40"></div>
			<!-- Gradient overlay for bottom text legibility -->
			<div class="absolute inset-0 bg-gradient-to-t from-black/70 via-transparent to-transparent"></div>

			<!-- Top bar: wordmark + back link -->
			<div class="relative z-10 flex items-center justify-between p-5">
				<a
					href="/"
					class="flex items-center gap-2 text-base font-semibold tracking-tight text-white no-underline"
				>
					<ThemeLogo {logoUrl} {logoDarkUrl} alt={t('app.name')} class="h-8 w-auto max-w-[140px] object-contain" />
				</a>
				<a
					href="/"
					class="flex items-center gap-1.5 rounded-full bg-white/10 px-3 py-1.5 text-xs text-white/90 no-underline backdrop-blur-sm transition hover:bg-white/20"
				>
					<ArrowLeft size={12} />
					{t('auth.brand.back_home')}
				</a>
			</div>

			<!-- Push text to bottom -->
			<div class="flex-1"></div>

			<!-- Slide headline + subheading (overlaid on image) -->
			<div class="relative z-10 p-5">
				{#each slides as slide, i (slide.image + i)}
					<div
						class="transition-opacity duration-700 {hasCarousel ? 'absolute inset-x-5 bottom-16' : 'relative'}"
						class:opacity-100={activeSlide === i}
						class:opacity-0={activeSlide !== i}
					>
						<h2 class="text-balance text-xl font-semibold leading-tight text-white">
							{slide.headline}
						</h2>
						<p class="mt-1 text-sm text-white/80">
							{slide.subheading}
						</p>
					</div>
				{/each}
			</div>

			<!-- Slide dots -->
			{#if hasCarousel}
				<div class="relative z-10 mb-4 flex items-center justify-center gap-1.5">
					{#each slides as _, i}
						<button
							type="button"
							onclick={() => (activeSlide = i)}
							class="h-0.5 rounded-full transition-all duration-300 {activeSlide === i
								? 'w-6 bg-white'
								: 'w-3 bg-white/20 hover:bg-white/40'}"
							aria-label="Slide {i + 1}"
						></button>
					{/each}
				</div>
			{/if}
		</aside>

		<!-- Right: form panel (50%) -->
		<main class="flex lg:w-[50%] flex-col justify-center p-6 lg:p-8">
			<!-- Mobile wordmark (hidden on desktop) -->
			<div class="mb-6 flex items-center justify-center lg:hidden">
				<a
					href="/"
					class="flex items-center gap-2 text-base font-semibold text-[var(--color-primary)] no-underline"
				>
					<ThemeLogo {logoUrl} {logoDarkUrl} alt={t('app.name')} class="h-8 w-auto max-w-[140px] object-contain" />
				</a>
			</div>

			{@render children()}
		</main>
	</div>
</div>
