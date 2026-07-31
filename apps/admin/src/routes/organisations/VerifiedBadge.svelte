<script lang="ts">
	let { value }: { value: unknown } = $props();
	const verified = $derived(Boolean(value));
</script>

<span
	class={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${verified ? 'bg-success/15 text-success dark:text-success' : 'bg-muted/15 text-muted dark:text-muted'}`}
>
	{verified ? 'Yes' : 'No'}
</span>
