<script lang="ts">
	// Tool page mini header: accent-tinted panel with the tool icon, title +
	// optional description, a decorative HeroArtTools scene on the right
	// (desktop only), and a "How it works" button opening the per-tool guide
	// slide-over (tools.guides system setting — see ToolGuideButton).
	// The full-width workbenches (readiness, JapaBot, travel) do NOT use this
	// header — they own their own heroes.
	import { page } from '$app/state';
	import HeroArtTools from '#lib/components/heroes/HeroArtTools.svelte';
	import ToolGuideButton from './ToolGuideButton.svelte';
	import type { IconComponent } from '#lib/components/ui/index.js';

	interface Props {
		icon: IconComponent;
		title: string;
		description?: string;
	}

	let { icon: Icon, title, description }: Props = $props();

	// Slug for the tools.guides lookup — resolved from the URL so every
	// /tools/{slug} page works without passing extra props.
	const toolSlug = $derived.by(() => {
		const segments = page.url.pathname.split('/').filter(Boolean);
		return segments.length === 2 && segments[0] === 'tools' ? segments[1] : '';
	});
</script>

<header
	class="tool-hero relative mb-5 overflow-hidden rounded-[var(--radius-card)] border sm:mb-6"
>
	<div class="relative flex items-center gap-3 px-4 py-3.5 sm:gap-4 sm:px-5 sm:py-4">
		<div class="min-w-0 flex-1">
			<div class="flex items-center gap-3 sm:gap-4">
				<span
					class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius)] text-[var(--color-primary-contrast)] shadow-sm sm:h-12 sm:w-12"
					style="background: linear-gradient(135deg, var(--color-primary), color-mix(in srgb, var(--color-primary) 72%, white))"
				>
					<Icon size={20} />
				</span>
				<div class="min-w-0">
					<h1 class="truncate text-base font-semibold leading-tight text-[var(--color-text)] sm:text-lg">
						{title}
					</h1>
					{#if description}
						<p class="mt-0.5 line-clamp-2 text-xs text-[var(--color-muted)] sm:text-sm">{description}</p>
					{/if}
				</div>
			</div>
			{#if toolSlug}
				<div class="mt-2">
					<ToolGuideButton slug={toolSlug} />
				</div>
			{/if}
		</div>
		{#if toolSlug}
			<!-- Decorative scene — desktop only; pointer-events-none so it can
			     never swallow clicks on the "How it works" trigger. -->
			<div class="pointer-events-none relative hidden h-20 w-32 shrink-0 md:block lg:h-24 lg:w-44" aria-hidden="true">
				<HeroArtTools accent="var(--color-primary)" />
			</div>
		{/if}
	</div>
</header>

<style>
	/* Mini version of EntityHero's tinted panel — same token-driven gradient
	   so it follows the site palette (and dark mode). */
	.tool-hero {
		border-color: color-mix(in srgb, var(--color-primary) 18%, var(--color-border));
		background:
			radial-gradient(
				140% 200% at 96% 0%,
				color-mix(in srgb, var(--color-primary) 14%, transparent),
				transparent 55%
			),
			linear-gradient(
				135deg,
				color-mix(in srgb, var(--color-primary) 6%, var(--color-surface)) 0%,
				color-mix(in srgb, var(--color-primary) 11%, var(--color-surface)) 100%
			);
	}
</style>
