<script lang="ts">
	// Ref: japaconnect-engineering/sections/79-community-story-wall.md
	import { t } from '$lib/i18n';
	import { i18n } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { relativeTime } from '$lib/feed/time';
	import { ArrowLeft, BadgeCheck, MapPin, Calendar, CreditCard, Briefcase, Lightbulb, BookMarked, Quote, Pencil, Trash2, Loader2 } from 'lucide-svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import type { PageData } from './$types';
import { safeJsonLdScript } from '$lib/utils/safeHtml';

	let { data }: { data: PageData } = $props();

	const story = $derived(data.story);
	const isAuthor = $derived(!!story && !!data.currentUserId && story.author_id === data.currentUserId);

	let deleting = $state(false);
	let showDeleteConfirm = $state(false);

	async function confirmDelete() {
		if (!story) return;
		deleting = true;
		try {
			const res = await api.delete(`/stories/${story.id}`);
			if (res.error === null) {
				goto('/stories');
			}
		} catch {
			// stay on page — the error is transient
		} finally {
			deleting = false;
			showDeleteConfirm = false;
		}
	}

	const title = $derived(story ? `${story.title} — ${t('stories.heading', 'Community Stories')}` : t('stories.heading', 'Community Stories'));
	const description = $derived(story?.biggest_challenge ?? story?.title ?? '');

	const jsonLd = $derived(
		story
			? JSON.stringify({
					'@context': 'https://schema.org',
					'@type': 'Article',
					headline: story.title,
					datePublished: story.created_at,
					author: { '@type': 'Person', name: story.author_name },
					url: page.url.href,
					articleBody: story.body,
					...(story?.biggest_challenge ? { description: story.biggest_challenge } : {})
				})
			: ''
	);

	function metaRow(icon: typeof MapPin, label: string, value: string | null | undefined) {
		return { icon, label, value, show: !!value };
	}

	const metaRows = $derived(
		story
			? [
					metaRow(MapPin, t('stories.origin', 'From'), story.origin_space_name),
					metaRow(MapPin, t('stories.destination', 'To'), story.destination_space_name),
					metaRow(Calendar, t('stories.relocation_year', 'Relocation year'), story.relocation_year ? String(story.relocation_year) : null),
					metaRow(CreditCard, t('stories.visa_category', 'Visa category'), story.visa_category),
					metaRow(Briefcase, t('stories.profession', 'Profession'), story.profession)
				].filter((r) => r.show)
			: []
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={story?.title ?? title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="article" />
	<meta property="og:url" content={page.url.href} />
	{#if jsonLd}
		{@html safeJsonLdScript(jsonLd)}
	{/if}
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	{#if data.loadError || !story}
		<div class="p-4">
			<EmptyState
				icon={BookMarked}
				title={t('stories.load_error', 'Could not load story')}
				body={t('stories.load_error_body', 'Please try again later.')}
				action={{ label: t('stories.back_to_wall', 'Back to stories'), href: '/stories' }}
			/>
		</div>
	{:else}
		<div class="p-4">
			<a
				href="/stories"
				class="inline-flex items-center gap-1 text-sm text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]"
			>
				<ArrowLeft size={16} /> {t('stories.back_to_wall', 'Back to stories')}
			</a>
		</div>

		<article class="px-4 pb-8">
			<!-- Title + verified badge + author actions -->
			<div class="flex items-start justify-between gap-2">
				<div class="flex items-start gap-2">
					<h1 class="text-2xl font-bold text-[var(--color-text)]">{story.title}</h1>
					{#if story.verified}
						<BadgeCheck size={22} class="mt-1 shrink-0 text-[var(--color-primary)]" />
					{/if}
				</div>
				{#if isAuthor}
					<div class="flex shrink-0 items-center gap-1">
						<a
							href="/stories/{story.slug}/edit"
							class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							<Pencil size={14} /> {t('stories.edit', 'Edit')}
						</a>
						<button
							type="button"
							onclick={() => (showDeleteConfirm = true)}
							class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-1.5 text-xs font-medium text-red-500 transition-colors hover:bg-red-500/10"
						>
							<Trash2 size={14} /> {t('stories.delete', 'Delete')}
						</button>
					</div>
				{/if}
			</div>

			<!-- Author + date -->
			<div class="mt-3 flex items-center gap-2 text-sm text-[var(--color-muted)]">
				<span class="font-medium text-[var(--color-text-subtle)]">@{story.author_name}</span>
				<span>·</span>
				<span>{relativeTime(story.created_at, new Date(), i18n.locale)}</span>
			</div>

			<!-- Meta grid -->
			{#if metaRows.length > 0}
				<dl class="mt-6 grid gap-3 sm:grid-cols-2">
					{#each metaRows as row (row.label)}
						<div class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2">
							<row.icon size={16} class="shrink-0 text-[var(--color-muted)]" />
							<div class="min-w-0">
								<dt class="text-xs text-[var(--color-muted)]">{row.label}</dt>
								<dd class="truncate text-sm font-medium text-[var(--color-text)]">{row.value}</dd>
							</div>
						</div>
					{/each}
				</dl>
			{/if}

			<!-- Body -->
			{#if story.body}
				<div class="mt-6 whitespace-pre-wrap text-sm leading-relaxed text-[var(--color-text)]">
					{story.body}
				</div>
			{/if}

			<!-- Highlighted meta fields -->
			<div class="mt-8 space-y-4">
				{#if story?.biggest_challenge}
					<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<h2 class="flex items-center gap-2 text-sm font-semibold text-[var(--color-text)]">
							<Lightbulb size={16} class="text-[var(--color-primary)]" />
							{t('stories.biggest_challenge', 'Biggest Challenge')}
						</h2>
						<p class="mt-2 whitespace-pre-wrap text-sm text-[var(--color-text-subtle)]">{story.biggest_challenge}</p>
					</div>
				{/if}

				{#if story?.most_useful_resource}
					<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<h2 class="flex items-center gap-2 text-sm font-semibold text-[var(--color-text)]">
							<BookMarked size={16} class="text-[var(--color-primary)]" />
							{t('stories.most_useful_resource', 'Most Useful Resource')}
						</h2>
						<p class="mt-2 whitespace-pre-wrap text-sm text-[var(--color-text-subtle)]">{story.most_useful_resource}</p>
					</div>
				{/if}

				{#if story?.best_advice}
					<div class="rounded-[var(--radius)] border border-[var(--color-primary)]/30 bg-[var(--color-primary)]/5 p-4">
						<h2 class="flex items-center gap-2 text-sm font-semibold text-[var(--color-text)]">
							<Quote size={16} class="text-[var(--color-primary)]" />
							{t('stories.best_advice', 'Best Advice')}
						</h2>
						<p class="mt-2 whitespace-pre-wrap text-sm text-[var(--color-text-subtle)]">{story.best_advice}</p>
					</div>
				{/if}
			</div>
		</article>

		{#if showDeleteConfirm}
			<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" role="dialog" aria-modal="true">
				<div class="w-full max-w-md rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6">
					<h2 class="text-lg font-semibold text-[var(--color-text)]">{t('stories.delete_confirm_title', 'Delete your story?')}</h2>
					<p class="mt-2 text-sm text-[var(--color-text-subtle)]">{t('stories.delete_confirm_body', 'This will permanently remove your story. This action cannot be undone.')}</p>
					<div class="mt-4 flex justify-end gap-2">
						<button
							type="button"
							onclick={() => (showDeleteConfirm = false)}
							class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							{t('common.cancel', 'Cancel')}
						</button>
						<button
							type="button"
							onclick={confirmDelete}
							disabled={deleting}
							class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-red-500 px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50"
						>
							{#if deleting}
								<Loader2 size={16} class="animate-spin" />
								{t('stories.deleting', 'Deleting…')}
							{:else}
								<Trash2 size={16} />
								{t('stories.delete', 'Delete')}
							{/if}
						</button>
					</div>
				</div>
			</div>
		{/if}
	{/if}
</div>
