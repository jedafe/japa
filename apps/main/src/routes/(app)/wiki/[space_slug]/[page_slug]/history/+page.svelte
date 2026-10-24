<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md
	import { t, siteName} from '$lib/i18n';
	import { goto } from '$app/navigation';
	import { page as pageState } from '$app/state';
	import AppealButton from '$lib/components/appeals/AppealButton.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import { History, ArrowLeft, Bot, ChevronLeft, ChevronRight, Table2 } from 'lucide-svelte';
	import type { WikiRevision, WikiRevisionStatus } from '$lib/types';
	import type { PageData } from './$types';
	import { tiptapToText, type TipTapDoc } from '$lib/tiptap/serialize';
	import SetSidebar from '$lib/components/shell/SetSidebar.svelte';
	import WikiSidebar from '$lib/components/wiki/WikiSidebar.svelte';

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

	function excerpt(content: unknown, max = 200): string {
		if (!content || typeof content !== 'object') return '';
		const text = tiptapToText(content as unknown as TipTapDoc);
		if (text.length <= max) return text;
		return text.slice(0, max).trimEnd() + '…';
	}

	const total = $derived(data.meta?.total ?? data.revisions.length);
	const page = $derived(data.meta?.page ?? 1);
	const limit = $derived(Math.max(1, data.meta?.limit ?? 20));
	const totalPages = $derived(Math.max(1, Math.ceil(total / limit)));
	const currentPage = $derived(Math.min(totalPages, Math.max(1, page)));

	// Backend default page size (parsePageParams) — the ?limit= param is
	// only written to the URL when it differs from the default.
	const DEFAULT_LIMIT = 20;
	const basePath = $derived(
		`/wiki/${encodeURIComponent(data.spaceSlug)}/${encodeURIComponent(data.pageSlug)}/history`
	);
	function pageHref(p: number): string {
		const usp = new URLSearchParams();
		if (p > 1) usp.set('page', String(p));
		if (limit !== DEFAULT_LIMIT) usp.set('limit', String(limit));
		const qs = usp.toString();
		return qs ? `${basePath}?${qs}` : basePath;
	}

	const limitOptions = $derived(
		['10', '20', '50', '100'].map((v) => ({
			value: v,
			label: t('studio.moderation.per_page', '{n} / page').replace('{n}', v)
		}))
	);
	function onLimitChange(v: string) {
		const n = parseInt(v, 10);
		if (!Number.isFinite(n) || n < 1) return;
		const usp = new URLSearchParams();
		if (n !== DEFAULT_LIMIT) usp.set('limit', String(n));
		const qs = usp.toString();
		goto(qs ? `${basePath}?${qs}` : basePath);
	}

	const title = $derived(`${t('wiki.history_title', 'Revision history')} — ${siteName()}`);
	const description = $derived(t('wiki.history_description'));
	const wikiEnabled = $derived(data.features?.['wiki.enabled'] !== false);
	const spaceName = $derived(data.spaceDetail?.space?.name ?? data.index[0]?.space_name ?? data.spaceSlug);
	const spaceId = $derived(data.spaceDetail?.space?.id ?? data.index[0]?.space_id ?? '');
	const spaceLevel = $derived(data.spaceDetail?.space?.level ?? '');

	function kindBadge(kind: string | undefined): { label: string; cls: string } | null {
		if (kind === 'fields') {
			return { label: t('wiki.kind_fields', 'Fields'), cls: 'bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]' };
		}
		if (kind === 'mixed') {
			return { label: t('wiki.kind_mixed', 'Content + fields'), cls: 'bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]' };
		}
		return null;
	}

	function opsCount(rev: WikiRevision): number {
		return Array.isArray(rev.ops) ? rev.ops.length : 0;
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
</svelte:head>

<SetSidebar>
	<WikiSidebar
		spaceSlug={data.spaceSlug}
		{spaceName}
		{spaceId}
		{spaceLevel}
		pages={data.index ?? []}
		currentPageSlug={data.pageSlug}
		wikiOnly={data.spaceDetail?.wiki_only ?? false}
	/>
</SetSidebar>

<div class="mx-auto w-full max-w-full">
	<div class="flex flex-col items-start gap-1 border-b border-[var(--color-border)] px-4 py-3" style="margin-inline: max(1rem, calc((100% - 48rem)/2))">
		<a
			href="/wiki/{data.spaceSlug}/{data.pageSlug}"
			class="inline-flex items-center gap-1 text-xs text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]"
		>
			<ArrowLeft class="rtl:-scale-x-100" size={14} /> {t('wiki.back_to_index', 'Back to page')}
		</a>
		<h1 class="text-lg font-semibold text-[var(--color-text)]">
			{t('wiki.history_title', 'Revision history')}
		</h1>
	</div>

	<div class="mx-auto w-full max-w-3xl p-4">
		{#if !wikiEnabled && data.loadError}
			<EmptyState
				icon={History}
				title={t('wiki.coming_soon', 'Wiki is not enabled for this destination')}
				body={t('wiki.coming_soon_body', 'Wiki is currently disabled. Please check back later.')}
				action={{ label: t('wiki.back_to_index', 'Back to page'), href: `/wiki/${data.spaceSlug}/${data.pageSlug}` }}
			/>
		{:else if data.loadError}
			<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-4 py-3 text-sm text-[var(--color-text-subtle)]">
				{t('wiki.load_error', 'Wiki could not be loaded.')}
			</div>
		{:else if data.revisions.length === 0}
			<EmptyState
				icon={History}
				title={t('wiki.history_empty_title', 'No revisions yet')}
				body={t('wiki.history_empty_body', 'No edits have been submitted for this page yet.')}
				action={{ label: t('wiki.back_to_index', 'Back to page'), href: `/wiki/${data.spaceSlug}/${data.pageSlug}` }}
			/>
		{:else}
			<ul class="mx-auto max-w-3xl divide-y divide-[var(--color-border)] rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)]">
				{#each data.revisions as rev (rev.id)}
					{@const badge = statusBadge(rev.status as WikiRevisionStatus)}
					{@const kind = kindBadge(rev.kind)}
					<li class="px-4 py-3">
						<div class="flex flex-wrap items-center gap-2">
							<span class="rounded-full px-2.5 py-0.5 text-xs font-medium {badge.cls}">
								{badge.label}
							</span>
							{#if kind}
								<span class="inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium {kind.cls}">
									<Table2 size={12} /> {kind.label}
								</span>
							{/if}
							{#if rev.ai_generated}
								<span class="inline-flex items-center gap-1 rounded-full bg-[var(--color-surface-subtle)] px-2.5 py-0.5 text-xs font-medium text-[var(--color-text-subtle)]">
									<Bot size={12} /> {t('wiki.revision_ai_generated', 'AI-generated')}
								</span>
							{/if}
							<span class="ms-auto text-xs text-[var(--color-muted)]">{formatDate(rev.created_at)}</span>
						</div>

						{#if rev.summary}
							<p class="mt-2 text-sm text-[var(--color-text)]">{rev.summary}</p>
						{/if}

						{#if rev.content}
							<p class="mt-2 text-xs leading-relaxed text-[var(--color-text-subtle)]">
								{excerpt(rev.content)}
							</p>
						{:else if opsCount(rev) > 0}
							<p class="mt-2 text-xs leading-relaxed text-[var(--color-text-subtle)]">
								{t('wiki.ops_count', '{count} field change(s)', { count: opsCount(rev) })}
							</p>
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
									<dt class="inline">{t('wiki.reviewed_at', 'Reviewed')}: </dt>
									<dd class="inline text-[var(--color-text-subtle)]">{formatDate(rev.reviewed_at ?? null)}</dd>
								</div>
							{:else}
								<div>
									<dt class="inline">{t('wiki.reviewed_at', 'Reviewed')}: </dt>
									<dd class="inline text-[var(--color-text-subtle)]">{t('wiki.not_reviewed', 'Not reviewed')}</dd>
								</div>
							{/if}
						</dl>

						{#if rev.status === 'rejected' && rev.submitted_by && rev.submitted_by === pageState.data.user?.id}
							<div class="mt-2 flex justify-end">
								<AppealButton
									entityType="wiki_revision"
									entityId={rev.id}
									label={t('appeals.appeal_rejection', 'Appeal rejection')}
									class="!px-3 !py-1.5 text-xs"
								/>
							</div>
						{/if}
					</li>
				{/each}
			</ul>

			{#if totalPages > 1 || total > 10}
				<div class="mx-auto mt-4 flex max-w-3xl flex-wrap items-center justify-between gap-2">
					{#if totalPages > 1}
						<nav class="flex flex-1 items-center justify-between gap-2" aria-label={t('wiki.history_pagination', 'Revision history pagination')}>
							<a
								href={pageHref(currentPage - 1)}
								class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)] transition hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
								class:pointer-events-none={currentPage <= 1}
								class:opacity-50={currentPage <= 1}
								tabindex={currentPage <= 1 ? -1 : 0}
							>
								<ChevronLeft class="rtl:-scale-x-100" size={14} /> {t('common.previous', 'Previous')}
							</a>
							<span class="text-sm text-[var(--color-muted)]">
								{t('wiki.page_x_of_y', 'Page {page} of {total}')
									.replace('{page}', String(currentPage))
									.replace('{total}', String(totalPages))}
							</span>
							<a
								href={pageHref(currentPage + 1)}
								class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)] transition hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
								class:pointer-events-none={currentPage >= totalPages}
								class:opacity-50={currentPage >= totalPages}
								tabindex={currentPage >= totalPages ? -1 : 0}
							>
								{t('common.next', 'Next')} <ChevronRight class="rtl:-scale-x-100" size={14} />
							</a>
						</nav>
					{:else}
						<span></span>
					{/if}
					<div class="w-28 shrink-0">
						<AutocompleteSelect
							value={String(limit)}
							options={limitOptions}
							onValueChange={onLimitChange}
							ariaLabel={t('profile.pager.page_size', 'Per page')}
						/>
					</div>
				</div>
			{/if}
		{/if}
	</div>
</div>
