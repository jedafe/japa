<script lang="ts">
	import { t } from '$lib/i18n';
	import { FileText, ArrowRight } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const pages = $derived(data.pages);

	function formatDate(iso: string): string {
		try {
			return new Date(iso).toLocaleDateString(undefined, {
				year: 'numeric',
				month: 'short',
				day: 'numeric'
			});
		} catch {
			return '';
		}
	}
</script>

<svelte:head>
	<title>{t('pages_list.title', 'Pages')}</title>
	<meta name="description" content={t('pages_list.description', 'Browse all published pages on JapaConnect.')} />
</svelte:head>

<div class="mx-auto max-w-full px-4 py-6">
	<h1 class="mb-1 text-2xl font-bold text-[var(--color-text)]">{t('pages_list.heading')}</h1>
	<p class="mb-6 text-sm text-[var(--color-muted)]">{t('pages_list.description')}</p>

	{#if pages.length === 0}
		<EmptyState icon={FileText} title={t('pages_list.empty')} />
	{:else}
		<ul class="grid gap-3 sm:grid-cols-2">
			{#each pages as p (p.id)}
				<li>
					<a
						href={`/${p.slug}`}
						class="group flex h-full flex-col rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition hover:border-[var(--color-primary)]/40"
					>
						<div class="mb-2 flex items-center gap-2">
							<FileText size={18} class="text-[var(--color-muted)]" />
							<h2 class="flex-1 text-sm font-medium text-[var(--color-text)]">{p.title}</h2>
						</div>
						{#if p.description}
							<p class="mb-3 line-clamp-2 flex-1 text-sm text-[var(--color-muted)]">{p.description}</p>
						{:else}
							<p class="mb-3 flex-1"></p>
						{/if}
						<div class="flex items-center justify-between text-xs text-[var(--color-muted)]">
							<span>{formatDate(p.created_at)}</span>
							<span class="flex items-center gap-1 text-[var(--color-primary)] opacity-0 transition group-hover:opacity-100">
								{t('pages_list.view')}
								<ArrowRight size={14} />
							</span>
						</div>
					</a>
				</li>
			{/each}
		</ul>
	{/if}
</div>
