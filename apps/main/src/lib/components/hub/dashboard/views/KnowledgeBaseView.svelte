<script lang="ts">
	import { t } from '$lib/i18n';
	import DashboardPanel from '$lib/components/hub/DashboardPanel.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { BookOpen } from 'lucide-svelte';

	interface Bookmark {
		id: string;
		title?: string;
		url?: string;
		slug?: string;
	}

	let { data }: { data: unknown } = $props();
	const bookmarks = (data as Bookmark[] | null) ?? [];
</script>

<section class="space-y-4">
	<header>
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('dashboard.nav.knowledge_base')}</h1>
	</header>

	<DashboardPanel titleKey="dashboard.kb.bookmarks">
		{#if bookmarks.length === 0}
			<EmptyState icon={BookOpen} title={t('dashboard.kb.empty_title')} body={t('dashboard.kb.empty_body')} />
		{:else}
			<ul class="space-y-2">
				{#each bookmarks as bm (bm.id)}
					<li>
						<a
							href={bm.url ?? `https://docs.japaconnect.com/docs/${bm.slug ?? bm.id}`}
							class="block truncate rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm text-[var(--color-text)] hover:underline"
						>
							{bm.title ?? bm.id}
						</a>
					</li>
				{/each}
			</ul>
		{/if}
	</DashboardPanel>

	<a
		href="https://docs.japaconnect.com/docs/users"
		class="inline-block rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white"
	>
		{t('dashboard.kb.browse_docs')}
	</a>
</section>
