<script lang="ts">
	// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md §11 — Contents tab:
	// the member content manager lives here (moved from /dashboard/content,
	// which now redirects to /studio/contents).
	import { Dialog } from 'bits-ui';
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { i18n } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto, invalidateAll } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { publishContent, archiveContent, deleteContent } from '$lib/api/hub';
	import { optimisticAction } from '$lib/components/hub/optimisticAction';
	import { buildContentQuery } from '$lib/studio/contentQuery';
	import JceSelect from '$lib/components/ui/JceSelect.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { CREATE_ALLOWED_TYPES } from '$lib/components/create/contentTypeRegistry';
import { FileText, Search, Loader2, X } from 'lucide-svelte';

	interface ContentItem {
		id: string;
		slug?: string;
		title?: string;
		content_type?: string;
		status?: string;
		view_count?: number;
		comment_count?: number;
		created_at?: string;
	}

	interface ContentTypeOption {
		value: string;
		label: string;
	}

	interface SelectOption {
		value: string;
		label: string;
		disabled?: boolean;
	}

	interface ContentManagerData {
		items: ContentItem[];
		contentTypes: ContentTypeOption[];
		pageSize: number;
		hasMore: boolean;
	}

	let { data }: { data: unknown } = $props();
	const viewData = $derived(data as ContentManagerData | null);
	// untrack initializer covers the first (SSR) render so the server-rendered
	// list is populated before hydration. The $effect below reconciles `items`
	// with fresh server data on every subsequent load refresh — tab switches
	// (published/drafts/scheduled), type/sort/filter changes, search, and
	// post-mutation invalidateAll() all re-run the server loader and update
	// viewData; without this sync the local $state would stay stale and the
	// list would never reflect the new selection. (§4.3 / §6.1 — $state
	// captured once never re-syncs from a $derived it no longer tracks.)
	let items = $state<ContentItem[]>(untrack(() => viewData?.items ?? []));
	// hasMore + nextOffset track the "Load more" pagination cursor. They reset
	// to the server-provided first-page values on every server refresh (the
	// $effect) and advance as the user loads additional pages client-side.
	let hasMore = $state(untrack(() => viewData?.hasMore ?? false));
	let nextOffset = $state(untrack(() => viewData?.pageSize ?? 20));
	let loadingMore = $state(false);
	let actionError = $state<string | null>(null);
	$effect(() => {
		const vd = viewData;
		items = vd?.items ?? [];
		hasMore = vd?.hasMore ?? false;
		nextOffset = vd?.pageSize ?? 20;
		loadingMore = false;
	});
	const contentTypes = $derived(viewData?.contentTypes ?? []);

	const status = $derived(page.url.searchParams.get('status') ?? 'published');
	const typeFilter = $derived(page.url.searchParams.get('type') ?? '');
	const qParam = $derived(page.url.searchParams.get('q') ?? '');
	const sortParam = $derived(page.url.searchParams.get('sort') ?? 'newest');

	const tabs: { key: string; labelKey: string }[] = [
		{ key: 'published', labelKey: 'dashboard.content.tab_published' },
		{ key: 'draft', labelKey: 'dashboard.content.tab_drafts' },
		{ key: 'in_review', labelKey: 'dashboard.content.tab_in_review' },
		{ key: 'scheduled', labelKey: 'dashboard.content.tab_scheduled' }
	];

	const sortOptions: SelectOption[] = $derived([
		{ value: 'newest', label: t('dashboard.content.sort_newest') },
		{ value: 'oldest', label: t('dashboard.content.sort_oldest') },
		{ value: 'most_viewed', label: t('dashboard.content.sort_most_viewed') }
	]);

	const typeOptions: SelectOption[] = $derived([
		{ value: '', label: t('dashboard.content.all_types') },
		...contentTypes.map((ct) => ({ value: ct.value, label: ct.label }))
	]);

	// Build a querystring preserving every active filter except page_offset
	// (a filter/tab/search change always resets to the first page).
	function filterParams(overrides: Record<string, string | undefined>): URLSearchParams {
		const params = new URLSearchParams(page.url.searchParams);
		params.delete('page_offset');
		for (const [k, v] of Object.entries(overrides)) {
			if (v) params.set(k, v);
			else params.delete(k);
		}
		return params;
	}

	function switchTab(key: string) {
		const params = filterParams(key === 'published' ? { status: undefined } : { status: key });
		goto(`/studio/contents?${params.toString()}`, { keepFocus: true, noScroll: true });
	}

	function setType(type: string) {
		const params = filterParams({ type: type || undefined });
		goto(`/studio/contents?${params.toString()}`, { keepFocus: true, noScroll: true });
	}

	function setSort(sort: string) {
		const params = filterParams({ sort: sort || undefined });
		goto(`/studio/contents?${params.toString()}`, { keepFocus: true, noScroll: true });
	}

	// Debounced search — update the ?q= query param 350ms after the user
	// stops typing so we don't fire a server load per keystroke.
	let searchTimer: ReturnType<typeof setTimeout> | null = null;
	function onSearchInput(value: string) {
		if (searchTimer) clearTimeout(searchTimer);
		searchTimer = setTimeout(() => {
			const params = filterParams({ q: value || undefined });
			goto(`/studio/contents?${params.toString()}`, { keepFocus: true, noScroll: true });
		}, 350);
	}

	// "Load more" — client-side fetch of the next offset page, appended to the
	// existing list. Uses the page_size+1 trick to detect a further page.
	// Ref: §50, api-response-format (offset pagination, { data: [...] }).
	async function loadMore() {
		if (loadingMore || !hasMore) return;
		loadingMore = true;
		actionError = null;
		const pageSize = viewData?.pageSize ?? 20;
		const path = buildContentQuery({
			status: status === 'published' ? undefined : status,
			type: typeFilter || undefined,
			q: qParam || undefined,
			sort: sortParam || undefined,
			pageOffset: nextOffset,
			pageSize: pageSize + 1
		});
		const res = await api.get<ContentItem[]>(path);
		if (res.error) {
			actionError = t('dashboard.content.action_error');
			loadingMore = false;
			return;
		}
		const batch = res.data ?? [];
		hasMore = batch.length > pageSize;
		const page = hasMore ? batch.slice(0, pageSize) : batch;
		items = [...items, ...page];
		nextOffset += pageSize;
		loadingMore = false;
	}

	// Format the content type for display — use the API label if available,
	// otherwise fall back to the raw value.
	function typeLabel(type: string | undefined): string {
		if (!type) return t('dashboard.content.all_types');
		const found = contentTypes.find((ct) => ct.value === type);
		return found?.label ?? type;
	}

	// Format an ISO date string as a locale-aware date.
	function formatDate(iso: string | undefined): string {
		if (!iso) return '—';
		const d = new Date(iso);
		return isNaN(d.getTime()) ? '—' : d.toLocaleDateString(i18n.locale);
	}

	// Route the Edit action to the correct surface by content type.
	// Composer-editable types go to /create; discussions open their public item page.
	function editHref(item: ContentItem): string {
		// Discussions are authored in the forum composer and cannot be edited in
		// /create; open their public item page instead. (FeedCard uses the same
		// /content/{slug} route for discussion-type content.)
		if (item.content_type === 'discussion' && item.slug) {
			return `/content/${item.slug}`;
		}
		if (item.content_type && (CREATE_ALLOWED_TYPES as readonly string[]).includes(item.content_type)) {
			return `/studio/create?type=${item.content_type}&draft=${item.id}`;
		}
		// Fallback for unknown/legacy types — keep the previous behaviour.
		return `/studio/create?draft=${item.id}`;
	}

	let deleteTarget = $state<string | null>(null);

	async function doPublish(id: string) {
		const original = items;
		actionError = null;
		const { ok } = await optimisticAction({
			apply: () => (items = items.map((i) => (i.id === id ? { ...i, status: 'published' } : i))),
			rollback: () => (items = original),
			call: () => publishContent(id)
		});
		if (!ok) {
			actionError = t('dashboard.content.action_error');
		}
		// invalidateAll() re-runs the server loader regardless of which tab is
		// active. Drafts/scheduled fetch /me/drafts and /me/scheduled (not
		// /me/content), so a targeted invalidate('/api/v1/me/content') would
		// no-op on those tabs and the list would never reconcile with server
		// truth after a mutation. (§3.3 / §27 — Rule 16 invalidate.)
		await invalidateAll();
	}

	async function doArchive(id: string) {
		const original = items;
		actionError = null;
		const { ok } = await optimisticAction({
			apply: () => (items = items.filter((i) => i.id !== id)),
			rollback: () => (items = original),
			call: () => archiveContent(id)
		});
		if (!ok) {
			actionError = t('dashboard.content.action_error');
		}
		await invalidateAll();
	}

	async function doDelete() {
		if (!deleteTarget) return;
		const id = deleteTarget;
		const original = items;
		actionError = null;
		deleteTarget = null;
		const { ok } = await optimisticAction({
			apply: () => (items = items.filter((i) => i.id !== id)),
			rollback: () => (items = original),
			call: () => deleteContent(id)
		});
		if (!ok) {
			actionError = t('dashboard.content.action_error');
		}
		await invalidateAll();
	}
