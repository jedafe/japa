<script lang="ts">
	let { open = $bindable(false), title = '', children, onConfirm, confirmLabel = 'Confirm' } = $props();
</script>

{#if open}
	<div>
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
