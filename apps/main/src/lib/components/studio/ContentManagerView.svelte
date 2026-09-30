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
	import Autocomplete, { type AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { CREATE_ALLOWED_TYPES } from '$lib/components/create/contentTypeRegistry';
	import { contentHref } from '$lib/utils/contentLink';
	import { FileText, Search, Loader2, X } from 'lucide-svelte';

	interface ContentItem {
		id: string;
		slug?: string;
		title?: string;
		content_type?: string;
		status?: string;
		view_count?: number;
		comment_count?: number;
		publish_at?: string | null;
		created_at?: string;
	}

	interface ContentTypeOption {
		value: string;
		label: string;
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
	// actingId guards the row actions against double-submit and concurrent
	// mutations — a second click while a publish/archive/delete is in flight
	// would fire a duplicate API call.
	let actingId = $state<string | null>(null);
	// Bulk selection — every row is selectable (every status has at least
	// one applicable action); ids leaving the list are pruned on refresh.
	let selectedIds = $state<Set<string>>(new Set());
	let bulkBusy = $state(false);
	// listEpoch is bumped on every server refresh so an in-flight loadMore
	// response can't append stale rows to a freshly filtered/reset list.
	let listEpoch = 0;
	$effect(() => {
		const vd = viewData;
		const next = vd?.items ?? [];
		items = next;
		hasMore = vd?.hasMore ?? false;
		nextOffset = vd?.pageSize ?? 20;
		loadingMore = false;
		actingId = null;
		listEpoch++;
		// Read `next`, not the `items` signal — reading a signal this effect
		// also writes would re-schedule it forever.
		const visible = new Set(next.map((i) => i.id));
		const pruned = [...selectedIds].filter((id) => visible.has(id));
		if (pruned.length !== selectedIds.size) selectedIds = new Set(pruned);
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
		{ key: 'scheduled', labelKey: 'dashboard.content.tab_scheduled' },
		{ key: 'archived', labelKey: 'dashboard.content.tab_archived' }
	];

	// Autocomplete option lists — clearing the chip resets the filter (sort
	// falls back to 'newest', the default). Both lists are small and fixed,
	// so the fetchers filter locally with no API call.
	const sortItems: AutocompleteItem[] = $derived([
		{ id: 'newest', label: t('dashboard.content.sort_newest') },
		{ id: 'oldest', label: t('dashboard.content.sort_oldest') },
		{ id: 'most_viewed', label: t('dashboard.content.sort_most_viewed') }
	]);

	const typeItems: AutocompleteItem[] = $derived(
		contentTypes.map((ct) => ({ id: ct.value, label: ct.label }))
	);

	function localFetcher(list: AutocompleteItem[]) {
		return async (q: string) => {
			const needle = q.toLowerCase();
			return list.filter((i) => i.label.toLowerCase().includes(needle));
		};
	}

	const selectedTypeLabel = $derived(
		typeItems.find((i) => i.id === typeFilter)?.label ?? (typeFilter || null)
	);
	const selectedSortLabel = $derived(
		sortItems.find((i) => i.id === sortParam)?.label ?? sortParam
	);

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
		const epoch = listEpoch;
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
		// A server refresh (filter/tab/mutation invalidateAll) may have reset
		// the list while this request was in flight — drop the stale batch.
		if (epoch !== listEpoch) return;
		if (res.error) {
			actionError = t('dashboard.content.action_error');
			loadingMore = false;
			return;
		}
		const batch = res.data ?? [];
		hasMore = batch.length > pageSize;
		const slice = hasMore ? batch.slice(0, pageSize) : batch;
		items = [...items, ...slice];
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
	function formatDate(iso: string | undefined | null): string {
		if (!iso) return '—';
		const d = new Date(iso);
		return isNaN(d.getTime()) ? '—' : d.toLocaleDateString(i18n.locale);
	}

	// Date + time — used for publish_at on the Scheduled tab.
	function formatDateTime(iso: string | undefined | null): string {
		if (!iso) return '—';
		const d = new Date(iso);
		return isNaN(d.getTime()) ? '—' : d.toLocaleString(i18n.locale, { dateStyle: 'medium', timeStyle: 'short' });
	}

	// Route the row's primary link to the correct surface by content type.
	// Only composer-supported types may open /studio/create — the composer
	// clamps an unknown content_type to its first allowed type, so handing it
	// a job listing / event / page / etc. would silently rewrite the type on
	// the next save. Non-composer types link to their canonical public page
	// (same mapping the feed uses) — but only when the item is published:
	// the public detail routes filter on status='published', so a draft,
	// scheduled, in-review, or archived row would link to a 404. Those rows
	// get no link at all (non-composable types have no other member surface).
	function isComposerEditable(item: ContentItem): boolean {
		return !!item.content_type && (CREATE_ALLOWED_TYPES as readonly string[]).includes(item.content_type);
	}

	function itemHref(item: ContentItem): string | null {
		if (isComposerEditable(item)) {
			return `/studio/create?type=${item.content_type}&draft=${item.id}`;
		}
		if (item.slug && item.status === 'published') {
			return contentHref({ slug: item.slug, content_type: item.content_type });
		}
		return null;
	}

	// Bulk selection — the eligible subset is computed per action so a mixed
	// selection (e.g. drafts + archived) can still run Archive on everything
	// or Publish only on the publishable rows.
	const selectedItems = $derived(items.filter((i) => selectedIds.has(i.id)));
	const allSelected = $derived(items.length > 0 && items.every((i) => selectedIds.has(i.id)));
	const publishableIds = $derived(
		selectedItems
			.filter((i) => i.status === 'draft' || i.status === 'scheduled' || i.status === 'archived')
			.map((i) => i.id)
	);
	const archivableIds = $derived(
		selectedItems.filter((i) => i.status !== 'archived').map((i) => i.id)
	);
	const publishLabel = $derived(
		publishableIds.length > 0 &&
			publishableIds.every((id) => selectedItems.find((i) => i.id === id)?.status === 'archived')
			? t('dashboard.content.restore', 'Restore')
			: t('dashboard.content.publish')
	);

	function toggleSelect(id: string) {
		const next = new Set(selectedIds);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		selectedIds = next;
	}

	function toggleSelectAll() {
		selectedIds = allSelected ? new Set() : new Set(items.map((i) => i.id));
	}

	let deleteIds = $state<string[] | null>(null);

	// Runs one lifecycle call per selected row — there is no /me bulk
	// endpoint, and per-row calls keep the backend's ownership + status
	// checks intact. Partial failures are reported as a summary; the list
	// re-syncs via invalidateAll() either way (Rule 16).
	async function doBulk(kind: 'publish' | 'archive' | 'delete', ids: string[]) {
		if (bulkBusy || actingId || ids.length === 0) return;
		bulkBusy = true;
		actionError = null;
		let failed = 0;
		let firstMsg = '';
		for (const id of ids) {
			const res =
				kind === 'publish'
					? await publishContent(id)
					: kind === 'archive'
						? await archiveContent(id)
						: await deleteContent(id);
			if (res.error) {
				failed++;
				if (!firstMsg) firstMsg = res.error.message || res.error.code;
			}
		}
		bulkBusy = false;
		if (failed === ids.length) {
			actionError = firstMsg || t('dashboard.content.action_error');
		} else if (failed > 0) {
			actionError = t(
				'dashboard.content.bulk_partial',
				'{done} of {total} done — {failed} failed',
				{ done: ids.length - failed, total: ids.length, failed }
			);
		}
		await invalidateAll();
	}

	async function doPublish(id: string) {
		if (actingId || bulkBusy) return;
		actingId = id;
		const original = items;
		actionError = null;
		const { ok, error } = await optimisticAction({
			apply: () => (items = items.map((i) => (i.id === id ? { ...i, status: 'published' } : i))),
			rollback: () => (items = original),
			call: () => publishContent(id)
		});
		if (!ok) {
			// Surface the API message (e.g. missing_primary_space) — a bare
			// "action failed" leaves the user guessing why publish refused.
			actionError = error?.message || t('dashboard.content.action_error');
			actingId = null;
		}
		// invalidateAll() re-runs the server loader regardless of which tab is
		// active. Drafts/scheduled fetch /me/drafts and /me/scheduled (not
		// /me/content), so a targeted invalidate('/api/v1/me/content') would
		// no-op on those tabs and the list would never reconcile with server
		// truth after a mutation. (§3.3 / §27 — Rule 16 invalidate.)
		await invalidateAll();
	}

	async function doArchive(id: string) {
		if (actingId || bulkBusy) return;
		actingId = id;
		const original = items;
		actionError = null;
		const { ok, error } = await optimisticAction({
			apply: () => (items = items.filter((i) => i.id !== id)),
			rollback: () => (items = original),
			call: () => archiveContent(id)
		});
		if (!ok) {
			actionError = error?.message || t('dashboard.content.action_error');
			actingId = null;
		}
		await invalidateAll();
	}

	async function doDelete() {
		if (!deleteIds || deleteIds.length === 0 || actingId || bulkBusy) return;
		const ids = deleteIds;
		deleteIds = null;
		if (ids.length > 1) {
			await doBulk('delete', ids);
			return;
		}
		const id = ids[0];
		actingId = id;
		const original = items;
		actionError = null;
		const { ok, error } = await optimisticAction({
			apply: () => (items = items.filter((i) => i.id !== id)),
			rollback: () => (items = original),
			call: () => deleteContent(id)
		});
		if (!ok) {
			actionError = error?.message || t('dashboard.content.action_error');
			actingId = null;
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
				<Autocomplete
					placeholder={t('dashboard.content.all_types')}
					emptyMessage={t('dashboard.content.no_matches', 'No matches')}
					fetcher={localFetcher(typeItems)}
					initialFetcher={async () => typeItems}
					selectedId={typeFilter || null}
					selectedLabel={selectedTypeLabel}
					onSelect={(i) => setType(i.id)}
					onClear={() => setType('')}
					class="w-full"
				/>
			</div>
			<div class="w-40">
				<Autocomplete
					placeholder={t('dashboard.content.sort_label', 'Sort')}
					emptyMessage={t('dashboard.content.no_matches', 'No matches')}
					fetcher={localFetcher(sortItems)}
					initialFetcher={async () => sortItems}
					selectedId={sortParam}
					selectedLabel={selectedSortLabel}
					onSelect={(i) => setSort(i.id)}
					onClear={() => setSort('')}
					class="w-full"
				/>
			</div>
		</div>
	</div>

	{#if items.length === 0}
		<EmptyState icon={FileText} title={t('dashboard.content.empty_title')} body={t('dashboard.content.empty_body')} />
	{:else}
		<div class="flex flex-wrap items-center gap-x-3 gap-y-2">
			<label class="flex items-center gap-2 text-sm text-[var(--color-muted)]">
				<input
					type="checkbox"
					checked={allSelected}
					onchange={toggleSelectAll}
					aria-label={t('dashboard.content.select_all', 'Select all')}
				/>
				{t('dashboard.content.select_all', 'Select all')}
			</label>
			{#if selectedIds.size > 0}
				<span class="text-xs text-[var(--color-muted)]">
					{t('dashboard.content.selected_count', '{count} selected', { count: selectedIds.size })}
				</span>
				<div class="ml-auto flex flex-wrap items-center gap-2">
					{#if bulkBusy}
						<Loader2 class="size-4 animate-spin text-[var(--color-muted)]" />
					{:else}
						{#if publishableIds.length > 0}
							<button
								type="button"
								class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1 text-xs text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
								onclick={() => doBulk('publish', publishableIds)}
							>
								{publishLabel} ({publishableIds.length})
							</button>
						{/if}
						{#if archivableIds.length > 0}
							<button
								type="button"
								class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1 text-xs text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
								onclick={() => doBulk('archive', archivableIds)}
							>
								{t('dashboard.content.archive')} ({archivableIds.length})
							</button>
						{/if}
						<button
							type="button"
							class="rounded-[var(--radius)] border border-[var(--color-danger)]/40 px-3 py-1 text-xs text-[var(--color-danger)] hover:bg-[var(--color-danger)]/10"
							onclick={() => (deleteIds = [...selectedIds])}
						>
							{t('dashboard.content.delete')} ({selectedIds.size})
						</button>
						<button
							type="button"
							class="px-2 py-1 text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
							onclick={() => (selectedIds = new Set())}
						>
							{t('common.clear', 'Clear')}
						</button>
					{/if}
				</div>
			{/if}
		</div>
		<ul class="space-y-2">
			{#each items as item (item.id)}
				{@const href = itemHref(item)}
				<li class="flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
					<input
						type="checkbox"
						class="shrink-0"
						checked={selectedIds.has(item.id)}
						onchange={() => toggleSelect(item.id)}
						aria-label={t('dashboard.content.select_row', 'Select item')}
					/>
					<div class="min-w-0 flex-1">
						<p class="truncate text-sm font-medium text-[var(--color-text)]">{item.title ?? t('dashboard.content.untitled')}</p>
						<p class="text-xs text-[var(--color-muted)]">
							{typeLabel(item.content_type)} · {formatDate(item.created_at)}
							{#if item.status === 'scheduled' && item.publish_at}
								· {t('dashboard.content.scheduled_for', 'Scheduled for')} {formatDateTime(item.publish_at)}
							{/if}
							{#if item.view_count !== undefined} · {item.view_count} {t('dashboard.content.views')}{/if}
							{#if item.comment_count !== undefined} · {item.comment_count} {t('dashboard.content.comments')}{/if}
						</p>
					</div>
					<div class="ml-3 flex shrink-0 items-center gap-2">
						{#if actingId === item.id || bulkBusy}
							<Loader2 class="size-4 animate-spin text-[var(--color-muted)]" />
						{:else}
							{#if href}
								<a href={href} class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]">
									{isComposerEditable(item) ? t('dashboard.content.edit') : t('common.view', 'View')}
								</a>
							{/if}
							{#if item.status === 'draft' || item.status === 'scheduled' || item.status === 'archived'}
								<button
									type="button"
									class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)] disabled:opacity-50"
									disabled={!!actingId || bulkBusy}
									onclick={() => doPublish(item.id)}
								>
									{item.status === 'archived' ? t('dashboard.content.restore', 'Restore') : t('dashboard.content.publish')}
								</button>
							{/if}
							{#if item.status !== 'archived'}
								<button
									type="button"
									class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)] disabled:opacity-50"
									disabled={!!actingId || bulkBusy}
									onclick={() => doArchive(item.id)}
								>
									{t('dashboard.content.archive')}
								</button>
							{/if}
							<button
								type="button"
								class="text-xs text-[var(--color-muted)] hover:text-[var(--color-danger)] disabled:opacity-50"
								disabled={!!actingId || bulkBusy}
								onclick={() => (deleteIds = [item.id])}
							>
								{t('dashboard.content.delete')}
							</button>
						{/if}
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

	<Dialog.Root open={deleteIds !== null} onOpenChange={(v) => { if (!v) deleteIds = null; }}>
		<Dialog.Portal>
			<Dialog.Overlay class="fixed inset-0 z-40 bg-black/50 backdrop-blur-sm" />
			<Dialog.Content class="fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 shadow-xl">
				<Dialog.Title class="mb-2 text-lg font-medium text-[var(--color-text)]">
					{t('dashboard.content.delete_confirm')}
					{#if deleteIds && deleteIds.length > 1}
						<span class="ml-1 text-sm font-normal text-[var(--color-muted)]">({deleteIds.length})</span>
					{/if}
				</Dialog.Title>
				<Dialog.Description class="mb-4 text-sm text-[var(--color-muted)]">
					{#if deleteIds && deleteIds.length > 1}
						{t('dashboard.content.delete_confirm_bulk_body', 'This will delete {count} items. This cannot be undone.', { count: deleteIds.length })}
					{:else}
						{t('dashboard.content.delete_confirm_body')}
					{/if}
				</Dialog.Description>
				<div class="flex justify-end gap-2">
					<Dialog.Close class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)]">{t('common.cancel')}</Dialog.Close>
					<button type="button" class="rounded-[var(--radius)] bg-[var(--color-danger)] px-4 py-1.5 text-sm font-medium text-white" onclick={doDelete}>{t('dashboard.content.delete')}</button>
				</div>
			</Dialog.Content>
		</Dialog.Portal>
	</Dialog.Root>
</section>
