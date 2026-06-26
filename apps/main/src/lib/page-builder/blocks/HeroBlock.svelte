<script lang="ts">
	interface Props {
		title?: string;
		subtitle?: string;
		background_image?: string;
		cta_text?: string;
		cta_url?: string;
		overlay_opacity?: number;
		readOnly?: boolean;
	}

	let {
		title = '',
		subtitle = '',
		background_image = '',
		cta_text = '',
		cta_url = '',
		overlay_opacity = 0.4,
		readOnly = false,
	}: Props = $props();
	let wrapperClass = $derived(
		`block-hero relative w-full overflow-hidden${!readOnly ? ' ring-2 ring-blue-400/40 hover:ring-blue-400/60' : ''}`
	);
</script>

<div class={wrapperClass}>
	{#if background_image}
		<img
			src={background_image}
			alt=""
			class="absolute inset-0 h-full w-full object-cover"
			aria-hidden="true"
		/>
	{/if}

	<div
		class="absolute inset-0 bg-black"
		style="opacity: {overlay_opacity}"
		aria-hidden="true"
	></div>

	<div class="relative z-10 flex min-h-[400px] flex-col items-center justify-center px-6 py-20 text-center text-white">
		{#if title}
			<h1 class="mb-4 max-w-3xl text-4xl font-bold leading-tight tracking-tight md:text-5xl lg:text-6xl">
				{title}
			</h1>
		{/if}

		{#if subtitle}
			<p class="mb-8 max-w-2xl text-lg text-white/80 md:text-xl">
				{subtitle}
			</p>
		{/if}

		{#if cta_text && cta_url}
			<a
				href={cta_url}
				class="inline-flex items-center rounded-lg bg-white px-8 py-3 text-base font-semibold text-gray-900 transition-colors hover:bg-gray-100 focus:outline-none focus:ring-2 focus:ring-white focus:ring-offset-2 focus:ring-offset-black"
			>
				{cta_text}
			</a>
		{/if}
	</div>
</div>
