<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /roadmap/releases — EntityPage scaffold (§113, ADR-135). Versioned
	// releases table with surface/status filters, soft-delete + hard delete,
	// and a "Change Status" state action driving draft→published→yanked
	// (publish optionally absorbs the unreleased entry queue).
	import { EntityPage, type EntityPageConfig, type EntityPageController } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { roadmapReleasesAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, Pencil, Trash2, Trash, RotateCcw, ArrowRightLeft } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';

	type RoadmapRelease = {
		id: string;
		surface: string;
		version: string;
		slug: string;
		title: string | null;
		summary: string | null;
		body: string | null;
		status: string;
		is_esr: boolean;
		released_at: string | null;
		eol_at: string | null;
		sort_order: number;
		deleted_at: string | null;
	};

	const STATUS_OPTIONS = [
		{ value: 'draft', label: t('roadmap.releases.status.draft', 'Draft') },
		{ value: 'published', label: t('roadmap.releases.status.published', 'Published') },
		{ value: 'yanked', label: t('roadmap.releases.status.yanked', 'Yanked') }
	];

	const SURFACE_OPTIONS = [
		{ value: 'platform', label: t('roadmap.releases.surface.platform', 'Platform') },
		{ value: 'mobile', label: t('roadmap.releases.surface.mobile', 'Mobile') }
	];

	const STATUS_COLORS: Record<string, string> = {
		draft: 'bg-muted/30 text-muted',
		published: 'bg-success/15 text-success',
		yanked: 'bg-danger/15 text-danger'
	};

	const canEdit = $derived(hasPermission('roadmap.edit'));
	const canDelete = $derived(hasPermission('roadmap.delete'));
	const canModerate = $derived(hasPermission('roadmap.moderate'));
	const canHardDelete = $derived(hasPermission('admin.hard_delete'));

	const filterDefs: FilterDef[] = [
		{ key: 'status', label: t('common.status', 'Status'), options: STATUS_OPTIONS },
		{ key: 'surface', label: t('roadmap.releases.surface', 'Surface'), options: SURFACE_OPTIONS }
	];

	// ── State-action confirmation modals ───────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);
	let actionTarget = $state<RoadmapRelease | null>(null);

	let showStatusModal = $state(false);
	let statusChoice = $state('');
	let absorbUnreleased = $state(true);
	let showRestoreConfirm = $state(false);
	let showHardDeleteConfirm = $state(false);

	function openStatusModal(row: RoadmapRelease) {
		actionTarget = row;
		statusChoice = row.status === 'draft' ? 'published' : row.status === 'published' ? 'yanked' : 'published';
		absorbUnreleased = true;
		showStatusModal = true;
	}

	async function confirmStatus() {
		if (!actionTarget || acting) return;
		acting = true;
		pageError = '';
		const res = await roadmapReleasesAdminApi.setStatus(actionTarget.id, statusChoice, absorbUnreleased);
		acting = false;
		if (!res.ok) {
			pageError = res.error ?? t('common.error', 'Request failed');
			return;
		}
		showStatusModal = false;
		entityController?.reload();
	}

	async function confirmRestore() {
		if (!actionTarget || acting) return;
		acting = true;
		const res = await roadmapReleasesAdminApi.restore(actionTarget.id);
		acting = false;
		if (!res.ok) {
			pageError = res.error ?? t('common.error', 'Request failed');
			return;
		}
		showRestoreConfirm = false;
		entityController?.reload();
	}

	async function confirmHardDelete() {
		if (!actionTarget || acting) return;
		acting = true;
		const res = await roadmapReleasesAdminApi.hardDelete(actionTarget.id);
		acting = false;
		if (!res.ok) {
			pageError = res.error ?? t('common.error', 'Request failed');
			return;
		}
		showHardDeleteConfirm = false;
		entityController?.reload();
	}

	const actions: ActionItem[] = [
		{ label: t('common.view', 'View'), icon: Eye, onclick: (row, ctx) => ctx?.openView(row) },
		{
			label: t('common.edit', 'Edit'),
			icon: Pencil,
			onclick: (row, ctx) => ctx?.openEdit(row),
			show: () => canEdit
		},
		{
			label: t('roadmap.change_status', 'Change Status'),
			icon: ArrowRightLeft,
			onclick: (row: RoadmapRelease) => openStatusModal(row),
			show: (row) => canModerate && !row.deleted_at
		},
		{
			label: t('common.restore', 'Restore'),
			icon: RotateCcw,
			onclick: (row: RoadmapRelease) => {
				actionTarget = row;
				showRestoreConfirm = true;
			},
			show: (row) => canDelete && !!row.deleted_at
		},
		{
			label: t('common.delete', 'Delete'),
			icon: Trash2,
			onclick: (row, ctx) => ctx?.openDelete(row),
			variant: 'destructive',
			show: (row) => canDelete && !row.deleted_at
		},
		{
			label: t('common.hard_delete', 'Delete Permanently'),
			icon: Trash,
			onclick: (row: RoadmapRelease) => {
				actionTarget = row;
				showHardDeleteConfirm = true;
			},
			variant: 'destructive',
			show: (row) => canHardDelete && !!row.deleted_at
		}
	];

	const config: EntityPageConfig = {
		title: t('roadmap.releases.title', 'Releases'),
		description: t('roadmap.releases.desc', 'Versioned release lines and publish lifecycle'),
		columns: [
			{ key: 'version', label: t('roadmap.releases.version', 'Version'), sortable: true },
			{
				key: 'title',
				label: t('common.title', 'Title'),
				render: (v: string | null, row: RoadmapRelease) =>
					(v ?? '') +
					(row.deleted_at
						? ` <span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium bg-muted/30 text-muted">${t('common.deleted', 'deleted')}</span>`
						: '') || `<span style="color: var(--color-muted)">—</span>`
			},
			{ key: 'surface', label: t('roadmap.releases.surface', 'Surface'), sortable: true },
			{
				key: 'status',
				label: t('common.status', 'Status'),
				render: (v: string) => {
					const label = STATUS_OPTIONS.find((o) => o.value === v)?.label ?? v;
					const colors = STATUS_COLORS[v] ?? 'bg-muted/30 text-muted';
					return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${colors}">${label}</span>`;
				}
			},
			{
				key: 'released_at',
				label: t('roadmap.releases.released_at', 'Released'),
				sortable: true,
				render: (v: string | null) =>
					v ? new Date(v).toLocaleDateString() : `<span style="color: var(--color-muted)">—</span>`
			},
			{
				key: 'eol_at',
				label: t('roadmap.releases.eol', 'EOL'),
				render: (v: string | null) =>
					v ? new Date(v).toLocaleDateString() : `<span style="color: var(--color-muted)">—</span>`
			}
		],
		filters: filterDefs,
		actions,
		fields: [
			{ key: 'version', label: t('roadmap.releases.version', 'Version'), type: 'text', help: t('roadmap.releases.version_hint', 'Version identifier — e.g. 2.1.0.'), required: true },
			{ key: 'slug', label: t('common.slug', 'Slug'), type: 'text', help: t('common.slug_hint', 'URL-safe identifier — lowercase, hyphens, no spaces.') },
			{ key: 'title', label: t('common.title', 'Title'), type: 'text', help: t('common.title_hint', 'The title shown in listings and detail views.') },
			{ key: 'summary', label: t('common.summary', 'Summary'), type: 'textarea', help: t('common.summary_hint', 'A one or two sentence overview shown on cards.') },
			{ key: 'body', label: t('roadmap.releases.notes', 'Release notes'), type: 'textarea', help: t('roadmap.releases.body_hint', 'The main content body.') },
			{
				key: 'surface',
				label: t('roadmap.releases.surface', 'Surface'),
				type: 'select',
			help: t('roadmap.releases.surface_hint', 'Where this appears — feed, sidebar, email.'),
				options: SURFACE_OPTIONS
			},
			{ key: 'is_esr', label: t('roadmap.releases.is_esr', 'ESR (extended support)'), type: 'checkbox', help: t('roadmap.releases.is_esr_hint', 'Mark as an emergency service resource.'),},
			{ key: 'released_at', label: t('roadmap.releases.released_at', 'Released at'), type: 'datetime-local', help: t('roadmap.releases.released_at_hint', 'When the release shipped.') },
			{ key: 'eol_at', label: t('roadmap.releases.eol', 'EOL at'), type: 'datetime-local', help: t('roadmap.releases.eol_at_hint', 'End-of-life date — support ends after this.') },
			{ key: 'sort_order', label: t('common.sort_order', 'Sort order'), type: 'number', help: t('common.sort_order_hint', 'Lower numbers appear first.') }
		],
		createPermission: 'roadmap.edit',
		editPermission: 'roadmap.edit',
		deletePermission: 'roadmap.delete',
		load: async ({ page, pageSize, filter }) => {
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String((page - 1) * pageSize),
				include_deleted: 'true'
			});
			if (filter?.status) params.set('status', filter.status);
			if (filter?.surface) params.set('surface', filter.surface);
			const res = await roadmapReleasesAdminApi.list(params.toString());
			if (!res.ok) return { data: [], total: 0, totalPages: 0 };
			const d = res.data as { releases: RoadmapRelease[]; total: number; total_pages: number };
			return { data: d.releases ?? [], total: d.total, totalPages: d.total_pages };
		},
		create: async (body) => {
			const res = await roadmapReleasesAdminApi.create(body);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await roadmapReleasesAdminApi.update(id as string, body);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await roadmapReleasesAdminApi.delete(id as string);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		isDeleted: (row: RoadmapRelease) => !!row.deleted_at
	};
