<script lang="ts">
	import { Inbox } from 'lucide-svelte';

	let { title = 'Nothing here yet', description = '', class: className = '', children = undefined } = $props();
</script>

<div class="flex flex-col items-center justify-center py-12 {className}">
	<Inbox class="mb-4 h-12 w-12 text-gray-400" />
	<h3 class="mb-1 text-lg font-semibold text-gray-900 dark:text-gray-100">{title}</h3>
	{#if description}
		<p class="text-sm text-gray-500 dark:text-gray-400">{description}</p>
	{/if}
	{#if children}
		<div class="mt-4">
			{@render children()}
		</div>
	{/if}
</div>
