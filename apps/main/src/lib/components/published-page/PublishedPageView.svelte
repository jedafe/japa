<script lang="ts">
	/**
	 * PublishedPageView — renders a published Page Builder page. Shared by the
	 * root-level /[slug] route and the legacy /pages/[slug] path.
	 *
	 * Ref: japaconnect-engineering/sections/27-page-builder.md
	 */
	import { page } from '$app/state';
	import { t } from '#lib/i18n/index.js';
	import { FileText } from 'lucide-svelte';
	import { EmptyState } from '#lib/components/ui/index.js';
	import { LandingRenderer, normalizeLegacyBlocks, type Block, type GuestHomeData } from '@japaconnect/landing-blocks';
	import { safeJsonLdScript } from '#lib/utils/safeHtml.js';
	import TocBlock from '#lib/components/toc/TocBlock.svelte';
	import TocFloat from '#lib/components/toc/TocFloat.svelte';
	import { scanDomToc, tocHidden, type TocItem } from '#lib/utils/toc.js';
	import type { PageBuilderPage } from '#lib/types/index.js';

import { ogCardUrl } from '#lib/seo/og.js';
	interface Props {
		page: PageBuilderPage;
		guestHome?: GuestHomeData;
	}

	let { page: pageData, guestHome }: Props = $props();

	const title = $derived(
		pageData.meta && typeof pageData.meta === 'object' && 'title' in pageData.meta
			? String(pageData.meta.title)
			: t('page.title', '{name} — {site}').replace('{name}', pageData.title)
	);
	const description = $derived(
		pageData.description ??
			t('page.description', 'A {site} page.')
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

	const seoOgImage = $derived(ogCardUrl(page.url.origin, pageData?.id ?? 'page'));

	// TOC — the block tree renders through LandingRenderer (no HTML string
	// to transform), so headings are harvested from the live DOM on mount:
	// scanDomToc assigns slugified ids to the rendered h2/h3s. LandingRenderer
	// lazy-mounts below-fold blocks on approach, so a MutationObserver
	// rescans as new sections enter the DOM. The inline block therefore
	// appears client-side only — SSR emits no TOC here.
	// meta.hide_toc / meta.toc === false opts the page out.
	let contentEl = $state<HTMLElement | null>(null);
	let domTocItems = $state<TocItem[]>([]);
	let tocAnchorEl = $state<HTMLElement | null>(null);

	$effect(() => {
		const root = contentEl;
		normalizedBlocks; // rescan when the block list changes
		if (!root || typeof MutationObserver === 'undefined') return;
		let queued = false;
		const scan = () => {
			queued = false;
			domTocItems = scanDomToc(root);
			// Re-seat the TOC host after the hero — covers remounts where the
			// positioning effect below can't see the host go stale.
			const host = tocAnchorEl;
			const hero = root.firstElementChild?.firstElementChild;
			if (host && hero && host.previousElementSibling !== hero) hero.after(host);
		};
		scan();
		const mo = new MutationObserver(() => {
			if (queued) return;
			queued = true;
			queueMicrotask(scan);
		});
		mo.observe(root, { childList: true, subtree: true });
		return () => mo.disconnect();
	});

	// The TOC card belongs right after the hero: .landing-page holds one
	// LazyMount wrapper per top-level block and the first is the page
	// header — move the host div once it's mounted (Svelte keeps the node
	// reference, so relocating it is safe).
	$effect(() => {
		const host = tocAnchorEl;
		const root = contentEl;
		if (!host || !root) return;
		const hero = root.firstElementChild?.firstElementChild;
		if (hero && host.previousElementSibling !== hero) hero.after(host);
	});

	const tocItems = $derived(tocHidden(pageData) ? [] : domTocItems);
	const showToc = $derived(tocItems.length >= 3);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<link rel="canonical" href={`${page.url.origin}${page.url.pathname}`} />
	<meta property="og:url" content={`${page.url.origin}${page.url.pathname}`} />
	<meta property="og:image" content={seoOgImage} />
	<meta name="twitter:card" content="summary_large_image" />
	<meta name="twitter:image" content={seoOgImage} />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
	{@html safeJsonLdScript(jsonLd)}
</svelte:head>

{#if !hasBlocks}
	<div class="mx-auto w-full max-w-full p-4">
		<EmptyState icon={FileText} title={pageData.title} />
	</div>
{:else}
	<!-- Center-shell width — max-w-4xl, matching the /wiki column. -->
	<div class="mx-auto w-full max-w-4xl">
		<!-- Collapsible TOC — collapsed by default; TocFloat's bottom-left
		     FAB takes over once this block scrolls out of view. -->
		{#if showToc}
			<div class="mx-auto max-w-3xl px-4 pt-4" bind:this={tocAnchorEl}>
				<TocBlock items={tocItems} />
			</div>
		{/if}
		<div bind:this={contentEl}>
			<LandingRenderer blocks={normalizedBlocks} {guestHome} />
		</div>
	</div>
{/if}

{#if showToc}
	<TocFloat items={tocItems} watch={tocAnchorEl} />
{/if}
