<script lang="ts">
	import { t } from '$lib/i18n';
	import DashboardPanel from '$lib/components/hub/DashboardPanel.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { learnUrl, docsUrl } from '$lib/appOrigins';
	import { BookOpen } from 'lucide-svelte';
	import type { Bookmark } from '$lib/types';

	let { data }: { data: unknown } = $props();
	const bookmarks = $derived((data as Bookmark[] | null) ?? []);

	/** Build the canonical main-app or cross-app URL for a bookmarked entity. */
	function bookmarkUrl(bm: Bookmark): string {
		const idOrSlug = bm.slug || bm.entity_id;
		switch (bm.entity_type) {
			case 'content':
			case 'blog_post':
				return `/${idOrSlug}`;
			case 'forum_thread':
				return `/forum/${idOrSlug}`;
			case 'space':
				return `/spaces/${idOrSlug}`;
			case 'tag':
				return `/tags/${idOrSlug}`;
			case 'learning_track':
				return learnUrl(`/tracks/${idOrSlug}`);
			default:
				return `/${idOrSlug}`;
		}
	}
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
							href={bookmarkUrl(bm)}
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
		href={docsUrl('/docs')}
		class="inline-block rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white"
	>
		{t('dashboard.kb.browse_docs')}
	</a>
</section>
