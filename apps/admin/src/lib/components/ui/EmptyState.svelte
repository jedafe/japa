<script lang="ts">
	import { Inbox } from 'lucide-svelte';

	let { title = 'Nothing here yet', description = '', class: className = '', children = undefined } = $props();
</script>

<div class="flex flex-col items-center justify-center py-12 {className}">
	<Inbox class="mb-4 h-12 w-12" style="color: var(--color-muted)" />
	<h3 class="mb-1 text-lg font-semibold" style="color: var(--color-foreground)">{title}</h3>
	{#if description}
		<p class="text-sm" style="color: var(--color-muted)">{description}</p>
	{/if}
	{#if children}
		<div class="mt-4">
			{@render children()}
		</div>
	{/if}
</div>
