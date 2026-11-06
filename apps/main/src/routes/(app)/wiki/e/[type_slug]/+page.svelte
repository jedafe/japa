<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md (Entity Directory amendment)
	import { t, siteName } from '$lib/i18n';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import PaginationControls from '$lib/components/ui/PaginationControls.svelte';
	import { ArrowLeft, Search, Table2 } from 'lucide-svelte';
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

<svelte:head>
	<title>{label} — {t('wiki.entities.title', 'Entity Wiki')} — {siteName()}</title>
</svelte:head>

<div class="wiki-e-list">
	<header class="head">
		<a href="/wiki/e" class="back"><ArrowLeft size={14} /> {t('wiki.entities.title', 'Entity Wiki')}</a>
		<h1>{label}</h1>
		<p class="sub">{total} {t('wiki.entities.items', 'entries')}</p>
	</header>

	<form class="search" onsubmit={(e) => { e.preventDefault(); applySearch(); }}>
		<Search size={16} />
		<input bind:value={search} placeholder={t('wiki.entities.search', 'Search…')} />
	</form>

	{#if items.length === 0}
		<EmptyState icon={Table2} title={t('wiki.entities.empty_list', 'No entries found')} />
	{:else}
		<div class="grid">
			{#each items as it (it.id)}
				<a href={it.href} class="item-card">
					<span class="item-name">{it.name}</span>
					{#if it.discriminator}
						<span class="item-disc">{it.discriminator}</span>
					{/if}
				</a>
			{/each}
		</div>
		{#if pageCount > 1}
			<PaginationControls page={data.page} totalPages={pageCount} onPageChange={setPage} />
		{/if}
	{/if}
</div>

<style>
	.wiki-e-list {
		max-width: 72rem;
		margin: 0 auto;
		padding: 1.5rem 1rem;
		display: flex;
		flex-direction: column;
		gap: 1.25rem;
	}
	.head h1 {
		font-size: 1.75rem;
		font-weight: 700;
	}
	.back {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		font-size: 0.8125rem;
		color: var(--color-muted-foreground, #64748b);
		text-decoration: none;
		margin-bottom: 0.25rem;
	}
	.sub {
		color: var(--color-muted-foreground, #64748b);
		font-size: 0.875rem;
		margin-top: 0.25rem;
	}
	.search {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		border: 1px solid var(--color-border, #e2e8f0);
		border-radius: 0.5rem;
		padding: 0.5rem 0.75rem;
		max-width: 24rem;
	}
	.search input {
		border: none;
		outline: none;
		background: transparent;
		flex: 1;
		font-size: 0.9375rem;
	}
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
		gap: 0.75rem;
	}
	.item-card {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		padding: 0.875rem;
		border: 1px solid var(--color-border, #e2e8f0);
		border-radius: 0.75rem;
		text-decoration: none;
		color: inherit;
	}
	.item-card:hover {
		border-color: var(--color-primary, #2563eb);
	}
	.item-name {
		font-weight: 600;
		font-size: 0.9375rem;
	}
	.item-disc {
		font-size: 0.75rem;
		color: var(--color-muted-foreground, #64748b);
		text-transform: capitalize;
	}
</style>
