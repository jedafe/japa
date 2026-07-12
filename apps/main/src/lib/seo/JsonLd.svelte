<script lang="ts">
	import type { JsonLdSchema } from './structuredData';

	let { schema }: { schema: JsonLdSchema } = $props();
</script>

<svelte:head>
	{@html `<script type="application/ld+json">${JSON.stringify(schema, null, 2).replace(/</g, '\\u003c')}</script>`}
</svelte:head>