<script lang="ts">
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import { mainAppUrl } from '$lib/seo';

	const status = $derived(page.status);
	const message = $derived(page.error?.message ?? 'Something went wrong');
</script>

<div class="flex flex-col items-center justify-center min-h-[60vh] px-4 text-center">
	<div class="text-6xl font-bold text-primary mb-4">{status}</div>
	<h1 class="text-2xl font-semibold text-text mb-2">
		{#if status === 404}
			Page not found
		{:else if status === 500}
			Server error
		{:else}
			{message}
		{/if}
	</h1>
	<p class="text-text-subtle mb-6 max-w-md">
		{#if status === 404}
			The tool or page you're looking for doesn't exist or may have moved.
		{:else}
			{message}
		{/if}
	</p>
	<a
		href="/"
		class="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-primary text-primary-contrast text-sm font-medium hover:bg-primary-hover transition-colors"
	>
		<Icon name="home" size={16} />
		Back to Tools
	</a>
</div>
