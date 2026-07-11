<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		class: className = '',
		children
	}: {
		class?: string;
		children?: Snippet;
	} = $props();
</script>

<div
	class="rounded-xl border {className}"
	style="border-color: var(--color-border); background: var(--color-surface)"
>
	{@render children?.()}
</div>
