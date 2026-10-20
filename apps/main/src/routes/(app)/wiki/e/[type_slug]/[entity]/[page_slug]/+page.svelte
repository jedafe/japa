<script lang="ts">
	import { t, siteName } from '$lib/i18n';
	import { tiptapToHtml, type TipTapDoc } from '$lib/tiptap/serialize';
	import { sanitizeContentHtml } from '$lib/utils/safeHtml';
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
</script>

<svelte:head>
	<title>{page.title} — {page.entity_name} | {siteName} Wiki</title>
</svelte:head>

<article class="mx-auto max-w-3xl space-y-4 px-4 py-6">
	<header class="space-y-1">
		<nav class="text-xs text-[var(--color-muted)]">
			<a href="/wiki" class="hover:underline">Wiki</a>
			<span> / </span>
			<span class="capitalize">{page.type_slug.replaceAll('_', ' ')}</span>
			<span> / </span>
			<span>{page.entity_name}</span>
		</nav>
		<h1 class="text-2xl font-bold text-[var(--color-text)]">{page.title}</h1>
		<p class="text-sm text-[var(--color-muted)]">
			{page.entity_name} · <span class="capitalize">{page.category.replaceAll('_', ' ')}</span>
			{#if page.lazy}
				<span class="ml-2 rounded bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs">not yet written</span>
			{/if}
		</p>
	</header>

	{#if htmlContent}
		<div class="prose max-w-none">{@html htmlContent}</div>
	{:else}
		<p class="text-sm text-[var(--color-muted)]">This page has no prose yet.</p>
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
</article>
