<script lang="ts">
	import clsx from 'clsx';
	import { untrack } from 'svelte';
	import { MessageSquare, ArrowUp, Eye, TriangleAlert } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { t } from '$lib/i18n';
	import type { FeedItem } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// Tabs + sort are the center-column structure (ADR-058). Selection is visual
	// for now; wiring each tab/sort to its own feed query is the next feed task.
	const tabs = $derived(
		data.authed
			? [
					{ id: 'following', label: t('feed.tab.following') },
					{ id: 'for_you', label: t('feed.tab.for_you') }
				]
			: [{ id: 'explore', label: 'Explore' }]
	);
	const sorts = [
		{ id: 'relevant', label: t('sort.relevant') },
		{ id: 'latest', label: t('sort.latest') },
		{ id: 'popular', label: t('sort.popular') },
		{ id: 'rising', label: t('sort.rising') }
	];

	let activeTab = $state(untrack(() => (data.authed ? 'for_you' : 'explore')));
	let activeSort = $state('relevant');

	const typeLabels: Record<string, string> = {
		blog_post: 'Blog',
		discussion: 'Discussion',
		question: 'Question',
		forum_thread: 'Thread',
		comparison: 'Comparison'
	};
	const typeLabel = (ct: string) => typeLabels[ct] ?? ct.replace(/_/g, ' ');

	function href(item: FeedItem): string {
		if (item.content_type === 'blog_post') return `/blog/${item.slug}`;
		return `/content/${item.slug}`;
	}
</script>

<div class="mx-auto w-full max-w-2xl">
	<!-- Tab bar -->
	<div
		class="sticky top-0 z-10 flex gap-1 border-b border-[var(--color-border)] bg-[var(--color-bg)]/90 px-3 backdrop-blur"
		style="min-height: var(--nav-height)"
	>
		{#each tabs as tab (tab.id)}
			<button
				type="button"
				onclick={() => (activeTab = tab.id)}
				class={clsx(
					'relative px-4 text-sm font-medium transition-colors',
					activeTab === tab.id
						? 'text-[var(--color-text)]'
						: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'
				)}
			>
				{tab.label}
				{#if activeTab === tab.id}
					<span
						class="absolute inset-x-3 -bottom-px h-0.5 rounded-full bg-[var(--color-primary)]"
					></span>
				{/if}
			</button>
		{/each}
	</div>

	<!-- Sort bar -->
	<div
		class="sticky z-10 flex items-center gap-2 bg-[var(--color-bg)]/90 px-4 py-2 backdrop-blur"
		style="top: var(--nav-height)"
	>
		{#each sorts as sort (sort.id)}
			<button
				type="button"
				onclick={() => (activeSort = sort.id)}
				class={clsx(
					'rounded-full px-3 py-1 text-xs font-medium transition-colors',
					activeSort === sort.id
						? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)]'
						: 'text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]'
				)}
			>
				{sort.label}
			</button>
		{/each}
	</div>

	<!-- Feed -->
	{#if data.loadError}
		<EmptyState
			icon={TriangleAlert}
			title={t('error.500_title')}
			body={t('error.500_body')}
		/>
	{:else if data.items.length === 0}
		<EmptyState title={t('empty.default_title')} body={t('empty.default_body')} />
	{:else}
		<ul class="divide-y divide-[var(--color-border)]">
			{#each data.items as item (item.id)}
				<li>
					<a href={href(item)} class="block px-4 py-4 hover:bg-[var(--color-surface-subtle)]/50">
						<div class="mb-1 flex items-center gap-2 text-xs text-[var(--color-muted)]">
							<span
								class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 font-medium text-[var(--color-text-subtle)]"
								>{typeLabel(item.content_type)}</span
							>
							{#if item.author_name}<span>{item.author_name}</span>{/if}
						</div>
						<h2 class="text-base font-medium text-[var(--color-text)]">{item.title}</h2>
						{#if item.excerpt}
							<p class="mt-1 line-clamp-2 text-sm text-[var(--color-text-subtle)]">
								{item.excerpt}
							</p>
						{/if}
						<div class="mt-2 flex items-center gap-4 text-xs text-[var(--color-muted)]">
							<span class="flex items-center gap-1"><ArrowUp size={14} /> {item.vote_score ?? 0}</span>
							<span class="flex items-center gap-1"
								><MessageSquare size={14} /> {item.comment_count ?? 0}</span
							>
							<span class="flex items-center gap-1"><Eye size={14} /> {item.view_count ?? 0}</span>
						</div>
					</a>
				</li>
			{/each}
		</ul>
	{/if}
</div>