</script>

<section class="space-y-4">
	{#if actionError}
		<div
			class="flex items-center justify-between gap-3 rounded-[var(--radius)] bg-[var(--color-danger)]/10 px-3 py-2 text-sm text-[var(--color-danger)]"
			role="alert"
		>
			<span>{actionError}</span>
			<button
				type="button"
				class="shrink-0 hover:opacity-70"
				aria-label={t('common.close', 'Close')}
				onclick={() => (actionError = null)}
			>
				<X class="size-4" />
			</button>
		</div>
	{/if}

	<div class="flex flex-wrap items-center gap-2">
		{#each tabs as tab (tab.key)}
			<button
				type="button"
				class="rounded-[var(--radius)] px-3 py-1.5 text-sm {status === tab.key || (tab.key === 'published' && !page.url.searchParams.get('status'))
					? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)]'
					: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
				onclick={() => switchTab(tab.key)}
			>
				{t(tab.labelKey)}
			</button>
		{/each}
	</div>

	<div class="mt-2 flex flex-wrap items-center gap-2">
		<div class="relative">
			<Search class="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-[var(--color-muted)]" />
			<input
				type="search"
				placeholder={t('dashboard.content.search_placeholder')}
				value={qParam}
				oninput={(e) => onSearchInput(e.currentTarget.value)}
				class="w-44 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-1.5 pl-8 pr-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:outline-none focus:ring-1 focus:ring-[var(--color-border)] sm:w-56"
			/>
		</div>
		<div class="ml-auto flex flex-wrap items-center gap-2">
			<div class="w-40">
				<JceSelect value={typeFilter} options={typeOptions} onchange={(v) => setType(v)} class="w-full" />
			</div>
			<div class="w-40">
				<JceSelect value={sortParam} options={sortOptions} onchange={(v) => setSort(v)} class="w-full" />
			</div>
		</div>
	</div>

	{#if items.length === 0}
		<EmptyState icon={FileText} title={t('dashboard.content.empty_title')} body={t('dashboard.content.empty_body')} />
	{:else}
		<ul class="space-y-2">
			{#each items as item (item.id)}
				<li class="flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
					<div class="min-w-0">
						<p class="truncate text-sm font-medium text-[var(--color-text)]">{item.title ?? t('dashboard.content.untitled')}</p>
						<p class="text-xs text-[var(--color-muted)]">
							{typeLabel(item.content_type)} · {formatDate(item.created_at)}
							{#if item.view_count !== undefined} · {item.view_count} {t('dashboard.content.views')}{/if}
							{#if item.comment_count !== undefined} · {item.comment_count} {t('dashboard.content.comments')}{/if}
						</p>
					</div>
					<div class="ml-3 flex shrink-0 gap-2">
						<a href={editHref(item)} class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]">
							{item.content_type === 'discussion' ? t('common.view', 'View') : t('dashboard.content.edit')}
						</a>
						{#if item.status === 'draft' || item.status === 'scheduled'}
							<button type="button" class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]" onclick={() => doPublish(item.id)}>
								{t('dashboard.content.publish')}
							</button>
						{/if}
						<button type="button" class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]" onclick={() => doArchive(item.id)}>
							{t('dashboard.content.archive')}
						</button>
						<button type="button" class="text-xs text-[var(--color-muted)] hover:text-[var(--color-danger)]" onclick={() => (deleteTarget = item.id)}>
							{t('dashboard.content.delete')}
						</button>
					</div>
				</li>
			{/each}
		</ul>
		{#if hasMore}
			<div class="flex justify-center pt-2">
				<button
					type="button"
					class="inline-flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					onclick={loadMore}
					disabled={loadingMore}
				>
					{#if loadingMore}
						<Loader2 class="size-4 animate-spin" />
					{/if}
					{t('dashboard.content.load_more')}
				</button>
			</div>
		{/if}
	{/if}

	<Dialog.Root open={deleteTarget !== null} onOpenChange={(v) => { if (!v) deleteTarget = null; }}>
		<Dialog.Portal>
			<Dialog.Content class="z-50 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 shadow-lg">
				<Dialog.Title class="mb-2 text-lg font-medium text-[var(--color-text)]">{t('dashboard.content.delete_confirm')}</Dialog.Title>
				<Dialog.Description class="mb-4 text-sm text-[var(--color-muted)]">{t('dashboard.content.delete_confirm_body')}</Dialog.Description>
				<div class="flex justify-end gap-2">
					<Dialog.Close class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)]">{t('common.cancel')}</Dialog.Close>
					<button type="button" class="rounded-[var(--radius)] bg-[var(--color-danger)] px-4 py-1.5 text-sm font-medium text-white" onclick={doDelete}>{t('dashboard.content.delete')}</button>
				</div>
			</Dialog.Content>
		</Dialog.Portal>
	</Dialog.Root>
</section>
