<script lang="ts">
	import type { PageMetadata } from './structuredData';

	let {
		metadata,
		noindex = false
	}: {
		metadata: PageMetadata;
		noindex?: boolean;
	} = $props();

	const title = $derived(metadata.title || 'JapaConnect');
	const description = $derived(metadata.description || 'Community platform for the Nigerian diaspora');
	const ogImage = $derived(metadata.og_image || '/og-default.png');
	const canonical = $derived(metadata.canonical_url || '');
	const contentType = $derived(metadata.content_type || 'website');
	const publishedAt = $derived(metadata.published_at || '');
	const updatedAt = $derived(metadata.updated_at || '');
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	
	{#if noindex}
		<meta name="robots" content="noindex, nofollow" />
	{:else}
		<meta name="robots" content="index, follow" />
	{/if}

	{#if canonical}
		<link rel="canonical" href={canonical} />
	{/if}

	<!-- Open Graph -->
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content={contentType} />
	<meta property="og:image" content={ogImage} />
	<meta property="og:site_name" content="JapaConnect" />
	
	{#if publishedAt}
		<meta property="article:published_time" content={publishedAt} />
	{/if}
	{#if updatedAt}
		<meta property="article:modified_time" content={updatedAt} />
	{/if}

	<!-- Twitter Card -->
	<meta name="twitter:card" content="summary_large_image" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
	<meta name="twitter:image" content={ogImage} />
</svelte:head>