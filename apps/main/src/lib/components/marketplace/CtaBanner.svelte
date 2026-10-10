<script lang="ts">
	// Bottom-of-page CTA banner — the §114 2026-10-10 mockup's full-width
	// gradient block ("Ready to work together?" / "Need a custom package?"
	// / "Not sure if this is the right mentor?"). The eyebrow eyebrow line
	// and two buttons are optional; the primary delegates to the page.
	interface Props {
		eyebrow?: string;
		heading: string;
		body?: string;
		primaryLabel: string;
		onPrimary?: () => void;
		primaryHref?: string;
		secondaryLabel?: string;
		secondaryHref?: string;
		onSecondary?: () => void;
	}
	let {
		eyebrow = '',
		heading,
		body = '',
		primaryLabel,
		onPrimary,
		primaryHref = '',
		secondaryLabel = '',
		secondaryHref = '',
		onSecondary
	}: Props = $props();
</script>

<section
	class="relative overflow-hidden rounded-2xl px-6 py-12 text-center text-[var(--color-primary-contrast)] sm:px-12"
	style="background: linear-gradient(135deg, var(--color-primary-hover) 0%, var(--color-primary) 55%, var(--color-primary-hover) 100%);"
>
	<!-- faint oversized rings — the mockup's decorative circles -->
	<span
		class="pointer-events-none absolute -left-20 -top-24 h-64 w-64 rounded-full border-[24px] border-white/10"
		aria-hidden="true"
	></span>
	<span
		class="pointer-events-none absolute -bottom-28 -right-16 h-72 w-72 rounded-full border-[28px] border-white/10"
		aria-hidden="true"
	></span>

	{#if eyebrow}
		<p class="text-xs font-semibold uppercase tracking-widest text-white/80">{eyebrow}</p>
	{/if}
	<h2 class="mt-2 text-2xl font-bold tracking-tight sm:text-3xl">{heading}</h2>
	{#if body}
		<p class="mx-auto mt-3 max-w-xl text-sm leading-relaxed text-white/85">{body}</p>
	{/if}
	<div class="mt-7 flex flex-wrap items-center justify-center gap-3">
		{#if primaryHref}
			<a
				href={primaryHref}
				class="inline-flex items-center justify-center rounded-lg bg-white px-6 py-2.5 text-sm font-semibold text-[var(--color-primary)] transition hover:bg-white/90"
			>
				{primaryLabel}
			</a>
		{:else}
			<button
				type="button"
				onclick={onPrimary}
				class="inline-flex items-center justify-center rounded-lg bg-white px-6 py-2.5 text-sm font-semibold text-[var(--color-primary)] transition hover:bg-white/90"
			>
				{primaryLabel}
			</button>
		{/if}
		{#if secondaryLabel}
			{#if secondaryHref}
				<a
					href={secondaryHref}
					class="inline-flex items-center justify-center rounded-lg border border-white/40 px-6 py-2.5 text-sm font-semibold text-white transition hover:bg-white/10"
				>
					{secondaryLabel}
				</a>
			{:else if onSecondary}
				<button
					type="button"
					onclick={onSecondary}
					class="inline-flex items-center justify-center rounded-lg border border-white/40 px-6 py-2.5 text-sm font-semibold text-white transition hover:bg-white/10"
				>
					{secondaryLabel}
				</button>
			{:else}
				<span class="text-sm font-medium text-white/80">{secondaryLabel}</span>
			{/if}
		{/if}
	</div>
</section>
