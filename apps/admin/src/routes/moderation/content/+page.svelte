<script lang="ts">
	import { siteName } from '#lib/stores/site.svelte.js';
	// /moderation/content — migrated to the EntityPage scaffold.
	// Distinct moderation-focused view of platform content (open-report count,
	// featured badge, type/status/deleted filters) with read-only list +
	// Remove/Restore state actions. Uses apiRaw because GET /admin/content
	// returns { data, total, total_pages } with totals alongside `data`.
	import { EntityPage, type EntityPageConfig, type EntityPageController } from '#lib/components/entity/index.js';
	import { Modal } from '#lib/components/ui/modal/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { contentAdminApi, apiRaw } from '#lib/api.js';
	import { escapeHtml } from '#lib/utils/html.js';
	import { t } from '#lib/stores/i18n.svelte.js';
	import { hasPermission } from '#lib/stores/permissions.svelte.js';
	import { Eye, Trash2, RotateCcw } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '#lib/components/ui/data-table/index.js';

	type ContentItem = {
		id: string;
		title: string;
		slug: string;
		content_type: string;
		status: string;
		visibility: string;
		author_id: string;
		author_username?: string;
		author_display_name?: string;
		space_id?: string;
		space_name?: string;
		body?: string;
		body_preview?: string;
		reports_count: number;
		is_featured: boolean;
		deleted_at?: string | null;
		created_at: string;
	};

	// Backend gates /admin/content list on admin.moderation and remove/restore
	// on content.delete_any.
	const canModerateContent = $derived(hasPermission('content.delete_any'));

	// ── Filter options ─────────────────────────────────────────────────────
	// Type and status options are loaded from the backend so they match the
	// contents table enums and content type catalog (Rule 16).
	let typeOptions = $state<{ value: string; label: string }[]>([]);
	let statusOptions = $state<{ value: string; label: string }[]>([]);

	$effect(() => {
		(async () => {
			try {
				const [typesRes, statusRes] = await Promise.all([
					contentAdminApi.listContentTypes(),
					contentAdminApi.getStatusOptions()
				]);
				if (typesRes.ok) {
					typeOptions = (typesRes.data as { value: string; label: string }[]).map((ct) => ({
						value: ct.value,
						label: t(`content.type.${ct.value}`, ct.label)
					}));
				}
				if (statusRes.ok) {
					statusOptions = (statusRes.data as { value: string; label: string }[]).map((s) => ({
						value: s.value,
						label: t(`content.status.${s.value}`, s.label)
					}));
				}
			} catch {
				// Keep empty fallback on failure.
			}
		})();
	});

	const filterDefs = $derived<FilterDef[]>([
		{ key: 'content_type', label: t('moderation.content_type', 'Content Type'), options: typeOptions },
		{ key: 'status', label: t('moderation.status', 'Status'), options: statusOptions },
		// Removed content is hidden by default (deleted_at IS NULL) — this
		// filter surfaces it so the Restore action has targets.
		{
			key: 'deleted',
			label: t('common.filter_deleted', 'Deleted'),
			options: [
				{ value: 'only', label: t('common.filter_deleted_only', 'Deleted only') },
				{ value: 'all', label: t('common.filter_deleted_all', 'Include deleted') }
			]
		}
	]);

	// ── Body preview: TipTap JSON / markdown → plain text for the view modal.
	function bodyPreview(body: string | undefined): string {
		const raw = (body ?? '').trim();
		if (!raw) return '';
		if (raw.startsWith('{') || raw.startsWith('[')) {
			try {
				const doc = JSON.parse(raw);
				const parts: string[] = [];
				const walk = (node: unknown): void => {
					if (!node || typeof node !== 'object') return;
					const n = node as { type?: string; text?: string; content?: unknown[] };
					if (n.type === 'text' && typeof n.text === 'string') parts.push(n.text);
					if (Array.isArray(n.content)) n.content.forEach(walk);
				};
				walk(doc);
				const text = parts.join(' ').replace(/\s+/g, ' ').trim();
				// JSON parsed but holds no readable text — show nothing rather
				// than dumping raw JSON into the preview.
				return text ? text.slice(0, 280) : '';
			} catch {
				// Not JSON — fall through to the plain-text path.
			}
		}
		return raw.replace(/\s+/g, ' ').slice(0, 280);
	}

	// ── State-action confirmation modals ───────────────────────────────────
	// EntityPage's confirm Modal only covers the default delete; state actions
	// keep page-local Modals, confirm calls the API + controller.reload().
	let entityController = $state<EntityPageController | undefined>(undefined);
	let acting = $state(false);
	let actionTarget = $state<ContentItem | null>(null);
	let pageError = $state('');

	let showRemoveModal = $state(false);
	let showRestoreModal = $state(false);

	function openRemove(item: ContentItem) {
		actionTarget = item;
		showRemoveModal = true;
	}

	function openRestore(item: ContentItem) {
		actionTarget = item;
		showRestoreModal = true;
	}

	async function confirmRemove() {
		if (!actionTarget) return;
		acting = true;
		try {
			const result = await contentAdminApi.remove(actionTarget.id);
			if (result.ok) {
				showRemoveModal = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'Failed to remove content');
			}
		} catch {
			pageError = t('common.error', 'Failed to remove content');
		} finally {
			acting = false;
		}
	}

	async function confirmRestore() {
		if (!actionTarget) return;
		acting = true;
		try {
			const result = await contentAdminApi.restore(actionTarget.id);
			if (result.ok) {
				showRestoreModal = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'Failed to restore content');
			}
		} catch {
			pageError = t('common.error', 'Failed to restore content');
		} finally {
			acting = false;
		}
	}

	// ── Row actions ────────────────────────────────────────────────────────
	// Custom actions replace the defaults, so View is included explicitly.
	// Remove/Restore are gated on content.delete_any (the node the backend
	// enforces) and filtered per-row on deleted_at, which is how the API
	// reports removal.
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row, ctx) => ctx?.openView(_row) }
		];
		if (canModerateContent) {
			acts.push({
				label: t('content.remove', 'Remove'),
				icon: Trash2,
				variant: 'destructive',
				onclick: openRemove,
				show: (row: ContentItem) => !row.deleted_at
			});
			acts.push({
				label: t('content.restore', 'Restore'),
				icon: RotateCcw,
				onclick: openRestore,
				show: (row: ContentItem) => !!row.deleted_at
			});
		}
		return acts;
	});

	function typeBadge(type: string) {
		switch (type) {
			case 'post':
			case 'comment':
				return 'bg-info/15 text-info dark:text-info';
			case 'blog_post':
				return 'bg-danger/15 text-danger dark:text-danger';
			case 'forum_thread':
				return 'bg-warning/15 text-warning dark:text-warning';
			default:
				return 'bg-muted/15 text-muted dark:text-muted';
		}
	}

	function statusBadge(status: string) {
		switch (status) {
			case 'published':
				return 'bg-success/15 text-success dark:text-success';
			case 'draft':
				return 'bg-muted/15 text-muted dark:text-muted';
			case 'in_review':
				return 'bg-warning/15 text-warning dark:text-warning';
			case 'archived':
				return 'bg-muted/30 text-muted dark:text-muted';
			default:
				return 'bg-info/15 text-info dark:text-info';
		}
	}

	const config = $derived<EntityPageConfig>({
		title: t('moderation.content_admin', 'Content Moderation'),
		description: t('moderation.content.description', 'Browse and manage platform content across all types.'),
		readonly: true,
		pageSize: 25,
		columns: [
			{
				key: 'title',
				label: t('moderation.content.title', 'Title'),
				sortable: true,
				render: (_: any, row: ContentItem) =>
					`<div><p class="font-medium">${escapeHtml(row.title)}</p>${row.is_featured ? '<span class="inline-block rounded-full bg-warning/15 px-2 py-0.5 text-xs text-warning dark:text-warning">' + escapeHtml(t('moderation.content.featured', 'Featured')) + '</span>' : ''}</div>`
			},
			{
				key: 'author',
				label: t('moderation.author', 'Author'),
				sortable: true,
				render: (_: any, row: ContentItem) =>
					`<span class="text-xs" style="color: var(--color-muted)">${escapeHtml(row.author_display_name || row.author_username || '—')}</span>`
			},
			{
				key: 'content_type',
				label: t('moderation.content_type', 'Content Type'),
				sortable: true,
				render: (v: string) =>
					`<span class="inline-block rounded-full px-2 py-0.5 text-xs ${typeBadge(v)}">${escapeHtml(v ?? '—')}</span>`
			},
			{
				key: 'status',
				label: t('moderation.status', 'Status'),
				sortable: true,
				render: (v: string, row: ContentItem) =>
					row.deleted_at
						? `<span class="inline-block rounded-full px-2 py-0.5 text-xs bg-danger/15 text-danger dark:text-danger">${escapeHtml(t('common.filter_deleted', 'Deleted'))}</span>`
						: `<span class="inline-block rounded-full px-2 py-0.5 text-xs ${statusBadge(v)}">${escapeHtml(v ?? '—')}</span>`
			},
			{
				key: 'reports_count',
				label: t('moderation.reports', 'Reports'),
				sortable: true,
				render: (v: number) =>
					v > 0
						? `<span class="inline-block rounded-full bg-danger/15 px-2 py-0.5 text-xs text-danger dark:text-danger">${v}</span>`
						: `<span class="text-xs" style="color: var(--color-muted)">0</span>`
			},
			{
				key: 'created_at',
				label: t('moderation.date', 'Date'),
				sortable: true,
				render: (v: string) =>
					`<span class="text-xs" style="color: var(--color-muted)">${new Date(v).toLocaleDateString()}</span>`
			}
		],
		filters: filterDefs,
		actions: rowActions,
		fields: [],
		load: async ({ page, pageSize, q, sort, filter }) => {
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(page * pageSize)
			});
			if (filter.content_type) params.set('type', filter.content_type);
			if (filter.status) params.set('status', filter.status);
			if (filter.deleted) params.set('deleted', filter.deleted);
			if (q) params.set('q', q);
			if (sort) {
				const [key, dir] = sort.split(':');
				params.set('sort', key);
				params.set('sort_dir', dir === 'asc' ? 'asc' : 'desc');
			}
			const result = await apiRaw<{ data?: ContentItem[]; total?: number; total_pages?: number }>(
				`/admin/content?${params.toString()}`
			);
			if (result.ok) {
				const d = result.data;
				const items = (d?.data ?? []).map((r) => ({ ...r, body_preview: bodyPreview(r.body) }));
				const total = d?.total ?? items.length;
				return { data: items, total, totalPages: d?.total_pages ?? Math.max(1, Math.ceil(total / pageSize)) };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load content'));
		},
		viewSnippet: ContentView
	});
