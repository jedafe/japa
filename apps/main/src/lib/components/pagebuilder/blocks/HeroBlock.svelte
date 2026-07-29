<script lang="ts">
	import type { HeroProps } from '../types';

	interface Props {
		props: HeroProps;
	}
	let { props }: Props = $props();

	const align = $derived(props.text_align ?? 'center');
	const overlay = $derived(props.overlay_opacity ?? 0.4);
</script>

<section
	class="relative flex min-h-[420px] flex-col items-center justify-center px-6 py-16 text-center"
	class:text-left={align === 'left'}
	class:text-right={align === 'right'}
>
	{#if props.background_image}
		<img
			src={props.background_image}
			alt=""
			class="absolute inset-0 h-full w-full object-cover"
			loading="eager"
		/>
		<div class="absolute inset-0 bg-black" style="opacity: {overlay}"></div>
	{/if}
	<div class="relative z-10 mx-auto max-w-3xl">
		{#if props.title}
			<h1 class="text-balance text-4xl font-bold tracking-tight text-white sm:text-5xl">{props.title}</h1>
		{/if}
		{#if props.subtitle}
			<p class="mt-4 text-balance text-lg text-white/80">{props.subtitle}</p>
		{/if}
		{#if props.cta_text && props.cta_url}
			<a
				href={props.cta_url}
				class="mt-8 inline-flex items-center justify-center rounded-[var(--radius)] bg-[var(--color-primary)] px-6 py-3 text-sm font-semibold text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				{props.cta_text}
			</a>
		{/if}
	</div>
</section>
