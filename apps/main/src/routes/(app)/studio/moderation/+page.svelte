<script lang="ts">
	// Ref: .jeos/specs/2026-09-23-studio-moderation-tab.md — Moderation tab:
	// scoped report queue for space/tag moderators, assignees, and platform
	// moderators. Actions hit PATCH /me/moderation/queue/{id} and the list
	// re-syncs via invalidateAll() (Rule 16 — no local patching).
	import { Dialog } from 'bits-ui';
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
	$effect(() => {
		items = data.queue.items ?? [];
		totalPages = data.queue.total_pages;
		actingId = null;
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

	// Action confirmation dialog — warn/ban/remove ask for a reason.
	let pendingAction = $state<{ id: string; action: string } | null>(null);
	let actionReason = $state('');

	function requestAction(id: string, action: string) {
		if (action === 'dismiss') {
			doAction(id, action, '');
			return;
		}
		actionReason = '';
		pendingAction = { id, action };
	}

	async function confirmAction() {
		if (!pendingAction) return;
		const { id, action } = pendingAction;
		pendingAction = null;
		await doAction(id, action, actionReason);
	}

	async function doAction(id: string, action: string, reason: string) {
		if (actingId) return;
		actingId = id;
		actionError = null;
		const res = await api.patch<unknown>(`/me/moderation/queue/${id}`, { action, reason });
		if (res.error) {
			actionError =
				res.error.code === 'action_not_allowed'
					? t('studio.moderation.action_not_allowed', 'This action exceeds your moderation scope')
					: res.error.code === 'already_resolved'
						? t('studio.moderation.already_resolved', 'This report was already resolved by another moderator')
						: res.error.message || t('studio.moderation.action_error', 'Action failed — please try again');
			actingId = null;
			// already_resolved means another moderator got there first — the
			// stale row must disappear, so re-sync even though the call failed.
			if (res.error.code === 'already_resolved') await invalidateAll();
			return;
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
		<ul class="space-y-2">
			{#each items as item (item.id)}
				{@const href = entityHref(item)}
				<li class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
					<div class="flex items-start justify-between gap-3">
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
								{#if actingId === item.id}
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

	<Dialog.Root open={pendingAction !== null} onOpenChange={(v) => { if (!v) pendingAction = null; }}>
		<Dialog.Portal>
			<Dialog.Overlay class="fixed inset-0 z-40 bg-black/50 backdrop-blur-sm" />
			<Dialog.Content class="fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 shadow-xl">
				<Dialog.Title class="mb-2 text-lg font-medium text-[var(--color-text)]">
					{#if pendingAction?.action === 'ban'}
						{t('studio.moderation.confirm_ban', 'Ban user')}
					{:else if pendingAction?.action === 'warn'}
						{t('studio.moderation.confirm_warn', 'Warn user')}
					{:else}
						{t('studio.moderation.confirm_remove', 'Remove report')}
					{/if}
				</Dialog.Title>
				<Dialog.Description class="mb-4 text-sm text-[var(--color-muted)]">
					{#if pendingAction?.action === 'ban'}
						{t('studio.moderation.confirm_ban_body', 'This bans the reported user and marks the report actioned.')}
					{:else if pendingAction?.action === 'warn'}
						{t('studio.moderation.confirm_warn_body', 'This records a warning against the reported user and marks the report actioned.')}
					{:else}
						{t('studio.moderation.confirm_remove_body', 'This marks the report as actioned.')}
					{/if}
				</Dialog.Description>
				<label class="mb-4 block text-sm text-[var(--color-muted)]" for="moderation-action-reason">
					{t('studio.moderation.reason_label', 'Reason (optional)')}
				</label>
				<textarea
					id="moderation-action-reason"
					bind:value={actionReason}
					rows="3"
					class="mb-4 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-1 focus:ring-[var(--color-border)]"
				></textarea>
				<div class="flex justify-end gap-2">
					<Dialog.Close class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)]">
						{t('common.cancel', 'Cancel')}
					</Dialog.Close>
					<button
						type="button"
						class="rounded-[var(--radius)] px-4 py-1.5 text-sm font-medium text-white {pendingAction?.action === 'ban' ? 'bg-[var(--color-danger)]' : 'bg-[var(--color-primary)]'}"
						onclick={confirmAction}
					>
						{t('common.confirm', 'Confirm')}
					</button>
				</div>
			</Dialog.Content>
		</Dialog.Portal>
	</Dialog.Root>
</section>
