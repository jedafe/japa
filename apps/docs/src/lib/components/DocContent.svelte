<script lang="ts">
	import type { Snippet } from 'svelte';

	let { children }: { children: Snippet } = $props();
</script>

<div class="prose prose-slate max-w-none dark:prose-invert">
	{@render children()}
</div>
