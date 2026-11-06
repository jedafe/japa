<script lang="ts">
	import SeoHead from '$lib/seo/SeoHead.svelte';
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md (Entity Directory amendment)
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { t, siteName } from '$lib/i18n';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import PaginationControls from '$lib/components/ui/PaginationControls.svelte';
	import { ArrowLeft, ChevronRight, Search, Table2 } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	let items = $derived(data.items ?? []);
	let total = $derived(data.total ?? 0);
	let label = $derived(data.label ?? data.kind);
	let pageCount = $derived(Math.max(1, Math.ceil(total / data.limit)));
	let search = $state(data.q ?? '');

	function applySearch() {
		const url = new URL(page.url);
		if (search) url.searchParams.set('q', search);
		else url.searchParams.delete('q');
		url.searchParams.delete('page');
		goto(url.toString(), { keepFocus: true });
	}

	function setPage(p: number) {
		const url = new URL(page.url);
		url.searchParams.set('page', String(p));
		goto(url.toString());
	}
</script>

<SeoHead title={`${label} — ${t('wiki.entities.title', 'Entity Wiki')} — ${siteName()}`} description={t('wiki.entities.type_description', 'Community-maintained wiki entries for {name} — maintained by members on {site}.').replace('{name}', label)} />

<div class="mx-auto w-full max-w-4xl px-4 py-4 sm:px-6">
	<header class="mt-4">
		<a href="/wiki/e" class="inline-flex items-center gap-1 text-xs font-medium text-[var(--color-muted)] transition-colors hover:text-[var(--color-primary)]">
			<ArrowLeft size={14} /> {t('wiki.entities.title', 'Entity Wiki')}
		</a>
		<h1 class="mt-2 text-2xl font-bold text-[var(--color-text)]">{label}</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)] tabular-nums">
			{t('wiki.entities.count', '{count} entries').replace('{count}', String(total))}
		</p>
	</header>

	{#if total > 0 || search}
		<form class="mt-6 flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 transition-colors focus-within:border-[var(--color-primary)]/60" onsubmit={(e) => { e.preventDefault(); applySearch(); }}>
			<Search size={16} class="shrink-0 text-[var(--color-muted)]" />
			<input
				bind:value={search}
				placeholder={t('wiki.entities.search', 'Search…')}
				class="w-full bg-transparent text-sm text-[var(--color-text)] outline-none placeholder:text-[var(--color-muted)]"
			/>
		</form>
	{/if}

	{#if items.length === 0}
		<div class="mt-8">
			<EmptyState icon={Table2} title={t('wiki.entities.empty_list', 'No entries found')} />
		</div>
	{:else}
		<div class="mt-6 grid grid-cols-1 gap-2 sm:grid-cols-2">
			{#each items as it (it.id)}
				<a
					href={it.href}
					class="group flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3 transition-colors hover:border-[var(--color-primary)]/60 focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary)]"
				>
					<span class="min-w-0 flex-1 truncate text-sm font-medium text-[var(--color-text)] group-hover:text-[var(--color-primary)]">{it.name}</span>
					{#if it.discriminator}
						<span class="shrink-0 rounded-full border border-[var(--color-border)] px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide text-[var(--color-muted)]">
							{it.discriminator}
						</span>
					{/if}
					<ChevronRight size={14} class="shrink-0 text-[var(--color-muted)] transition-all group-hover:translate-x-0.5 group-hover:text-[var(--color-primary)]" />
				</a>
			{/each}
		</div>
		{#if pageCount > 1}
			<div class="mt-6 flex justify-center">
				<PaginationControls page={data.page} totalPages={pageCount} onPageChange={setPage} />
			</div>
		{/if}
	{/if}
</div>
