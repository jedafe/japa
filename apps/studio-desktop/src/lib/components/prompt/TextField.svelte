<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	interface Props {
		label: string;
		value: string;
		placeholder?: string;
		type?: string;
		oninput?: (value: string) => void;
		hint?: string;
	}

	let { label, value, placeholder, type = 'text', oninput, hint }: Props = $props();

	function handleInput(event: Event) {
		oninput?.((event.currentTarget as HTMLInputElement).value);
	}
</script>

<label class="block text-sm font-medium">
	{label}
	<input
		class="mt-1 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
		{type}
		{value}
		{placeholder}
		oninput={handleInput}
	/>
	{#if hint}<HintIcon text={hint} />{/if}
</label>
