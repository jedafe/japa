<script lang="ts">
	// Ref: .jeos/specs/2026-09-23-studio-moderation-tab.md — Moderation tab:
	// scoped report queue for space/tag moderators, assignees, and platform
	// moderators. Actions hit PATCH /me/moderation/queue/{id} and the list
	// re-syncs via invalidateAll() (Rule 16 — no local patching).
	import { Modal } from '$lib/components/ui';
	import { untrack } from 'svelte';
	import { t, i18n } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto, invalidateAll } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { contentHref } from '$lib/utils/contentLink';
	import Autocomplete, { type AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { Shield, Search, Loader2, X, Flag } from 'lucide-svelte';
	import type { PageData } from './$types';
	import type { ModerationQueueItem } from './+page.server';

	let { data }: { data: PageData } = $props();

	// untrack initializer covers the first (SSR) render; the $effect re-syncs
	// after every server refresh (filter change, pagination, invalidateAll).
	let items = $state<ModerationQueueItem[]>(untrack(() => data.queue.items ?? []));
	let totalPages = $state(untrack(() => data.queue.total_pages));
	let actionError = $state<string | null>(null);
	let actingId = $state<string | null>(null);
	// Bulk selection — only actionable rows (pending/reviewing) can be
	// selected. Ids that leave the list after a refresh are pruned in the
	// sync effect below.
	let selectedIds = $state<Set<string>>(new Set());
	let bulkBusy = $state(false);
	$effect(() => {
		// Compute from the local `next`, not the `items` signal — reading a
		// signal this effect also writes re-schedules it forever (the stored
		// proxy never equals the incoming array).
		const next = data.queue.items ?? [];
		items = next;
		totalPages = data.queue.total_pages;
		actingId = null;
		const visible = new Set(next.map((i) => i.id));
		const pruned = [...selectedIds].filter((id) => visible.has(id));
		if (pruned.length !== selectedIds.size) selectedIds = new Set(pruned);
	});

	const scope = $derived(data.scope);
	const statusParam = $derived(page.url.searchParams.get('status') ?? 'pending');
	const typeParam = $derived(page.url.searchParams.get('entity_type') ?? '');
	const spaceParam = $derived(page.url.searchParams.get('space_id') ?? '');
	const tagParam = $derived(page.url.searchParams.get('tag_id') ?? '');
	const assignedParam = $derived(page.url.searchParams.get('assigned') ?? '');
	const qParam = $derived(page.url.searchParams.get('q') ?? '');
	const currentPage = $derived(data.queue.page);
	// page_size is part of the queue contract (spec §4) — the backend clamps
	// to [1,100] with default 20. The control only offers sane preset sizes.
	const pageSizeParam = $derived(page.url.searchParams.get('page_size') ?? '20');
	const pageSizeItems: AutocompleteItem[] = $derived(
		['10', '20', '50'].map((v) => ({
			id: v,
			label: t('studio.moderation.per_page', '{n} / page').replace('{n}', v)
		}))
	);
	const selectedPageSizeLabel = $derived(
		pageSizeItems.find((i) => i.id === pageSizeParam)?.label ??
			t('studio.moderation.per_page', '{n} / page').replace('{n}', pageSizeParam)
	);

	// Rule 16 — status/entity-type options come from the backend catalogs
	// loaded in +page.server.ts; the labels fall back to the raw value.
	// 'all' is a UI-only sentinel — +page.server.ts strips it so the backend
	// sees no status filter (which is its "all" semantics).
	const statusTabs = $derived([
		...data.statusOptions.map((o) => ({ key: o.value, label: o.label })),
		{ key: 'all', label: t('studio.moderation.status_all', 'All') }
	]);

	const typeItems = $derived(
		data.typeOptions.map((o): AutocompleteItem => ({ id: o.value, label: o.label }))
	);
	const selectedTypeLabel = $derived(
		typeItems.find((i) => i.id === typeParam)?.label ?? (typeParam || null)
	);

	// Space/tag filters are scoped to the caller's own moderated entities —
	// rendered only when the caller actually moderates spaces/tags. The
	// Autocomplete fetchers filter these small lists locally (no API call).
	const spaceItems = $derived(
		(scope?.moderated_spaces ?? []).map((s): AutocompleteItem => ({ id: s.id, label: s.name }))
	);
	const tagItems = $derived(
		(scope?.moderated_tags ?? []).map((tg): AutocompleteItem => ({ id: tg.id, label: tg.name }))
	);

	function localFetcher(list: AutocompleteItem[]) {
		return async (q: string) => {
			const needle = q.toLowerCase();
			return list.filter((i) => i.label.toLowerCase().includes(needle));
		};
	}

	const selectedSpaceLabel = $derived(spaceItems.find((i) => i.id === spaceParam)?.label ?? null);
	const selectedTagLabel = $derived(tagItems.find((i) => i.id === tagParam)?.label ?? null);

	function filterParams(overrides: Record<string, string | undefined>): URLSearchParams {
		const params = new URLSearchParams(page.url.searchParams);
		params.delete('page');
		for (const [k, v] of Object.entries(overrides)) {
			if (v) params.set(k, v);
			else params.delete(k);
		}
		return params;
	}

	function applyFilter(overrides: Record<string, string | undefined>) {
		goto(`/studio/moderation?${filterParams(overrides).toString()}`, { keepFocus: true, noScroll: true });
	}

	let searchTimer: ReturnType<typeof setTimeout> | null = null;
	function onSearchInput(value: string) {
		if (searchTimer) clearTimeout(searchTimer);
		searchTimer = setTimeout(() => applyFilter({ q: value || undefined }), 350);
	}

	function goPage(p: number) {
		const params = new URLSearchParams(page.url.searchParams);
		params.set('page', String(p));
		goto(`/studio/moderation?${params.toString()}`, { keepFocus: true, noScroll: true });
	}

	// Selection helpers. Actionable = pending/reviewing (the four PATCH
	// actions). Resolved rows are inert for actions, but platform moderators
	// can clear them — DELETE /admin/reports/{id} hard-deletes the report
	// row and is gated by reports.review, which platform_moderator proxies.
	function isActionable(item: ModerationQueueItem): boolean {
		return item.status === 'pending' || item.status === 'reviewing';
	}
	function isClearable(item: ModerationQueueItem): boolean {
		return !isActionable(item) && !!scope?.platform_moderator;
	}
	const actionableIds = $derived(items.filter(isActionable).map((i) => i.id));
	const clearableIds = $derived(items.filter(isClearable).map((i) => i.id));
	const selectableIds = $derived([...actionableIds, ...clearableIds]);
	const selectedItems = $derived(items.filter((i) => selectedIds.has(i.id)));
	const selectedActionable = $derived(selectedItems.filter(isActionable));
	const selectedClearable = $derived(selectedItems.filter(isClearable));
	const allSelected = $derived(
		selectableIds.length > 0 && selectableIds.every((id) => selectedIds.has(id))
	);
	// warn/ban are scope-gated per row — only offer them in bulk when every
	// selected actionable row qualifies, otherwise the batch is guaranteed
	// to partially 403 (ErrActionNotAllowed per row).
	const canBulkWarnBan = $derived(
		selectedActionable.length > 0 && selectedActionable.every(canWarnBan)
	);

	function toggleSelect(id: string) {
		const next = new Set(selectedIds);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		selectedIds = next;
	}

	function toggleSelectAll() {
		selectedIds = allSelected ? new Set() : new Set(selectableIds);
	}

	// Action confirmation dialog — warn/ban/remove ask for a reason. The
	// same dialog serves single rows and the bulk selection (ids array).
	let pendingAction = $state<{ ids: string[]; action: string } | null>(null);
	let actionReason = $state('');

	function requestAction(id: string, action: string) {
		if (action === 'dismiss') {
			void doActions([id], action, '');
			return;
		}
		actionReason = '';
		pendingAction = { ids: [id], action };
	}

	function requestBulk(action: string) {
		if (bulkBusy || selectedIds.size === 0) return;
		// Mixed selections are allowed: the action buttons operate on the
		// actionable subset, Clear on the resolved subset (spec §4 bulk).
		const ids =
			action === 'clear'
				? selectedClearable.map((i) => i.id)
				: selectedActionable.map((i) => i.id);
		if (ids.length === 0) return;
		if (action === 'dismiss') {
			void doActions(ids, action, '');
			return;
		}
		actionReason = '';
		pendingAction = { ids, action };
	}

	async function confirmAction() {
		if (!pendingAction) return;
		const { ids, action } = pendingAction;
		pendingAction = null;
		await doActions(ids, action, actionReason);
	}

	function actionErrorMessage(err: { code?: string; message?: string }): string {
		return err.code === 'action_not_allowed'
			? t('studio.moderation.action_not_allowed', 'This action exceeds your moderation scope')
			: err.code === 'already_resolved'
				? t('studio.moderation.already_resolved', 'This report was already resolved by another moderator')
				: err.message || t('studio.moderation.action_error', 'Action failed — please try again');
	}

	// Runs the action over one or many rows. There is no /me bulk endpoint —
	// per-row PATCHes keep the backend's per-report scope checks intact, so
	// a mixed-scope selection degrades to partial failure instead of an
	// all-or-nothing 403. After the batch, invalidateAll() re-syncs the list.
	async function doActions(ids: string[], action: string, reason: string) {
		if (actingId || bulkBusy || ids.length === 0) return;
		const single = ids.length === 1;
		if (single) actingId = ids[0];
		else bulkBusy = true;
		actionError = null;

		// 'clear' hard-deletes the report row via the admin endpoint
		// (reports.review-gated); the other actions PATCH the /me queue.
		let failed = 0;
		let firstError: { code?: string; message?: string } | null = null;
		for (const id of ids) {
			const res =
				action === 'clear'
					? await api.delete<unknown>(`/admin/reports/${id}`)
					: await api.patch<unknown>(`/me/moderation/queue/${id}`, { action, reason });
			if (res.error) {
				failed++;
				firstError ??= res.error;
			}
		}

		if (failed === ids.length) {
			actionError = actionErrorMessage(firstError ?? {});
		} else if (failed > 0) {
			actionError =
				action === 'clear'
					? t(
							'studio.moderation.bulk_clear_partial',
							'{done} of {total} cleared — {failed} failed',
							{ done: ids.length - failed, total: ids.length, failed }
						)
					: t(
							'studio.moderation.bulk_partial',
							'{done} of {total} actioned — {failed} failed',
							{ done: ids.length - failed, total: ids.length, failed }
						);
		}
		await invalidateAll();
	}

	// warn/ban need platform, assigned, or space scope — tag scope alone is
	// content-actions only (spec §4). scope flags come from the row.
	// space/source reports have no user target — hide warn/ban there.
	function canWarnBan(item: ModerationQueueItem): boolean {
		if (item.entity_type === 'space' || item.entity_type === 'source') return false;
		return !!(scope?.platform_moderator || item.scope_assigned || item.scope_space);
	}

	function scopeLabels(item: ModerationQueueItem): string[] {
		const labels: string[] = [];
		if (scope?.platform_moderator) labels.push(t('studio.moderation.scope_platform', 'platform'));
		if (item.scope_assigned) labels.push(t('studio.moderation.scope_assigned', 'assigned'));
		if (item.scope_space) labels.push(t('studio.moderation.scope_space', 'my space'));
		if (item.scope_tag) labels.push(t('studio.moderation.scope_tag', 'my tag'));
		return labels;
	}

	function entityHref(item: ModerationQueueItem): string | null {
		// entity_slug carries the content slug for content/comment reports, the
		// space slug for space reports, and the source slug for source reports.
		// entity_content_type picks the canonical per-type route via contentHref
		// (/story, /video, /guide, /forum, …). thread_slug marks content backing
		// a forum thread — those render at /forum/[slug]. Messages have no public
		// page (DMs are private to participants) — the excerpt renders inline.
		if (item.entity_type === 'content' || item.entity_type === 'comment') {
			if (item.thread_slug) return `/forum/${item.thread_slug}`;
			if (item.entity_slug) {
				return contentHref({ slug: item.entity_slug, content_type: item.entity_content_type });
			}
			return null;
		}
		if (item.entity_type === 'space' && item.entity_slug) return `/spaces/${item.entity_slug}`;
		if (item.entity_type === 'user' && item.entity_title) return `/u/${item.entity_title}`;
		if (item.entity_type === 'source') return `/sources/${item.entity_slug || item.entity_id}`;
		return null;
	}

	function formatDate(iso: string): string {
		const d = new Date(iso);
		return isNaN(d.getTime()) ? '—' : d.toLocaleDateString(i18n.locale);
	}

	function reasonLabel(reason: string): string {
		return t(`studio.moderation.reason_${reason}`, reason.replace(/_/g, ' '));
	}
</script>

<svelte:head>
	<title>{t('studio.tabs.moderation', 'Moderation')} — {t('studio.title', 'Studio — JapaConnect')}</title>
	<meta name="robots" content="noindex" />
</svelte:head>

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
		{#each statusTabs as tab (tab.key)}
			<button
				type="button"
				class="rounded-[var(--radius)] px-3 py-1.5 text-sm {statusParam === tab.key
					? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)]'
					: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
				onclick={() => applyFilter({ status: tab.key === 'pending' ? undefined : tab.key })}
			>
				{tab.label}
			</button>
		{/each}
		<button
			type="button"
			class="rounded-[var(--radius)] px-3 py-1.5 text-sm {assignedParam
				? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)]'
				: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
			onclick={() => applyFilter({ assigned: assignedParam ? undefined : 'me' })}
		>
			{t('studio.moderation.assigned_to_me', 'Assigned to me')}
		</button>
	</div>

	<div class="mt-2 flex flex-wrap items-center gap-2">
		<div class="relative">
			<Search class="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-[var(--color-muted)]" />
			<input
				type="search"
				placeholder={t('studio.moderation.search_placeholder', 'Search reports…')}
				value={qParam}
				oninput={(e) => onSearchInput(e.currentTarget.value)}
				class="w-44 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-1.5 pl-8 pr-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:outline-none focus:ring-1 focus:ring-[var(--color-border)] sm:w-56"
			/>
		</div>
		<div class="ml-auto flex flex-wrap items-center gap-2">
			<div class="w-40">
				<Autocomplete
					placeholder={t('studio.moderation.all_types', 'All types')}
					emptyMessage={t('studio.moderation.no_matches', 'No matches')}
					fetcher={localFetcher(typeItems)}
					initialFetcher={async () => typeItems}
					selectedId={typeParam || null}
					selectedLabel={selectedTypeLabel}
					onSelect={(i) => applyFilter({ entity_type: i.id })}
					onClear={() => applyFilter({ entity_type: undefined })}
					class="w-full"
				/>
			</div>
			{#if spaceItems.length > 0}
				<div class="w-44">
					<Autocomplete
						placeholder={t('studio.moderation.all_spaces', 'All my spaces')}
						emptyMessage={t('studio.moderation.no_matches', 'No matches')}
						fetcher={localFetcher(spaceItems)}
						initialFetcher={async () => spaceItems}
						selectedId={spaceParam || null}
						selectedLabel={selectedSpaceLabel}
						onSelect={(i) => applyFilter({ space_id: i.id })}
						onClear={() => applyFilter({ space_id: undefined })}
						class="w-full"
					/>
				</div>
			{/if}
			{#if tagItems.length > 0}
				<div class="w-44">
					<Autocomplete
						placeholder={t('studio.moderation.all_tags', 'All my tags')}
						emptyMessage={t('studio.moderation.no_matches', 'No matches')}
						fetcher={localFetcher(tagItems)}
						initialFetcher={async () => tagItems}
						selectedId={tagParam || null}
						selectedLabel={selectedTagLabel}
						onSelect={(i) => applyFilter({ tag_id: i.id })}
						onClear={() => applyFilter({ tag_id: undefined })}
						class="w-full"
					/>
				</div>
			{/if}
			<div class="w-32">
				<Autocomplete
					ariaLabel={t('studio.moderation.page_size', 'Page size')}
					placeholder={t('studio.moderation.page_size', 'Page size')}
					emptyMessage={t('studio.moderation.no_matches', 'No matches')}
					fetcher={localFetcher(pageSizeItems)}
					initialFetcher={async () => pageSizeItems}
					selectedId={pageSizeParam}
					selectedLabel={selectedPageSizeLabel}
					onSelect={(i) => applyFilter({ page_size: i.id === '20' ? undefined : i.id })}
					onClear={() => applyFilter({ page_size: undefined })}
					class="w-full"
				/>
			</div>
		</div>
	</div>

	{#if items.length === 0}
		<EmptyState
			icon={Flag}
			title={t('studio.moderation.empty_title', 'Nothing in your queue')}
			body={t('studio.moderation.empty_body', 'Reports in your moderated spaces and tags will appear here.')}
		/>
	{:else}
		{#if selectableIds.length > 0}
			<div class="flex flex-wrap items-center gap-x-3 gap-y-2">
				<label class="flex items-center gap-2 text-sm text-[var(--color-muted)]">
					<input
						type="checkbox"
						checked={allSelected}
						onchange={toggleSelectAll}
						aria-label={t('studio.moderation.select_all', 'Select all')}
					/>
					{t('studio.moderation.select_all', 'Select all')}
				</label>
				{#if selectedIds.size > 0}
					<span class="text-xs text-[var(--color-muted)]">
						{t('studio.moderation.selected_count', '{count} selected', { count: selectedIds.size })}
					</span>
					<div class="ml-auto flex flex-wrap items-center gap-2">
						{#if bulkBusy}
							<Loader2 class="size-4 animate-spin text-[var(--color-muted)]" />
						{:else}
							{#if selectedActionable.length > 0}
								<button
									type="button"
									class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1 text-xs text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
									onclick={() => requestBulk('dismiss')}
								>
									{t('studio.moderation.dismiss', 'Dismiss')} ({selectedActionable.length})
								</button>
								<button
									type="button"
									class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1 text-xs text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
									onclick={() => requestBulk('remove')}
								>
									{t('studio.moderation.remove', 'Remove')} ({selectedActionable.length})
								</button>
								{#if canBulkWarnBan}
									<button
										type="button"
										class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1 text-xs text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
										onclick={() => requestBulk('warn')}
									>
										{t('studio.moderation.warn', 'Warn')} ({selectedActionable.length})
									</button>
									<button
										type="button"
										class="rounded-[var(--radius)] border border-[var(--color-danger)]/40 px-3 py-1 text-xs text-[var(--color-danger)] hover:bg-[var(--color-danger)]/10"
										onclick={() => requestBulk('ban')}
									>
										{t('studio.moderation.ban', 'Ban')} ({selectedActionable.length})
									</button>
								{/if}
							{/if}
							{#if selectedClearable.length > 0}
								<button
									type="button"
									class="rounded-[var(--radius)] border border-[var(--color-danger)]/40 px-3 py-1 text-xs text-[var(--color-danger)] hover:bg-[var(--color-danger)]/10"
									onclick={() => requestBulk('clear')}
								>
									{t('studio.moderation.clear', 'Clear')} ({selectedClearable.length})
								</button>
							{/if}
							<button
								type="button"
								class="px-2 py-1 text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
								onclick={() => (selectedIds = new Set())}
							>
								{t('studio.moderation.clear_selection', 'Clear selection')}
							</button>
						{/if}
					</div>
				{/if}
			</div>
		{/if}
		<ul class="space-y-2">
			{#each items as item (item.id)}
				{@const href = entityHref(item)}
				<li class="flex items-start gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
					{#if isActionable(item) || isClearable(item)}
						<input
							type="checkbox"
							class="mt-1 shrink-0"
							checked={selectedIds.has(item.id)}
							onchange={() => toggleSelect(item.id)}
							aria-label={t('studio.moderation.select_row', 'Select report')}
						/>
					{/if}
					<div class="flex min-w-0 flex-1 items-start justify-between gap-3">
						<div class="min-w-0">
							<div class="flex flex-wrap items-center gap-2">
								{#if href}
									<a href={href} class="truncate text-sm font-medium text-[var(--color-text)] hover:underline">
										{item.entity_title || t('studio.moderation.untitled', 'Untitled')}
									</a>
								{:else}
									<p class="truncate text-sm font-medium text-[var(--color-text)]">
										{item.entity_title || t('studio.moderation.untitled', 'Untitled')}
									</p>
								{/if}
								<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs text-[var(--color-muted)]">
									{item.entity_type}
								</span>
								<span class="rounded-full bg-[var(--color-danger)]/10 px-2 py-0.5 text-xs text-[var(--color-danger)]">
									{reasonLabel(item.reason)}
								</span>
								{#each scopeLabels(item) as label}
									<span class="rounded-full border border-[var(--color-border)] px-2 py-0.5 text-xs text-[var(--color-muted)]">
										{label}
									</span>
								{/each}
							</div>
							{#if item.entity_excerpt}
								<p class="mt-1 line-clamp-2 text-xs text-[var(--color-muted)]">{item.entity_excerpt}</p>
							{/if}
							{#if item.notes}
								<p class="mt-1 line-clamp-2 text-xs italic text-[var(--color-muted)]">“{item.notes}”</p>
							{/if}
							<p class="mt-1 text-xs text-[var(--color-muted)]">
								{t('studio.moderation.reported_by', 'Reported by')}
								{item.reporter_display_name ?? item.reporter_username ?? t('studio.moderation.anonymous', 'anonymous')}
								· {formatDate(item.created_at)}
								{#if item.entity_author_username}
									· {t('studio.moderation.author', 'author')}: {item.entity_author_username}
								{/if}
								{#if item.entity_space_name}
									· {item.entity_space_name}
								{/if}
							</p>
						</div>
						{#if item.status === 'pending' || item.status === 'reviewing'}
							<div class="flex shrink-0 flex-wrap justify-end gap-2">
								{#if actingId === item.id || bulkBusy}
									<Loader2 class="size-4 animate-spin text-[var(--color-muted)]" />
								{:else}
									<button
										type="button"
										class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
										onclick={() => requestAction(item.id, 'dismiss')}
									>
										{t('studio.moderation.dismiss', 'Dismiss')}
									</button>
									<button
										type="button"
										class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
										onclick={() => requestAction(item.id, 'remove')}
									>
										{t('studio.moderation.remove', 'Remove')}
									</button>
									{#if canWarnBan(item)}
										<button
											type="button"
											class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
											onclick={() => requestAction(item.id, 'warn')}
										>
											{t('studio.moderation.warn', 'Warn')}
										</button>
										<button
											type="button"
											class="text-xs text-[var(--color-danger)] hover:opacity-80"
											onclick={() => requestAction(item.id, 'ban')}
										>
											{t('studio.moderation.ban', 'Ban')}
										</button>
									{/if}
								{/if}
							</div>
						{:else if isClearable(item)}
							<div class="flex shrink-0 flex-wrap justify-end gap-2">
								{#if actingId === item.id || bulkBusy}
									<Loader2 class="size-4 animate-spin text-[var(--color-muted)]" />
								{:else}
									<button
										type="button"
										class="text-xs text-[var(--color-danger)] hover:opacity-80"
										onclick={() => requestAction(item.id, 'clear')}
									>
										{t('studio.moderation.clear', 'Clear')}
									</button>
								{/if}
							</div>
						{/if}
					</div>
				</li>
			{/each}
		</ul>

		{#if totalPages > 1}
			<div class="flex items-center justify-center gap-2 pt-2">
				<button
					type="button"
					class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] disabled:opacity-50"
					disabled={currentPage <= 0}
					onclick={() => goPage(currentPage - 1)}
				>
					{t('common.previous', 'Previous')}
				</button>
				<span class="text-xs text-[var(--color-muted)]">
					{currentPage + 1} / {totalPages}
				</span>
				<button
					type="button"
					class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] disabled:opacity-50"
					disabled={currentPage >= totalPages - 1}
					onclick={() => goPage(currentPage + 1)}
				>
					{t('common.next', 'Next')}
				</button>
			</div>
		{/if}
	{/if}

	<Modal open={pendingAction !== null} onOpenChange={(v) => { if (!v) pendingAction = null; }} overlayClass="backdrop-blur-sm" size="md">
		{#snippet title()}
			{#if pendingAction?.action === 'ban'}
				{t('studio.moderation.confirm_ban', 'Ban user')}
			{:else if pendingAction?.action === 'warn'}
				{t('studio.moderation.confirm_warn', 'Warn user')}
			{:else if pendingAction?.action === 'clear'}
				{t('studio.moderation.confirm_clear', 'Clear report')}
			{:else}
				{t('studio.moderation.confirm_remove', 'Remove report')}
			{/if}
			{#if pendingAction && pendingAction.ids.length > 1}
				<span class="ml-1 text-sm font-normal text-[var(--color-muted)]">
					({pendingAction.ids.length})
				</span>
			{/if}
		{/snippet}
		{#snippet description()}
			{#if pendingAction?.action === 'ban'}
				{t('studio.moderation.confirm_ban_body', 'This bans the reported user and marks the report actioned.')}
			{:else if pendingAction?.action === 'warn'}
				{t('studio.moderation.confirm_warn_body', 'This records a warning against the reported user and marks the report actioned.')}
			{:else if pendingAction?.action === 'clear'}
				{t('studio.moderation.confirm_clear_body', 'This permanently deletes the resolved report from the queue. This cannot be undone.')}
			{:else}
				{t('studio.moderation.confirm_remove_body', 'This marks the report as actioned.')}
			{/if}
			{#if pendingAction && pendingAction.ids.length > 1}
				<span class="mt-1 block text-xs">
					{t('studio.moderation.bulk_applies_to', 'Applies to {count} selected reports.', { count: pendingAction.ids.length })}
				</span>
			{/if}
		{/snippet}
		{#snippet children(close)}
				{#if pendingAction?.action !== 'clear'}
					<label class="mb-4 block text-sm text-[var(--color-muted)]" for="moderation-action-reason">
						{t('studio.moderation.reason_label', 'Reason (optional)')}
					</label>
					<textarea
						id="moderation-action-reason"
						bind:value={actionReason}
						rows="3"
						class="mb-4 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-1 focus:ring-[var(--color-border)]"
					></textarea>
				{/if}
				<div class="flex justify-end gap-2">
					<button type="button" onclick={close} class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)]">
						{t('common.cancel', 'Cancel')}
					</button>
					<button
						type="button"
						class="rounded-[var(--radius)] px-4 py-1.5 text-sm font-medium text-white {pendingAction?.action === 'ban' || pendingAction?.action === 'clear' ? 'bg-[var(--color-danger)]' : 'bg-[var(--color-primary)]'}"
						onclick={confirmAction}
					>
						{t('common.confirm', 'Confirm')}
					</button>
				</div>
		{/snippet}
	</Modal>
</section>
