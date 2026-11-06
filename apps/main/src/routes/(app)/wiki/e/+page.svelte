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
				<div class="kind-wrap">
					<a href="/wiki/e/{k.kind}" class="kind-card">
						<span class="kind-label">{k.label}</span>
						<span class="kind-count">{k.entity_count}</span>
						<ArrowRight size={16} />
					</a>
					{#if k.types?.length}
						<div class="type-chips">
							{#each k.types as ty (ty.slug)}
								<a href="/wiki/e/{ty.slug}" class="chip">{ty.label} <span>{ty.entity_count}</span></a>
							{/each}
						</div>
					{/if}
				</div>
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
	.kind-wrap {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}
	.type-chips {
		display: flex;
		flex-wrap: wrap;
		gap: 0.375rem;
		padding-left: 0.25rem;
	}
	.chip {
		font-size: 0.75rem;
		padding: 0.2rem 0.55rem;
		border: 1px solid var(--color-border, #e2e8f0);
		border-radius: 999px;
		text-decoration: none;
		color: var(--color-muted-foreground, #64748b);
	}
	.chip:hover {
		border-color: var(--color-primary, #2563eb);
		color: var(--color-primary, #2563eb);
	}
	.chip span {
		opacity: 0.7;
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
