<script lang="ts">
	// /community/story-wall — admin HITL review queue for Story Wall submissions.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { storyAdminApi, apiRaw, adminCatalogApi, type CatalogOption } from '$lib/api';
	import { useCountries } from '$lib/utils/countries.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, Trash2 } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';

	type StoryItem = {
		id: string;
		title: string;
		author_name: string;
		author_id: string;
		status: string;
		is_verified?: boolean;
		deleted_at?: string;
		created_at: string;
		body?: string;
		body_html?: string;
		// ADR-088 AI generation columns
		ai_generated?: boolean;
		ai_model?: string;
		hitl_reviewed_by?: string;
		ai_generated_at?: string;
	};

	const canManage = $derived(hasPermission('story.verify'));
	// Rule 11 amendment: permanent delete requires the dedicated node.
	const canHardDelete = $derived(hasPermission('admin.hard_delete'));

	// ── Country selector (250 primary geo space countries) ───────────────────
	const countryUtils = useCountries();
	$effect(() => {
		countryUtils.load();
	});

	// ── State ───────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);
	let actionTarget = $state<StoryItem | null>(null);
	let showDeleteModal = $state(false);
	// Force-delete (admin.hard_delete) — permanent removal of soft-deleted rows.
	let showForceModal = $state(false);
	let forceTarget = $state<StoryItem | null>(null);

	let storyWallStatusOptions = $state<CatalogOption[]>([
		{ value: 'draft', label: t('app.story.status_draft', 'Draft') },
		{ value: 'pending', label: t('app.story.status_pending', 'Pending') },
		{ value: 'published', label: t('app.story.status_published', 'Published') }
	]);

	$effect(() => {
		void (async () => {
			const res = await adminCatalogApi.get('community.story_wall.status');
			if (res.ok && res.data?.length) storyWallStatusOptions = res.data;
		})();
	});

	function openDelete(item: StoryItem) {
		actionTarget = item;
		showDeleteModal = true;
	}

	async function confirmDelete() {
		if (!actionTarget) return;
		acting = true;
		pageError = '';
		try {
			const result = await storyAdminApi.delete(actionTarget.id);
			if (result.ok) {
				showDeleteModal = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	// Force delete — admin.hard_delete-gated ?force=true on the same endpoint;
	// the backend rejects rows that are not already soft-deleted.
	function openForceDelete(item: StoryItem) {
		forceTarget = item;
		showForceModal = true;
	}

	async function confirmForceDelete() {
		if (!forceTarget) return;
		acting = true;
		pageError = '';
		try {
			const result = await storyAdminApi.hardDelete(forceTarget.id);
			if (result.ok) {
				showForceModal = false;
				forceTarget = null;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	async function verifyStory(item: StoryItem) {
		// ADR-088 HITL gate: AI-generated content cannot be verified without HITL review.
		if (item.ai_generated && !item.hitl_reviewed_by) {
			pageError = t('app.story.hitl_required', 'Cannot verify AI-generated story without HITL review');
			return;
		}
		acting = true;
		pageError = '';
		try {
			const result = await storyAdminApi.verify(item.id);
			if (result.ok) {
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	async function unverifyStory(item: StoryItem) {
		acting = true;
		pageError = '';
		try {
			const result = await storyAdminApi.unverify(item.id);
			if (result.ok) {
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	// ── Row actions ─────────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: StoryItem, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('app.story.verify', 'Verify'),
				icon: CheckCircle,
				onclick: verifyStory,
				show: (row: StoryItem) => row.is_verified !== true && !row.deleted_at
			});
			acts.push({
				label: t('app.story.unverify', 'Unverify'),
				icon: CheckCircle,
				onclick: unverifyStory,
				show: (row: StoryItem) => row.is_verified === true && !row.deleted_at
			});
			acts.push({
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: openDelete,
				show: (row: StoryItem) => !row.deleted_at
			});
		}
		if (canHardDelete) {
			// Permanent removal — only for already-soft-deleted rows.
			acts.push({
				label: t('common.force_delete', 'Force Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: openForceDelete,
				show: (row: StoryItem) => !!row.deleted_at
			});
		}
		return acts;
	});

	const fields: FieldDef[] = [
		{ key: 'title', label: t('app.story.col_title', 'Title'), type: 'text' },
		{ key: 'author_name', label: t('app.story.col_author', 'Author'), type: 'text' },
		{ key: 'status', label: t('app.story.col_status', 'Status'), type: 'text' },
		{ key: 'body', label: t('app.story.detail_content', 'Content'), type: 'textarea' }
	];

	const filters = $derived.by<FilterDef[]>(() => {
		countryUtils.ready;
		return [
			{
				key: 'status',
				label: t('app.story.col_status', 'Status'),
				options: storyWallStatusOptions
			},
			{
				key: 'verified',
				label: t('app.story.col_verified', 'Verified'),
				options: [
					{ value: 'true', label: t('app.story.status_verified', 'Verified') },
					{ value: 'false', label: t('app.story.status_unverified', 'Unverified') }
				]
			},
			{
				key: 'dest',
				label: t('app.story.col_destination', 'Destination'),
				type: 'autocomplete',
				placeholder: t('app.tools.country_placeholder', 'Search country…'),
				emptyMessage: t('common.no_results', 'No results found'),
				fetcher: countryUtils.fetcher,
				initialFetcher: countryUtils.initialFetcher,
				getSelectedLabel: countryUtils.getSelectedLabel
			},
			// Soft-deleted stories are hidden by default — this filter surfaces
			// them so the Force Delete action has targets.
			{
				key: 'deleted',
				label: t('common.filter_deleted', 'Deleted'),
				options: [
					{ value: 'only', label: t('common.filter_deleted_only', 'Deleted only') },
					{ value: 'all', label: t('common.filter_deleted_all', 'Include deleted') }
				]
			}
		];
	});

	const config = $derived<EntityPageConfig>({
		filters,
		title: t('app.story.review_queue_heading', 'Story Wall Review Queue'),
		description: t('app.story.review_queue_desc', 'Review and verify pending Story Wall submissions'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'title', label: t('app.story.col_title', 'Title'), sortable: true },
			{
				key: 'author_name',
				label: t('app.story.col_author', 'Author'),
				sortable: true,
				render: (value: string) => value || t('app.story.unknown', 'Unknown')
			},
			{
				key: 'status',
				label: t('app.story.col_status', 'Status'),
				sortable: true,
				render: (value: string, row: StoryItem) =>
					row.deleted_at
						? `<span class="inline-block rounded-full px-2 py-0.5 text-xs bg-danger/15 text-danger">${t('common.filter_deleted', 'Deleted')}</span>`
						: value || '—'
			},
			{
				key: 'created_at',
				label: t('app.story.col_created_at', 'Created'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async (params) => {
			// Build query string with server-side pagination params.
			const qp = new URLSearchParams();
			if (params.page && params.page > 1) qp.set('page_offset', String((params.page - 1) * (params.pageSize ?? 25)));
			if (params.pageSize) qp.set('page_size', String(params.pageSize));
			if (params.q) qp.set('q', params.q);
			if (params.sort) {
				// EntityPage passes sort as "key:dir" (e.g. "created_at:desc").
				const [sortKey, sortDir] = params.sort.split(':');
				if (sortKey === 'created_at') qp.set('sort', sortDir === 'asc' ? 'oldest' : 'latest');
				else if (sortKey === 'title') qp.set('sort', 'title');
			}
			if (params.filter?.dest) qp.set('dest', params.filter.dest);
			if (params.filter?.status) qp.set('status', params.filter.status);
			if (params.filter?.verified) qp.set('verified', params.filter.verified);
			if (params.filter?.deleted) qp.set('deleted', params.filter.deleted);
			const result = await apiRaw<{ data: StoryItem[]; pagination: { total: number } }>(
				`/admin/stories${qp.toString() ? `?${qp.toString()}` : ''}`
			);
			if (result.ok) {
				const data = (result.data as { data: StoryItem[]; pagination: { total: number } }).data ?? [];
				const total = (result.data as { data: StoryItem[]; pagination: { total: number } }).pagination?.total ?? data.length;
				const totalPages = Math.max(1, Math.ceil(total / (params.pageSize ?? 25)));
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: StoryView
	});
</script>

<svelte:head>
	<title>{t('app.story.review_queue_page_title', 'Story Wall Review Queue — JapaConnect Admin')}</title>
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

<!-- ── Delete confirmation modal ─────────────────────────────────────────── -->
<Modal bind:open={showDeleteModal} title={t('app.story.modal_delete_story', 'Delete Story')}>
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.story.delete_confirm', 'Are you sure you want to delete "{title}"? This cannot be undone.').replace('{title}', actionTarget?.title ?? '')}
		</p>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmDelete} disabled={acting}>
			{acting ? t('app.story.deleting', 'Deleting...') : t('common.delete', 'Delete')}
		</Button>
	</div>
</Modal>

<!-- ── Force Delete confirmation (admin.hard_delete — permanent) ─────────── -->
<Modal bind:open={showForceModal} title={t('common.force_delete', 'Force Delete')} size="sm">
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('common.force_delete_confirm', 'Permanently delete "{name}"? This cannot be undone.').replace('{name}', forceTarget?.title ?? '')}
		</p>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => { showForceModal = false; forceTarget = null; }}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmForceDelete} disabled={acting}>
			{acting ? t('common.deleting', 'Deleting...') : t('common.force_delete', 'Force Delete')}
		</Button>
	</div>
</Modal>

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet StoryView({ row }: { row: StoryItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.story.col_title', 'Title')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.story.col_author', 'Author')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.author_name || t('app.story.unknown', 'Unknown')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.story.col_status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.story.col_created_at', 'Created')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.story.detail_content', 'Content')}</p>
			<p class="mt-1 whitespace-pre-wrap text-sm" style="color: var(--color-foreground)">{row.body || row.body_html || '—'}</p>
		</div>
		{#if row.ai_generated}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.common.ai_generated', 'AI Generated')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{t('common.yes', 'Yes')} ({row.ai_model ?? '—'})</p>
			</div>
		{/if}
		{#if row.hitl_reviewed_by}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.common.hitl_reviewed_by', 'HITL Reviewed By')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.hitl_reviewed_by}</p>
			</div>
		{/if}
	</div>
{/snippet}
