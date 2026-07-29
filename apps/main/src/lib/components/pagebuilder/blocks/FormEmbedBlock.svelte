<script lang="ts">
	import type { FormEmbedProps } from '../types';

	let { props }: { props: FormEmbedProps } = $props();
	const height = $derived(props.height_px ?? 600);
</script>

<div class="mx-auto w-full max-w-2xl px-4 py-6">
	{#if props.title}
		<h3 class="mb-3 text-center text-sm font-medium text-[var(--color-text)]">{props.title}</h3>
	{/if}
	<div class="overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)]">
		<iframe
			src={props.form_url}
			title={props.title ?? 'Form'}
			class="w-full"
			style="height: {height}px; border: 0;"
			loading="lazy"
		></iframe>
	</div>
</div>
