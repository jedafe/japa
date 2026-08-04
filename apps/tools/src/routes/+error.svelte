<script lang="ts">
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import { mainAppUrl } from '$lib/seo';
	import { t } from '$lib/i18n';

	const status = $derived(page.status);
	const message = $derived(page.error?.message ?? 'Something went wrong');
</script>

<div class="flex flex-col items-center justify-center min-h-[60vh] px-4 text-center">
	<div class="text-6xl font-bold text-primary mb-4">{status}</div>
	<h1 class="text-2xl font-semibold text-text mb-2">
		{#if status === 404}
			{t('error.404.title', 'Page not found')}
		{:else if status === 500}
			{t('error.500.title', 'Server error')}
		{:else}
			{message}
		{/if}
	</h1>
	<p class="text-text-subtle mb-6 max-w-md">
		{#if status === 404}
			{t('error.404.description', "The tool or page you're looking for doesn't exist or may have moved.")}
		{:else}
			{message}
		{/if}
	</p>
	<a
		href="/"
		class="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-primary text-primary-contrast text-sm font-medium hover:bg-primary-hover transition-colors"
	>
		<Icon name="home" size={16} />
		{t('error.back_to_tools', 'Back to Tools')}
	</a>
</div>
