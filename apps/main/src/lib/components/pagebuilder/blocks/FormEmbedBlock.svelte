<script lang="ts">
	import type { FormEmbedProps } from '../types';
	import { safeHref } from '$lib/utils/safeHtml';

	let { props }: { props: FormEmbedProps } = $props();
	const height = $derived(props.height_px ?? 600);
	// Validate the embed URL like any href — a javascript:/data: iframe src
	// would execute on load. Unsafe values render nothing (fail closed).
	const src = $derived(safeHref(props.form_url));
</script>

<div class="mx-auto w-full max-w-2xl px-4 py-6">
	{#if props.title}
		<h3 class="mb-3 text-center text-sm font-medium text-[var(--color-text)]">{props.title}</h3>
	{/if}
	{#if src}
		<div class="overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)]">
			<iframe
				src={src}
				title={props.title ?? 'Form'}
				class="w-full"
				style="height: {height}px; border: 0;"
				loading="lazy"
				sandbox="allow-forms allow-scripts allow-same-origin allow-popups"
				referrerpolicy="no-referrer"
			></iframe>
		</div>
	{/if}
</div>
