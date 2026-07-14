<script lang="ts">
	let {
		title,
		description
	}: {
		title: string;
		description?: string;
	} = $props();
</script>

<div class="mb-6">
	<h1 class="text-foreground text-2xl font-bold tracking-tight">{title}</h1>
	{#if description}
		<p class="text-muted-foreground mt-1 text-sm">{description}</p>
	{/if}
</div>
