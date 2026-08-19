<script lang="ts">
	// /moderation/content — migrated to the EntityPage scaffold.
	// Distinct moderation-focused view of platform content (reports count,
	// featured badge, type/status filters) with read-only list + Remove/Restore
	// state actions. Uses `contentAdminApi` and the `content.moderate` permission.
	import { EntityPage, type EntityPageConfig, type EntityPageController } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { contentAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, Trash2, RotateCcw } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

	type ContentItem = {
		id: string;
		title: string;
		author: string;
		author_id: string;
		type: string;
		status: string;
		reports_count: number;
		is_featured: boolean;
		created_at: string;
		body_preview?: string;
	};

	// BUG-006: backend admin moderation uses admin.moderation for list and
	// content.delete_any for remove/restore.
	const canModerateContent = $derived(hasPermission('admin.moderation') || hasPermission('content.delete_any'));

	// ── State-action confirmation modals ───────────────────────────────────
	// EntityPage's confirm Modal only covers the default delete; state actions
	// keep page-local Modals, confirm calls the API + controller.reload().
	let entityController = $state<EntityPageController | undefined>(undefined);
	let acting = $state(false);
	let actionTarget = $state<ContentItem | null>(null);

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

	let pageError = $state('');

	// ── Row actions ────────────────────────────────────────────────────────
	// Custom actions replace the defaults, so View is included explicitly.
	// Remove/Restore are gated on `content.moderate` and filtered per-row via show().
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row, ctx) => ctx?.openView(_row) }
		];
		if (canModerateContent) {
			acts.push({
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: openRemove,
				show: (row: ContentItem) => row.status !== 'removed'
			});
			acts.push({
				label: t('moderation.content.restore', 'Restore'),
				icon: RotateCcw,
				onclick: openRestore,
				show: (row: ContentItem) => row.status === 'removed'
			});
		}
		return acts;
	});

	function typeBadge(type: string) {
		switch (type) {
			case 'post': return 'bg-info/15 text-info dark:text-info';
			case 'comment': return 'bg-info/15 text-info dark:text-info';
			case 'blog': return 'bg-danger/15 text-danger dark:text-danger';
			case 'forum': return 'bg-warning/15 text-warning dark:text-warning';
			default: return 'bg-muted/15 text-muted dark:text-muted';
		}
	}

	function statusBadge(status: string) {
		switch (status) {
			case 'published': return 'bg-success/15 text-success dark:text-success';
			case 'draft': return 'bg-muted/15 text-muted dark:text-muted';
			case 'flagged': return 'bg-danger/15 text-danger dark:text-danger';
			case 'removed': return 'bg-muted/30 text-muted dark:text-muted';
			default: return 'bg-warning/15 text-warning dark:text-warning';
		}
	}

	// BUG-108 (Rule 16): type and status options are loaded from the backend
	// so they match the contents table enums and content type catalog.
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
						label: t(`content.type_${ct.value}`, ct.label)
					}));
				}
				if (statusRes.ok) {
					statusOptions = (statusRes.data as { value: string; label: string }[]).map((s) => ({
						value: s.value,
						label: t(`content.status_${s.value}`, s.label)
					}));
				}
			} catch {
				// Keep empty fallback on failure.
			}
		})();
	});

	const config = $derived<EntityPageConfig>({
		title: t('moderation.content_admin', 'Content Moderation'),
		description: t('moderation.content.description', 'Browse and manage platform content across all types.'),
		readonly: true,
		pageSize: 25,
		columns: [
			{
				key: 'title',
				label: t('moderation.title', 'Title'),
				sortable: true,
				render: (_: any, row: ContentItem) =>
					`<div><p class="font-medium">${row.title}</p>${row.is_featured ? '<span class="inline-block rounded-full bg-warning/15 px-2 py-0.5 text-xs text-warning dark:text-warning">' + t('moderation.content.featured', 'Featured') + '</span>' : ''}</div>`
			},
			{
				key: 'author',
				label: t('moderation.author', 'Author'),
				sortable: true,
				render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v}</span>`
			},
			{
				key: 'type',
				label: t('moderation.content_type', 'Content Type'),
				sortable: true,
				render: (v: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs ${typeBadge(v)}">${v}</span>`
			},
			{
				key: 'status',
				label: t('moderation.status', 'Status'),
				sortable: true,
				render: (v: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs ${statusBadge(v)}">${v}</span>`
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
				render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${new Date(v).toLocaleDateString()}</span>`
			}
		],
		filters: [
			{
				key: 'type',
				label: t('moderation.content_type', 'Content Type'),
				options: typeOptions
			},
			{
				key: 'status',
				label: t('moderation.status', 'Status'),
				options: statusOptions
			}
		],
		actions: rowActions,
		fields: [],
		load: async ({ q, filter, pageSize }) => {
			const params = new URLSearchParams();
			params.set('page_size', String(pageSize));
			if (filter.type) params.set('type', filter.type);
			if (filter.status) params.set('status', filter.status);
			if (q) params.set('q', q);
			const result = await contentAdminApi.list(params.toString() || undefined);
			if (result.ok) {
				const d = result.data as { items: ContentItem[]; total?: number; total_pages?: number } | ContentItem[];
				const items = Array.isArray(d) ? d : (d?.items ?? []);
				const total = Array.isArray(d) ? items.length : (d.total ?? items.length);
				const totalPages = Array.isArray(d) ? 1 : (d.total_pages ?? Math.max(1, Math.ceil(total / pageSize)));
				return { data: items, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load content'));
		},
		viewSnippet: ContentView
	});
</script>

<svelte:head>
	<title>{t('moderation.content_admin', 'Content Moderation')} — JapaConnect Admin</title>
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
<Modal bind:open={showRemoveModal} title={t('app.moderation.content.confirm_delete', 'Confirm Delete')}>
	{#if actionTarget}
		<p class="text-sm" style="color: var(--color-muted)">
			{t('moderation.content.are_you_sure_you_want_to_delete_this_con', 'Are you sure you want to delete this content?')}
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showRemoveModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmRemove} disabled={acting}>{t('common.delete', 'Delete')}</Button>
	</div>
</Modal>

<!-- Restore -->
<Modal bind:open={showRestoreModal} title={t('moderation.content.restore_title', 'Restore Content')}>
	{#if actionTarget}
		<p class="text-sm" style="color: var(--color-muted)">
			{t('moderation.content.restore_confirm', 'Restore this content?')}
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showRestoreModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="default" onclick={confirmRestore} disabled={acting}>
			{t('moderation.content.restore', 'Restore')}
		</Button>
	</div>
</Modal>

<!-- ── View snippet (mirrors the legacy expanded detail panel) ──────────── -->
{#snippet ContentView({ row }: { row: ContentItem })}
	<div class="space-y-4">
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('moderation.title', 'Title')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('moderation.author', 'Author')}
				</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.author}</p>
				<p class="text-xs font-mono" style="color: var(--color-muted)">{row.author_id?.slice(0, 12)}</p>
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
