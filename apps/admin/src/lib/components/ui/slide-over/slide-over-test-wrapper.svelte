<script lang="ts">
	import SlideOver from './slide-over.svelte';

	let {
		open = true,
		title = 'Edit',
		description = '',
		body = 'form fields',
		footerText = ''
	}: {
		open?: boolean;
		title?: string;
		description?: string;
		body?: string;
		footerText?: string;
	} = $props();
</script>

<SlideOver {open} {title} {description}>
	<p>{body}</p>

	{#snippet footer()}
		{#if footerText}
			<button>{footerText}</button>
		{/if}
	{/snippet}
</SlideOver>
