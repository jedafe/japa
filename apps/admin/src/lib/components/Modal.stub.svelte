<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		open = $bindable(false),
		title,
		children,
		onConfirm,
		confirmLabel = 'Confirm'
	}: {
		open: boolean;
		title: string;
		children?: Snippet;
		onConfirm?: () => void;
		confirmLabel?: string;
	} = $props();
</script>

{#if open}
	<div data-testid="modal-stub">
		<h2>{title}</h2>
		{#if children}
			{@render children()}
		{/if}
		<button onclick={() => (open = false)}>Cancel</button>
		{#if onConfirm}
			<button onclick={() => { onConfirm(); open = false; }}>{confirmLabel}</button>
		{/if}
	</div>
{/if}
