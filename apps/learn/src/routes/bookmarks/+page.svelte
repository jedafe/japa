<script lang="ts">
	// Bookmarks — CSR-only. Bookmarked lessons/courses for the learner.
	// The backend bookmark endpoint is polymorphic (entity_type = 'learning_track'
	// or 'lesson'); this page fetches /me/bookmarks and filters to learning entities.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { LoadingSkeleton, EmptyState } from '$lib/components/ui';
	import { Bookmark } from 'lucide-svelte';

	interface BookmarkItem {
		id: string;
		entity_type: string;
		entity_id: string;
		title: string;
		url?: string;
		created_at: string;
	}

	let bookmarks = $state<BookmarkItem[]>([]);
	let loading = $state(true);

	$effect(() => {
		(async () => {
			const r = await api.get<BookmarkItem[]>('/me/bookmarks');
			loading = false;
			if (r.data) bookmarks = r.data;
		})();
	});
</script>

<svelte:head>
	<title>{t('learn.bookmarks')} · {t('app.name')}</title>
</svelte:head>

<div class="px-4 py-6 md:px-6 lg:px-8">
	<header class="mb-6">
		<h1 class="flex items-center gap-2 text-2xl font-bold text-[var(--color-text)]">
			<Bookmark size={24} class="text-[var(--color-primary)]" />
			{t('learn.bookmarks')}
		</h1>
	</header>

	{#if loading}
		<div class="space-y-3">
			{#each Array(3) as _}
				<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<LoadingSkeleton class="h-5 w-3/4" />
					<LoadingSkeleton class="mt-2 h-4 w-1/3" />
				</div>
			{/each}
		</div>
	{:else if bookmarks.length === 0}
		<EmptyState title={t('learn.no_bookmarks')} />
	{:else}
		<ul class="space-y-3">
			{#each bookmarks as bm (bm.id)}
				<li class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<a
						href={bm.url ?? `/tracks/${bm.entity_id}`}
						class="block hover:text-[var(--color-primary)]"
					>
						<p class="font-medium text-[var(--color-text)]">{bm.title}</p>
						<p class="mt-1 text-xs text-[var(--color-muted)]">
							{bm.entity_type} · {new Date(bm.created_at).toLocaleDateString()}
						</p>
					</a>
				</li>
			{/each}
		</ul>
	{/if}
</div>
