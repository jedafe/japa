<script lang="ts">
	import type { JsonLdSchema } from './structuredData';

	let { schema }: { schema: JsonLdSchema } = $props();

	const jsonLdString = $derived(JSON.stringify(schema, null, 2).replace(/</g, '\\u003c'));
</script>

<svelte:head>
	<script type="application/ld+json">
		{@html jsonLdString}
	</script>
</svelte:head>