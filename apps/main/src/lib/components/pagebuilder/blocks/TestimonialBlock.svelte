<script lang="ts">
	import type { TestimonialProps } from '../types';
	import { Star } from 'lucide-svelte';
	import { safeHref } from '$lib/utils/safeHtml';

	let { props }: { props: TestimonialProps } = $props();
	const avatarSrc = $derived(props.avatar_url ? safeHref(props.avatar_url) : '');
</script>

<div class="mx-auto w-full max-w-2xl px-4 py-6">
	<figure class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6">
		{#if props.rating}
			<div class="mb-3 flex gap-0.5">
				{#each Array(5) as _, i (i)}
					<Star size={16} class={i < (props.rating ?? 0) ? 'fill-[var(--color-primary)] text-[var(--color-primary)]' : 'text-[var(--color-muted)]'} />
				{/each}
			</div>
		{/if}
		<blockquote class="text-base leading-relaxed text-[var(--color-text)]">
			"{props.quote}"
		</blockquote>
		<figcaption class="mt-4 flex items-center gap-3">
			{#if avatarSrc}
				<img src={avatarSrc} alt={props.author_name} class="h-10 w-10 rounded-full object-cover" />
			{:else}
				<div class="grid h-10 w-10 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]">
					{props.author_name?.charAt(0).toUpperCase() ?? '?'}
				</div>
			{/if}
			<div>
				<p class="text-sm font-medium text-[var(--color-text)]">{props.author_name}</p>
				{#if props.author_title}
					<p class="text-xs text-[var(--color-muted)]">{props.author_title}</p>
				{/if}
			</div>
		</figcaption>
	</figure>
</div>
