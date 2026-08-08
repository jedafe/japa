<script lang="ts">
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { FileText } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { LandingRenderer, normalizeLegacyBlocks, type Block } from '@japaconnect/landing-blocks';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const pageData = $derived(data.page);

	const title = $derived(
		pageData.meta && typeof pageData.meta === 'object' && 'title' in pageData.meta
			? String(pageData.meta.title)
			: t('page.title', '{name} — JapaConnect').replace('{name}', pageData.title)
	);
	const description = $derived(
		pageData.description ??
			t('page.description', 'A JapaConnect page.')
	);

	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'WebPage',
			name: pageData.title,
			description,
			url: page.url.href
		})
	);

	const hasBlocks = $derived((pageData.blocks ?? []).length > 0);

	// Normalize legacy Page Builder blocks into the new Block[] format for
	// LandingRenderer. This consolidates rendering onto a single path
	// (LandingRenderer) instead of maintaining a separate legacy
	// BlockRenderer. The compat layer maps legacy type names (rich_text →
	// content, dynamic_listing → entity_collection, etc.) and migrates
	// prop shapes.
	const normalizedBlocks = $derived(
		hasBlocks ? normalizeLegacyBlocks(pageData.blocks ?? []) as Block[] : []
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
	{@html `<script type="application/ld+json">${jsonLd}</script>`}
</svelte:head>

{#if !hasBlocks}
	<div class="mx-auto w-full max-w-full p-4">
		<EmptyState icon={FileText} title={pageData.title} />
	</div>
{:else}
	<LandingRenderer blocks={normalizedBlocks} />
{/if}
