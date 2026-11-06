<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md (Entity Directory amendment)
	import { t, siteName } from '$lib/i18n';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { ArrowRight, BookOpen } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	let kinds = $derived(data.kinds ?? []);
</script>

<svelte:head>
	<title>{t('wiki.entities.title', 'Entity Wiki')} — {siteName()}</title>
</svelte:head>

<div class="wiki-e-index">
	<header class="head">
		<h1>{t('wiki.entities.title', 'Entity Wiki')}</h1>
		<p class="sub">{t('wiki.entities.subtitle', 'Browse entities by kind — programs, course offerings, banks, embassies, and more.')}</p>
	</header>

	{#if kinds.length === 0}
		<EmptyState icon={BookOpen} title={t('wiki.entities.empty', 'No entity kinds available')} />
	{:else}
		<div class="grid">
			{#each kinds as k (k.kind)}
				<a href="/wiki/e/{k.kind}" class="kind-card">
					<span class="kind-label">{k.label}</span>
					<span class="kind-count">{k.entity_count}</span>
					<ArrowRight size={16} />
				</a>
			{/each}
		</div>
	{/if}
</div>

<style>
	.wiki-e-index {
		max-width: 72rem;
		margin: 0 auto;
		padding: 1.5rem 1rem;
		display: flex;
		flex-direction: column;
		gap: 1.5rem;
	}
	.head h1 {
		font-size: 1.75rem;
		font-weight: 700;
	}
	.sub {
		color: var(--color-muted-foreground, #64748b);
		margin-top: 0.25rem;
		font-size: 0.9375rem;
	}
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
		gap: 0.75rem;
	}
	.kind-card {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 1rem;
		border: 1px solid var(--color-border, #e2e8f0);
		border-radius: 0.75rem;
		text-decoration: none;
		color: inherit;
	}
	.kind-card:hover {
		border-color: var(--color-primary, #2563eb);
	}
	.kind-label {
		font-weight: 600;
		flex: 1;
	}
	.kind-count {
		font-size: 0.8125rem;
		color: var(--color-muted-foreground, #64748b);
	}
</style>
