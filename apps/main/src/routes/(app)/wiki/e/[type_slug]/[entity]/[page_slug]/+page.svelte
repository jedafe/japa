<script lang="ts">
	import { t, siteName } from '$lib/i18n';
	import { tiptapToHtml, type TipTapDoc } from '$lib/tiptap/serialize';
	import { sanitizeContentHtml } from '$lib/utils/safeHtml';
	import { entityPageHref } from '$lib/utils/wikiEntityLinks';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	let page = $derived(data.page);

	// Render the wiki content to safe HTML — same pipeline as the
	// space-bound wiki page (TipTap JSON → HTML → sanitize, Rule 16/§5.1).
	const htmlContent = $derived.by(() => {
		const c = page?.content;
		if (c == null) return '';
		if (typeof c === 'string') return sanitizeContentHtml(c);
		if (typeof c === 'object') {
			return sanitizeContentHtml(tiptapToHtml(c as unknown as TipTapDoc));
		}
		return '';
	});

	// §wiki-links — mode:"related" grid heading: an offering page lists
	// other universities with the same catalog course; a program page
	// lists every university offering it.
	const relatedHeading = $derived(
		page?.kind === 'course'
			? t('wiki.also_offered_at', 'Also offered at')
			: t('wiki.universities_offering', 'Universities offering this program')
	);
</script>

<svelte:head>
	<title>{page.title} — {page.entity_name} | {siteName} Wiki</title>
</svelte:head>

<article class="mx-auto max-w-3xl space-y-4 px-4 py-6">
	<header class="space-y-1">
		<nav class="text-xs text-[var(--color-muted)]">
			<a href="/wiki" class="hover:underline">{t('nav.wiki', 'Wiki')}</a>
			<span> / </span>
			<span class="capitalize">{page.type_slug.replaceAll('_', ' ')}</span>
			<span> / </span>
			<span>{page.entity_name}</span>
		</nav>
		<h1 class="text-2xl font-bold text-[var(--color-text)]">{page.title}</h1>
		<p class="text-sm text-[var(--color-muted)]">
			{page.entity_name} · <span class="capitalize">{page.category.replaceAll('_', ' ')}</span>
			{#if page.lazy}
				<span class="ms-2 rounded bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs">{t('wiki.not_yet_written', 'not yet written')}</span>
			{/if}
		</p>
	</header>

	{#if page.categories && page.categories.length > 1}
		<nav
			class="flex gap-1 overflow-x-auto border-b border-[var(--color-border)] pb-2"
			aria-label={t('wiki.categories', 'Categories')}
		>
			{#each page.categories as cat (cat.slug)}
				{@const active = cat.category === page.category}
				<a
					href={entityPageHref(page.type_slug, page.entity_slug, cat.slug)}
					class="whitespace-nowrap rounded-full px-3 py-1 text-xs font-medium transition-colors {active
						? 'bg-[var(--color-primary)] text-white'
						: 'text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]'}"
				>
					{cat.label}
				</a>
			{/each}
		</nav>
	{/if}


	{#if htmlContent}
		<div class="prose max-w-none">{@html htmlContent}</div>
	{:else}
		<p class="text-sm text-[var(--color-muted)]">{t('wiki.empty_content', 'This page has no content yet.')}</p>
	{/if}

	{#if page.fields && page.fields.fields.length > 0}
		<section class="space-y-2">
			<h2 class="text-sm font-semibold uppercase tracking-wide text-[var(--color-muted)]">
				{page.fields.name}
			</h2>
			<dl class="divide-y divide-[var(--color-border)] rounded-lg border border-[var(--color-border)]">
				{#each page.fields.fields as field}
					<div class="grid grid-cols-3 gap-4 px-4 py-2.5">
						<dt class="text-sm text-[var(--color-muted)]">{field.label}</dt>
						<dd class="col-span-2 text-sm text-[var(--color-text)]">
							{#if field.value != null}
								{typeof field.value === 'object' ? JSON.stringify(field.value) : String(field.value)}
							{:else}
								<span class="italic text-[var(--color-muted)]">—</span>
							{/if}
						</dd>
					</div>
				{/each}
			</dl>
		</section>
	{/if}

	{#if page.related && page.related.items.length > 0}
		<section class="space-y-3">
			<h2 class="text-sm font-semibold uppercase tracking-wide text-[var(--color-muted)]">
				{relatedHeading}
			</h2>
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
				{#each page.related.items as uni (uni.slug)}
					<div class="flex flex-col gap-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3 transition-colors hover:border-[var(--color-primary)]">
						<a
							href="/wiki/{encodeURIComponent(uni.slug)}"
							class="flex items-center gap-2 text-sm font-medium text-[var(--color-text)] hover:text-[var(--color-primary)]"
							data-sveltekit-preload-data="hover"
						>
							{uni.name}
							{#if uni.geo_code}
								<span class="rounded bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10px] uppercase text-[var(--color-muted)]">{uni.geo_code}</span>
							{/if}
						</a>
						{#if uni.offering_slug}
							<a
								href={entityPageHref('course', uni.offering_slug)}
								class="text-xs text-[var(--color-muted)] hover:text-[var(--color-primary)]"
							>
								{t('wiki.view_at_university', 'View at {name}', { name: uni.name })}
							</a>
						{/if}
					</div>
				{/each}
			</div>
			{#if page.related.total > page.related.items.length}
				<p class="text-xs text-[var(--color-muted)]">
					{t('wiki.directory_truncated', 'Showing {shown} of {total} — search to find the rest.', {
						shown: page.related.items.length,
						total: page.related.total
					})}
				</p>
			{/if}
		</section>
	{/if}
</article>