</script>

<svelte:head>
	<title>{t('moderation.content_admin', 'Content Moderation')} — {siteName()} Admin</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- ── State-action confirmation modals ─────────────────────────────────── -->

<!-- Remove -->
<Modal bind:open={showRemoveModal} title={t('content.remove_title', 'Remove Content')}>
	{#if actionTarget}
		<p class="mb-2 text-sm" style="color: var(--color-foreground)">
			{t('content.remove_confirm', 'Remove this content?')} <strong>{actionTarget.title || 'Untitled'}</strong>
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showRemoveModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmRemove} disabled={acting}>{t('content.remove', 'Remove')}</Button>
	</div>
</Modal>

<!-- Restore -->
<Modal bind:open={showRestoreModal} title={t('content.restore_title', 'Restore Content')}>
	{#if actionTarget}
		<p class="mb-2 text-sm" style="color: var(--color-foreground)">
			{t('content.restore_confirm', 'Restore this content?')} <strong>{actionTarget.title || 'Untitled'}</strong>
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showRestoreModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="default" onclick={confirmRestore} disabled={acting}>
			{t('content.restore', 'Restore')}
		</Button>
	</div>
</Modal>

<!-- ── View snippet (mirrors the legacy expanded detail panel) ──────────── -->
{#snippet ContentView({ row }: { row: ContentItem })}
	<div class="space-y-4">
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('moderation.content.title', 'Title')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title || 'Untitled'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('moderation.author', 'Author')}
				</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">
					{row.author_display_name || row.author_username || '—'}
				</p>
				<p class="text-xs font-mono" style="color: var(--color-muted)">{row.author_id?.slice(0, 12)}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('moderation.content_type', 'Content Type')}
				</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.content_type || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('moderation.status', 'Status')}
				</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">
					{row.deleted_at ? t('common.filter_deleted', 'Deleted') : row.status || '—'}
				</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('moderation.reports', 'Reports')}
				</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.reports_count ?? 0}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('moderation.created', 'Created')}
				</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{new Date(row.created_at).toLocaleDateString()}</p>
			</div>
		</div>
		{#if row.body_preview}
			<div class="rounded-md border p-3 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)">
				<p class="mb-1 text-xs font-medium" style="color: var(--color-muted)">
					{t('moderation.content_preview', 'Content Preview')}
				</p>
				{row.body_preview}
			</div>
		{/if}
	</div>
{/snippet}
