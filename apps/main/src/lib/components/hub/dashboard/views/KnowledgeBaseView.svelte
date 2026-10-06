<script lang="ts">
	import { t } from '$lib/i18n';
	import DashboardPanel from '$lib/components/hub/DashboardPanel.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { academyUrl, docsUrl } from '$lib/appOrigins';
	import { BookOpen } from 'lucide-svelte';
	import type { Bookmark } from '$lib/types';

	let { data }: { data: unknown } = $props();
	const bookmarks = $derived((data as Bookmark[] | null) ?? []);

	// academyUrl/docsUrl throw when the cross-app origin env var is unset — guard
	// so a missing env doesn't crash the render.
	function safeCrossUrl(builder: (p: string) => string, path: string): string {
		try { return builder(path); } catch { return '#'; }
	}

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
			// Doc pages live on the Docs app under /docs/{audience}/{slug}; the
			// bookmark row carries the page's audience slug.
			case 'doc_page':
				return safeCrossUrl(docsUrl, `/docs/${bm.audience ?? 'users'}/${idOrSlug}`);
			case 'academy_track':
				return safeCrossUrl(academyUrl, `/tracks/${idOrSlug}`);
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
