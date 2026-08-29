<script lang="ts">
	import { PenSquare, Tag, X } from 'lucide-svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import ForumList from '$lib/components/forum/ForumList.svelte';
	import { compose } from '$lib/components/shell/compose.svelte';
	import { t } from '$lib/i18n';
	import type { CardStyleConfig } from '$lib/feed/cardStyle';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	const cardStyleConfig = $derived((page.data.cardStyleConfig as CardStyleConfig) ?? undefined);

	const title = $derived(t('forum.list_title', 'Forum — JapaConnect'));
	const description = $derived(
		t('forum.list_description', 'Discussions, questions, and answers from the Nigerian diaspora community.')
	);

	function buildUrl(sort: string, status: string) {
		const params = new URLSearchParams();
		if (sort !== 'latest') params.set('sort', sort);
		if (status !== 'all') params.set('status', status);
		if (data.tagSlug) params.set('tag', data.tagSlug);
		return `/forum${params.toString() ? `?${params.toString()}` : ''}`;
	}

	function applyFilter(kind: 'sort' | 'status', value: string) {
		const sort = kind === 'sort' ? value : data.sort;
		const status = kind === 'status' ? value : data.status;
		goto(buildUrl(sort, status));
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<div class="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="text-lg font-semibold text-[var(--color-text)]">{t('forum.heading', 'Forum')}</h1>
		{#if data.authed}
			<button
				type="button"
				onclick={() => compose.openDiscussion()}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				<PenSquare size={14} /> {t('forum.new_discussion', 'New discussion')}
			</button>
		{:else}
			<a
				href="/login?redirect=/forum"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				<PenSquare size={14} /> {t('forum.new_discussion', 'New discussion')}
			</a>
		{/if}
	</div>

	<div class="p-3">
		<div class="mb-3 flex flex-wrap items-center gap-2">
			<div class="flex items-center gap-2 text-xs text-[var(--color-text-subtle)]">
				<label class="font-medium" for="forum-sort">{t('forum.sort', 'Sort')}</label>
				<select
					id="forum-sort"
					value={data.sort}
					onchange={(e) => applyFilter('sort', e.currentTarget.value)}
					class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-xs text-[var(--color-text)]"
				>
					<option value="latest">{t('forum.latest', 'Latest')}</option>
					<option value="top">{t('forum.top', 'Top')}</option>
				</select>
			</div>
			<div class="flex items-center gap-2 text-xs text-[var(--color-text-subtle)]">
				<label class="font-medium" for="forum-status">{t('forum.status', 'Status')}</label>
				<select
					id="forum-status"
					value={data.status}
					onchange={(e) => applyFilter('status', e.currentTarget.value)}
					class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-xs text-[var(--color-text)]"
				>
					<option value="all">{t('forum.all', 'All')}</option>
					<option value="solved">{t('forum.solved', 'Solved')}</option>
					<option value="open">{t('forum.open', 'Open')}</option>
				</select>
			</div>
			{#if data.tagSlug}
				<div class="ml-auto flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-1.5 text-xs text-[var(--color-text-subtle)]">
					<Tag size={12} />
					<span>{data.tagSlug}</span>
					<a href={buildUrl(data.sort, data.status)} class="inline-flex items-center gap-1 text-[var(--color-muted)] hover:text-[var(--color-text)]">
						<X size={11} /> {t('common.clear', 'Clear')}
					</a>
				</div>
			{/if}
		</div>
		<ForumList
			threads={data.threads}
			pageSize={data.pageSize}
			loadError={data.loadError}
			config={cardStyleConfig}
		/>
	</div>
</div>
