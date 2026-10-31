<script lang="ts">
	import { t, siteName } from '$lib/i18n';
	import { ArrowLeft, Bot, ChevronRight } from 'lucide-svelte';
	import { entityPageHref } from '$lib/utils/wikiEntityLinks';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	const page = $derived(data.page);
	const backHref = $derived(entityPageHref(page.type_slug, page.entity_slug, page.slug));

	function statusClass(status: string): string {
		if (status === 'approved') return 'bg-green-500/10 text-green-600 dark:text-green-400';
		if (status === 'rejected') return 'bg-red-500/10 text-red-500';
		return 'bg-amber-500/10 text-amber-600 dark:text-amber-400';
	}
</script>

<svelte:head>
	<title>{t('wiki.history', 'History')} — {page.title} | {siteName} Wiki</title>
</svelte:head>

<article class="mx-auto max-w-3xl space-y-4 px-4 py-6">
	<header class="space-y-2">
		<nav class="text-xs text-[var(--color-muted)]">
			<ol class="flex items-center gap-1.5">
				<li><a href="/wiki" class="transition-colors hover:text-[var(--color-text)]">{t('nav.wiki', 'Wiki')}</a></li>
				{#each page.ancestors ?? [] as anc (anc.slug)}
					<li aria-hidden="true"><ChevronRight class="rtl:-scale-x-100" size={12} /></li>
					<li><a href="/wiki/{encodeURIComponent(anc.slug)}" class="truncate transition-colors hover:text-[var(--color-text)]">{anc.name}</a></li>
				{/each}
				<li aria-hidden="true"><ChevronRight class="rtl:-scale-x-100" size={12} /></li>
				<li class="truncate">{page.entity_name}</li>
			</ol>
		</nav>
		<h1 class="text-2xl font-bold text-[var(--color-text)]">{t('wiki.history', 'History')} — {page.title}</h1>
		<a href={backHref} class="inline-flex items-center gap-1 text-sm text-[var(--color-primary)] hover:underline">
			<ArrowLeft class="rtl:-scale-x-100" size={14} />{t('wiki.back_to_page', 'Back to page')}
		</a>
	</header>

	{#if data.revisions.length === 0}
		<p class="text-sm text-[var(--color-muted)]">{t('wiki.no_revisions', 'No revisions yet.')}</p>
	{:else}
		<ul class="divide-y divide-[var(--color-border)] rounded-[12px] border border-[var(--color-border)]">
			{#each data.revisions as rev (rev.id)}
				<li class="space-y-1 px-4 py-3">
					<div class="flex flex-wrap items-center gap-2 text-xs">
						<span class="rounded px-2 py-0.5 font-medium {statusClass(rev.status)}">{rev.status}</span>
						{#if rev.kind}<span class="rounded bg-[var(--color-surface-subtle)] px-2 py-0.5 text-[var(--color-muted)]">{rev.kind}</span>{/if}
						{#if rev.ai_generated}<Bot size={14} class="text-[var(--color-muted)]" aria-label={t('wiki.ai_generated', 'AI generated')} />{/if}
						<time class="text-[var(--color-muted)]">{new Date(rev.created_at).toLocaleDateString()}</time>
					</div>
					{#if rev.summary}<p class="text-sm text-[var(--color-text)]">{rev.summary}</p>{/if}
					{#if rev.submitted_by}<p class="text-xs text-[var(--color-muted)]">{t('wiki.submitted_by', 'by {name}', { name: rev.submitted_by })}</p>{/if}
				</li>
			{/each}
		</ul>
	{/if}
</article>
