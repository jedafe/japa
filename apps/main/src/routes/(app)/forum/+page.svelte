<script lang="ts">
	import { PenSquare, Tag, X } from 'lucide-svelte';
	import { page } from '$app/state';
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
		{#if data.tagSlug}
			<div class="mb-3 flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm text-[var(--color-text-subtle)]">
				<Tag size={14} />
				<span>{t('forum.filtered_by_tag', 'Tagged')}: <span class="font-medium text-[var(--color-text)]">{data.tagSlug}</span></span>
				<a href="/forum" class="ml-auto inline-flex items-center gap-1 text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]">
					<X size={13} /> {t('common.clear', 'Clear')}
				</a>
			</div>
		{/if}
		<ForumList
			threads={data.threads}
			pageSize={data.pageSize}
			loadError={data.loadError}
			config={cardStyleConfig}
		/>
	</div>
</div>
