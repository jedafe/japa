// Ref: japaconnect-engineering/sections/80-destination-wiki.md
<script lang="ts">
	import { t } from '$lib/i18n';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { History, ArrowLeft, Bot } from 'lucide-svelte';
	import type { WikiRevision, WikiRevisionStatus } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	function statusBadge(status: WikiRevisionStatus): { label: string; cls: string } {
		switch (status) {
			case 'approved':
				return { label: t('wiki.revision_approved', 'Approved'), cls: 'bg-green-500/10 text-green-600' };
			case 'rejected':
				return { label: t('wiki.revision_rejected', 'Rejected'), cls: 'bg-red-500/10 text-red-600' };
			default:
				return { label: t('wiki.revision_pending', 'Pending'), cls: 'bg-amber-500/10 text-amber-600' };
		}
	}

	function formatDate(iso: string | null): string {
		if (!iso) return '—';
		try {
			return new Date(iso).toLocaleString();
		} catch {
			return iso;
		}
	}

	const title = $derived(`${t('wiki.history_title', 'Revision history')} — JapaConnect`);
	const description = $derived(t('wiki.history_description'));
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<div class="flex items-center gap-3 border-b border-[var(--color-border)] px-4 py-3">
		<a
			href="/wiki/{data.spaceSlug}/{data.pageSlug}"
			class="inline-flex items-center gap-1 text-xs text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]"
		>
			<ArrowLeft size={14} /> {t('wiki.back_to_index', 'Back to page')}
		</a>
		<h1 class="text-lg font-semibold text-[var(--color-text)]">
			{t('wiki.history_title', 'Revision history')}
		</h1>
	</div>

	<div class="p-4">
		{#if data.loadError}
			<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-4 py-3 text-sm text-[var(--color-text-subtle)]">
				{t('wiki.load_error', 'Wiki could not be loaded.')}
			</div>
		{:else if data.revisions.length === 0}
			<EmptyState
				icon={History}
				title={t('wiki.empty_title', 'No revisions yet')}
				body={t('wiki.history_description', 'All revisions for this wiki page.')}
				action={{ label: t('wiki.back_to_index', 'Back to page'), href: `/wiki/${data.spaceSlug}/${data.pageSlug}` }}
			/>
		{:else}
			<ul class="mx-auto max-w-3xl divide-y divide-[var(--color-border)] rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)]">
				{#each data.revisions as rev (rev.id)}
					{@const badge = statusBadge(rev.status as WikiRevisionStatus)}
					<li class="px-4 py-3">
						<div class="flex flex-wrap items-center gap-2">
							<span class="rounded-full px-2.5 py-0.5 text-xs font-medium {badge.cls}">
								{badge.label}
							</span>
							{#if rev.ai_generated}
								<span class="inline-flex items-center gap-1 rounded-full bg-[var(--color-surface-subtle)] px-2.5 py-0.5 text-xs font-medium text-[var(--color-text-subtle)]">
									<Bot size={12} /> {t('wiki.revision_ai_generated', 'AI-generated')}
								</span>
							{/if}
							<span class="ml-auto text-xs text-[var(--color-muted)]">{formatDate(rev.created_at)}</span>
						</div>

						{#if rev.summary}
							<p class="mt-2 text-sm text-[var(--color-text)]">{rev.summary}</p>
						{/if}

						<dl class="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-[var(--color-muted)]">
							<div>
								<dt class="inline">{t('wiki.submitted_by', 'Submitted by')}: </dt>
								<dd class="inline text-[var(--color-text-subtle)]">{rev.submitted_by}</dd>
							</div>
							{#if rev.reviewed_by}
								<div>
									<dt class="inline">{t('wiki.reviewed_by', 'Reviewed by')}: </dt>
									<dd class="inline text-[var(--color-text-subtle)]">{rev.reviewed_by}</dd>
								</div>
								<div>
									<dt class="inline">{t('wiki.reviewed_by', 'Reviewed')}: </dt>
									<dd class="inline text-[var(--color-text-subtle)]">{formatDate(rev.reviewed_at ?? null)}</dd>
								</div>
							{:else}
								<div>
									<dt class="inline">{t('wiki.reviewed_by', 'Reviewed')}: </dt>
									<dd class="inline text-[var(--color-text-subtle)]">{t('wiki.not_reviewed', 'Not reviewed')}</dd>
								</div>
							{/if}
						</dl>
					</li>
				{/each}
			</ul>
		{/if}
	</div>
</div>
