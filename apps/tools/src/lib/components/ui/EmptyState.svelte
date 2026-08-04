<script lang="ts">
	import Icon from './Icon.svelte';

	let {
		icon = 'inbox',
		title,
		description,
		action
	}: {
		icon?: string;
		title: string;
		description?: string;
		action?: { label: string; href: string };
	} = $props();
</script>

<div class="flex flex-col items-center justify-center py-16 px-4 text-center">
	<div
		class="flex items-center justify-center w-12 h-12 rounded-full bg-surface-subtle text-muted mb-4"
	>
		<Icon name={icon} size={24} />
	</div>
	<h3 class="text-lg font-semibold text-text">{title}</h3>
	{#if description}
		<p class="mt-1 text-sm text-text-subtle max-w-sm">{description}</p>
	{/if}
	{#if action}
		<a
			href={action.href}
			class="mt-4 inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-primary text-primary-contrast text-sm font-medium hover:bg-primary-hover transition-colors"
		>
			{action.label}
		</a>
	{/if}
</div>
