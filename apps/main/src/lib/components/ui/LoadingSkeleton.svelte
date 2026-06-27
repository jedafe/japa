<script lang="ts">
	let {
		lines = 3,
		height = '4'
	}: {
		lines?: number;
		height?: string;
	} = $props();
</script>

<div class="animate-pulse space-y-3">
	{#each Array(lines) as _, i}
		<div
			class="rounded"
			style="background-color: var(--color-muted); opacity: 0.2; width: {i === lines - 1 ? '60%' : '100%'}; height: {height}px"
		></div>
	{/each}
</div>