</script>

<svelte:head>
	<title>{t('roadmap.releases.title', 'Releases')} — {siteName()}</title>
</svelte:head>

<EntityPage {config} bind:controller={entityController} />

<!-- Lifecycle transition modal (draft→published→yanked) -->
<Modal bind:open={showStatusModal} title={t('roadmap.change_status', 'Change Status')}>
	<div class="space-y-3">
		<p class="text-sm text-muted">
			{actionTarget?.version} · {actionTarget?.status}
		</p>
		<label class="flex flex-col gap-1 text-sm">
			{t('common.status', 'Status')}
			<select bind:value={statusChoice} class="rounded border border-border bg-surface px-2 py-1.5">
				{#each STATUS_OPTIONS as o}
					<option value={o.value}>{o.label}</option>
				{/each}
			</select>
		</label>
		{#if statusChoice === 'published'}
			<label class="flex items-center gap-2 text-sm">
				<input type="checkbox" bind:checked={absorbUnreleased} />
				{t('roadmap.releases.absorb', 'Absorb unreleased changelog entries into this release')}
			</label>
		{/if}
		{#if pageError}
			<p class="text-sm text-danger">{pageError}</p>
		{/if}
		<div class="flex justify-end gap-2">
			<Button variant="ghost" onclick={() => (showStatusModal = false)}>
				{t('common.cancel', 'Cancel')}
			</Button>
			<Button disabled={acting} onclick={confirmStatus}>
				{t('common.confirm', 'Confirm')}
			</Button>
		</div>
	</div>
</Modal>

<Modal bind:open={showRestoreConfirm} title={t('common.restore', 'Restore')}>
	<p class="text-sm">{t('roadmap.releases.restore_confirm', 'Restore this release?')}</p>
	{#if pageError}<p class="mt-2 text-sm text-danger">{pageError}</p>{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showRestoreConfirm = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button disabled={acting} onclick={confirmRestore}>{t('common.restore', 'Restore')}</Button>
	</div>
</Modal>

<Modal bind:open={showHardDeleteConfirm} title={t('common.hard_delete', 'Delete Permanently')}>
	<p class="text-sm text-danger">
		{t('roadmap.releases.hard_delete_confirm', 'Permanently delete this release? This cannot be undone.')}
	</p>
	{#if pageError}<p class="mt-2 text-sm text-danger">{pageError}</p>{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showHardDeleteConfirm = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" disabled={acting} onclick={confirmHardDelete}>
			{t('common.delete', 'Delete')}
		</Button>
	</div>
</Modal>
