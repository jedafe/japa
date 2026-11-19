<script lang="ts">
	import { siteName } from '#lib/stores/site.svelte.js';
	// /docs — migrated to shared components (ActionMenu, FilterPanel, Modal,
	// Modal). Replaces the legacy FilterBar.svelte and hand-rolled bits-ui
	// Dialog with shared Modal/Modal. State action: toggle publish.
	import { t } from '#lib/stores/i18n.svelte.js';
	import { hasPermission } from '#lib/stores/permissions.svelte.js';
	import PageHeader from '#lib/components/layout/page-header.svelte';
	import { Select } from '#lib/components/ui/select/index.js';
	import { DataTable } from '#lib/components/ui/data-table/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Modal } from '#lib/components/ui/modal/index.js';
	import type { ActionItem, BulkAction } from '#lib/components/ui/data-table/index.js';
	import { api, docsApi } from '#lib/api.js';
	import { goto, replaceState } from '$app/navigation';
	import { page as pageState } from '$app/state';
	import Autocomplete from '#lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '#lib/components/ui/Autocomplete.svelte';
	import { RichTextEditor } from '@japaconnect/ui';
	import { Pencil, Trash2, Send, FileDown, Plus, Sparkles, CheckCircle2, Tags, ArrowUp, ArrowDown, History, RotateCcw, Eye, Eraser } from 'lucide-svelte';
	import { onMount } from 'svelte';

	const canEdit = $derived(hasPermission('docs.admin'));
	const canDelete = $derived(hasPermission('docs.admin'));
	const canPublish = $derived(hasPermission('docs.admin'));
	const canGenerate = $derived(hasPermission('docs.admin'));
	// Rule 11: ?force=true hard deletes are gated by a dedicated node —
	// super_admin by default, assignable to selected groups.
	const canHardDelete = $derived(hasPermission('admin.hard_delete'));

	type DocPage = {
		id: string;
		title: string;
		slug: string;
		audience: string;
		status: 'draft' | 'in_review' | 'approved' | 'published';
		body: string;
		updated_at: string;
		category?: string | null;
		subcategory?: string | null;
		parent_id?: string | null;
		position?: number;
		ai_generated?: boolean;
		feature_image_id?: string | null;
		feature_image_url?: string | null;
		deleted_at?: string | null;
	};

	type DocRevision = {
		id: string;
		doc_page_id: string;
		version: number;
		title: string;
		summary?: string | null;
		created_by?: string | null;
		created_at: string;
		excerpt: string;
		is_latest: boolean;
	};

	let docs = $state<DocPage[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	// FilterPanel binds filterValue LIVE (per-select), so it can't drive
	// fetches — every dropdown change would fire a request before Apply.
	// appliedFilters is the staged snapshot the API actually sees.
	let filterValue = $state<Record<string, string>>({});
	let appliedFilters = $state<Record<string, string>>({});
	let searchQuery = $state('');
	// Server-side table state — every change re-fetches the current slice.
	let tblPage = $state(0);
	let tblPageSize = $state(25);
	let sortKey = $state('');
	let sortDir = $state<'asc' | 'desc'>('asc');
	let serverTotal = $state(0);
	let serverPages = $state(1);
	// Timestamp of the last AI-generate enqueue — the poll only counts a
	// matching slug as "arrived" when the row is AI-generated AND created
	// after the request (a pre-existing page isn't a false positive).
	let generateStartedAt = '';

	// Revision history modal (§44 amendment 5).
	let showHistoryModal = $state(false);
	let historyRow = $state<DocPage | null>(null);
	let revisions = $state<DocRevision[]>([]);
	let revisionsLoading = $state(false);
	let revisionBusy = $state('');
	// Inline revision preview + clear-history confirm state.
	let expandedRevId = $state('');
	let revViewContent = $state<unknown>(null);
	let revViewLoading = $state(false);
	let showClearConfirm = $state(false);
	let clearBusy = $state(false);

	// Read-only view modal — the spec's View affordance. Row-click opens this
	// for admins without docs.admin (edit form stays docs.admin-gated).
	let showViewModal = $state(false);
	let viewRow = $state<(DocPage & { content?: unknown; created_at?: string; last_reviewed_at?: string | null }) | null>(null);
	let viewLoading = $state(false);

	// Delete confirmation Modal — shared by single-row and bulk deletes.
	// 'soft' moves to the trash (restorable); 'hard' permanently removes a
	// trashed page (admin.hard_delete-gated ?force=true, Rule 11).
	let showDeleteConfirm = $state(false);
	let deleteMode = $state<'soft' | 'hard'>('soft');
	let deleteTargetIds = $state<string[]>([]);
	let deleteBusy = $state(false);
	let restoreBusy = $state(false);
	// Bulk selection — bound to the DataTable checkboxes.
	let selectedIds = $state<Set<string>>(new Set());

	// DOC-009: audience/status options are loaded from the API.
	let audiences = $state<string[]>(['users', 'premium', 'organizations', 'moderators', 'api', 'admin', 'superadmin']);
	let statuses = $state<string[]>(['draft', 'in_review', 'approved', 'published']);

	const audienceOptions = $derived(audiences.map((a) => ({ value: a, label: a })));
	// Filter panel — all four statuses (the review-queue filter needs the
	// full set, unlike the form's forward-only select).
	const statusFilterOptions = $derived(
		statuses.map((s) => ({
			value: s,
			label: t(`docs.status.${s}`, s.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase()))
		}))
	);

	// Category/subcategory suggestions — the managed catalog (docs.categories,
	// public settings) merged with the taxonomy already in use on pages. Page
	// fields stay free-text; these are suggestions only (freeText Autocomplete).
	let managedCats = $state<{ name: string; subcategories: string[] }[]>([]);

	// AI-generate taxonomy state — declared here because the suggestion
	// deriveds below read it (block-scoped ordering).
	let generateCategory = $state('');
	let generateSubcategory = $state('');

	const categorySuggestions = $derived.by(() => {
		const seen = new Map<string, string>();
		for (const c of managedCats.map((m) => m.name)) if (c.trim()) seen.set(c.toLowerCase(), c);
		for (const d of docs) if (d.category?.trim()) seen.set(d.category.toLowerCase(), d.category.trim());
		return [...seen.values()];
	});

	// Subcategory suggestions follow the selected category: its managed subs
	// plus subs already used by pages in that category. No category selected →
	// the union of all known subs. Used by the AI-generate form.
	function subSuggestionsFor(category: string): string[] {
		const cat = category.trim().toLowerCase();
		const seen = new Map<string, string>();
		const managedSubs = cat
			? (managedCats.find((c) => c.name.toLowerCase() === cat)?.subcategories ?? [])
			: managedCats.flatMap((c) => c.subcategories);
		for (const s of managedSubs) if (s.trim()) seen.set(s.toLowerCase(), s);
		for (const d of docs) {
			if (cat && d.category?.trim().toLowerCase() !== cat) continue;
			if (d.subcategory?.trim()) seen.set(d.subcategory.toLowerCase(), d.subcategory.trim());
		}
		return [...seen.values()];
	}
	const generateSubSuggestions = $derived(subSuggestionsFor(generateCategory));

	const toItems = (list: string[]): AutocompleteItem[] => list.map((s) => ({ id: s, label: s }));
	const filterItems = (list: string[], q: string) =>
		Promise.resolve(toItems(list.filter((s) => s.toLowerCase().includes(q.toLowerCase()))));

	const categoryFetcher = (q: string) => filterItems(categorySuggestions, q);
	const categoryInitial = () => Promise.resolve(toItems(categorySuggestions));
	const generateSubFetcher = (q: string) => filterItems(generateSubSuggestions, q);
	const generateSubInitial = () => Promise.resolve(toItems(generateSubSuggestions));

	// The categories/subcategories catalog lives on the /docs/categories
	// subpage. It writes the docs.categories setting, which requires the
	// admin.settings permission — gate the entry point the same way.
	const canManageCategories = $derived(hasPermission('admin.settings'));

	// AI generation modal state (DOC-010)
	let showGenerateModal = $state(false);
	let generateTitle = $state('');
	let generateSlug = $state('');
	let generatePrompt = $state('');
	let generateAudience = $state('users');
	let generateRelated = $state('');
	let generateLoading = $state(false);
	let generateError = $state('');
	let generateSuccess = $state('');

	const columns = [
		{
			key: 'title',
			label: t('docs.title_2'),
			sortable: true,
			// HITL visibility: AI-generated drafts carry a chip so reviewers can
			// tell machine drafts from human pages in the review queue (§44).
			render: (v: string, row: DocPage) =>
				`${v}${row.ai_generated ? ` <span class="ml-1 inline-block rounded-full px-1.5 py-0.5 align-middle text-[10px] font-semibold uppercase tracking-wide" style="background: color-mix(in srgb, var(--color-primary) 12%, transparent); color: var(--color-primary)">${t('docs.ai_chip', 'AI')}</span>` : ''}`
		},
		{ key: 'slug', label: t('docs.slug'), sortable: true, render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v}</span>` },
		{ key: 'audience', label: t('docs.audience'), sortable: true, render: (v: string) => `<span class="inline-block rounded-full bg-black/5 px-2 py-0.5 text-xs dark:bg-surface/10">${v}</span>` },
		{ key: 'status', label: t('docs.status'), sortable: true, render: (v: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs ${STATUS_BADGE[v] ?? 'bg-black/5'}" style="color: var(--color-text)">${v}</span>` },
		{ key: 'updated_at', label: t('docs.updated'), sortable: true, render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${fmtDate(v)}</span>` }
	];

	// Status badge palette — the review queue needs approved distinct from
	// draft/in_review at a glance (all three were the same warning hue).
	const STATUS_BADGE: Record<string, string> = {
		draft: 'bg-black/5 dark:bg-surface/10',
		in_review: 'bg-warning/15 text-warning dark:text-warning',
		approved: 'bg-info/15 text-info dark:text-info',
		published: 'bg-success/15 text-success dark:text-success'
	};

	const fmtDate = (v: string) => {
		const d = new Date(v);
		return Number.isNaN(d.getTime()) ? v : d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
	};

	const rowActions: ActionItem[] = [
		{
			label: t('common.view', 'View'),
			icon: Eye,
			onclick: (row) => openView(row),
			show: (row: DocPage) => !row.deleted_at
		},
		{
			label: t('common.edit', 'Edit'),
			icon: Pencil,
			onclick: (row) => openEdit(row),
			show: (row: DocPage) => canEdit && !row.deleted_at
		},
		{
			label: t('docs.history', 'History'),
			icon: History,
			onclick: (row) => openHistory(row),
			show: (row: DocPage) => canEdit && !row.deleted_at
		},
		{
			label: t('docs.approve', 'Approve'),
			icon: CheckCircle2,
			onclick: (row) => approvePage(row),
			show: (row: DocPage) => canPublish && !row.deleted_at && (row.status === 'draft' || row.status === 'in_review')
		},
		{
			label: t('docs.publish', 'Publish'),
			icon: Send,
			onclick: (row) => togglePublish(row),
			show: (row: DocPage) => canPublish && !row.deleted_at && row.status === 'approved'
		},
		{
			label: t('docs.unpublish', 'Unpublish'),
			icon: FileDown,
			onclick: (row) => togglePublish(row),
			show: (row: DocPage) => canPublish && !row.deleted_at && row.status === 'published'
		},
		{
			// Can't prove "has a sibling" from the paginated slice — the real
			// neighbor may be off-page. Always offer the action; movePage
			// reports the edge when the full bucket is fetched.
			label: t('docs.move_up', 'Move up'),
			icon: ArrowUp,
			onclick: (row) => movePage(row, -1),
			show: (row: DocPage) => canEdit && !row.deleted_at
		},
		{
			label: t('docs.move_down', 'Move down'),
			icon: ArrowDown,
			onclick: (row) => movePage(row, 1),
			show: (row: DocPage) => canEdit && !row.deleted_at
		},
		{
			label: t('common.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onclick: (row) => confirmDelete([row.id], 'soft'),
			show: (row: DocPage) => canDelete && !row.deleted_at
		},
		{
			label: t('docs.restore', 'Restore'),
			icon: RotateCcw,
			onclick: (row) => restorePage(row.id),
			show: (row: DocPage) => canDelete && !!row.deleted_at
		},
		{
			label: t('docs.delete_permanently', 'Delete Permanently'),
			icon: Trash2,
			variant: 'destructive',
			onclick: (row) => confirmDelete([row.id], 'hard'),
			show: (row: DocPage) => canHardDelete && !!row.deleted_at
		}
	];

	function onSort(sort: { key: string | null; dir: 'asc' | 'desc' }) {
		sortKey = sort.key ?? '';
		sortDir = sort.dir;
		tblPage = 0;
	}

	function slugify(text: string): string {
		return text.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
	}

	// Audience/status filters are URL-synced like the table's q/sort/page —
	// a filtered review queue stays shareable and back-nav restores it.
	function syncFilterUrl() {
		// $app/state's page.url can lag behind the DataTable's own
		// replaceState writes (sort/dir/q/page) — merging against the laggy
		// copy would silently drop those params. window.location is current.
		const url = new URL(window.location.href);
		for (const key of ['audience', 'status', 'deleted'] as const) {
			if (appliedFilters[key]) url.searchParams.set(key, appliedFilters[key]);
			else url.searchParams.delete(key);
		}
		replaceState(url, pageState.state);
	}

	function applyFilters() {
		// Snapshot the panel's draft — the $effect re-runs loadDocs when
		// appliedFilters changes.
		appliedFilters = { ...filterValue };
		syncFilterUrl();
		tblPage = 0;
	}

	onMount(async () => {
		// Seed filters from the URL (DataTable restores q/sort/page itself;
		// filters are page-managed state).
		const sp = pageState.url.searchParams;
		const seeded = {
			...(sp.get('audience') ? { audience: sp.get('audience')! } : {}),
			...(sp.get('status') ? { status: sp.get('status')! } : {}),
			...(sp.get('deleted') === '1' || sp.get('deleted') === 'true' ? { deleted: 'true' } : {})
		};
		// Draft and applied state both start from the URL — the table must not
		// fetch unfiltered first and then re-filter.
		filterValue = seeded;
		appliedFilters = { ...seeded };

		// Seed the server-side table state from the URL (q/sort/dir/page/
		// pageSize are the table's shareable params).
		searchQuery = sp.get('q') ?? '';
		if (sp.get('sort')) sortKey = sp.get('sort')!;
		if (sp.get('dir') === 'desc') sortDir = 'desc';
		const urlPage = parseInt(sp.get('page') ?? '1', 10);
		if (urlPage > 1) tblPage = urlPage - 1;
		const urlSize = parseInt(sp.get('pageSize') ?? '', 10);
		if (urlSize > 0) tblPageSize = urlSize;

		const [audResult, statusResult] = await Promise.all([docsApi.audiences(), docsApi.statuses()]);
		if (audResult.ok && Array.isArray(audResult.data) && audResult.data.length > 0) {
			audiences = audResult.data;
		}
		if (statusResult.ok && Array.isArray(statusResult.data) && statusResult.data.length > 0) {
			statuses = statusResult.data;
		}

		// Managed category catalog — public settings, same source the docs
		// portal's authoring forms use for taxonomy suggestions.
		const pub = await api<Record<string, unknown>>('/site/settings/public');
		if (pub.ok) {
			const raw = pub.data?.['docs.categories'];
			if (Array.isArray(raw)) {
				managedCats = raw
					.filter((c): c is { name: string; subcategories?: unknown[] } => !!c && typeof c.name === 'string')
					.map((c) => ({
						name: c.name,
						subcategories: Array.isArray(c.subcategories) ? c.subcategories.filter((s): s is string => typeof s === 'string') : []
					}));
			}
		}
	});

	// Monotonic request id — rapid filter/sort/page changes fire overlapping
	// requests, and a slow earlier response must not clobber newer state.
	let loadSeq = 0;

	async function loadDocs() {
		const seq = ++loadSeq;
		loading = true;
		errorMessage = '';
		const params = new URLSearchParams();
		if (appliedFilters.audience) params.set('audience', appliedFilters.audience);
		if (appliedFilters.status) params.set('status', appliedFilters.status);
		if (appliedFilters.deleted === 'true') params.set('deleted', '1');
		if (searchQuery.trim()) params.set('q', searchQuery.trim());
		if (sortKey) {
			params.set('sort', sortKey);
			params.set('dir', sortDir);
		}
		params.set('limit', String(tblPageSize));
		params.set('offset', String(tblPage * tblPageSize));
		const result = await docsApi.list(params.toString());
		if (seq !== loadSeq) return; // superseded — a newer request owns the table
		if (result.ok) {
			const rows = result.data?.data ?? [];
			serverTotal = result.data?.meta?.total ?? rows.length;
			serverPages = Math.max(result.data?.meta?.total_pages ?? 1, 1);
			docs = rows.map((d: any) => ({ ...d, status: d.review_status ?? d.status ?? 'draft', body: d.body ?? d.content ?? '' }));
			// Selection can't outlive the dataset — stale ids would bulk-act on
			// rows the admin can no longer see.
			selectedIds = new Set();
		} else {
			errorMessage = result.error;
		}
		loading = false;
	}

	// Create/edit live on dedicated routes — the form is too rich for a modal
	// (TipTap + taxonomy + media pickers), matching /spaces' goto("/new") flow.
	// ?from= carries the current list URL (filters/page/sort) so save/cancel
	// returns to the same view — read from window.location because $app/state
	// can lag behind the DataTable's replaceState writes.
	function listReturnTo(): string {
		const u = new URL(window.location.href);
		return u.pathname + u.search;
	}
	function openCreate() {
		goto(`/docs/new?from=${encodeURIComponent(listReturnTo())}`);
	}

	function openEdit(item: DocPage) {
		goto(`/docs/${item.id}?from=${encodeURIComponent(listReturnTo())}`);
	}

	// Read-only detail view (spec: View stays modal, never navigates). The
	// list row is a summary — fetch the detail for the body preview.
	async function openView(item: DocPage) {
		viewRow = item;
		viewLoading = true;
		showViewModal = true;
		const detail = await docsApi.get(item.id);
		if (detail.ok && detail.data && viewRow?.id === item.id) {
			viewRow = { ...item, ...(detail.data as any) };
		}
		viewLoading = false;
	}



	// §44 amendment 2: the review workflow is draft/in_review → approve →
	// publish, enforced server-side. Approve and publish are dedicated calls
	// so reviewer metadata stays truthful.
	async function approvePage(item: DocPage) {
		errorMessage = '';
		const result = await docsApi.approve(item.id);
		if (result.ok) loadDocs();
		else errorMessage = result.error;
	}

	async function togglePublish(item: DocPage) {
		errorMessage = '';
		if (item.status === 'published') {
			// Unpublish: revert to draft via PATCH (no dedicated unpublish endpoint).
			const result = await docsApi.update(item.id, { review_status: 'draft' });
			if (result.ok) loadDocs();
			else errorMessage = result.error;
		} else {
			// Publish: dedicated endpoint — requires the page to be approved
			// first (§44). The row action only shows on approved pages.
			const result = await docsApi.publish(item.id);
			if (result.ok) loadDocs();
			else errorMessage = result.error;
		}
	}

	// Reorder — swap a page's position with its adjacent sibling in the same
	// nav bucket (audience + parent + category + subcategory). The nav tree
	// nests children under parents and groups roots by category, so moving
	// across those boundaries changed a position the reader never sees.
	// Equal positions get a synthesized ±1 offset so the move never no-ops.
	function siblingsOf(item: DocPage, pool: DocPage[] = docs): DocPage[] {
		const sameBucket = (d: DocPage) =>
			d.audience === item.audience &&
			(d.parent_id ?? null) === (item.parent_id ?? null) &&
			(d.category ?? null) === (item.category ?? null) &&
			(d.subcategory ?? null) === (item.subcategory ?? null);
		return pool
			.filter(sameBucket)
			.sort((a, b) => (a.position ?? 0) - (b.position ?? 0) || a.title.localeCompare(b.title));
	}

	async function movePage(item: DocPage, dir: -1 | 1) {
		errorMessage = '';
		// The real position-neighbor may sit outside the current paginated
		// `docs` slice — resolve siblings from the full unfiltered audience
		// list (limit=0), falling back to the loaded page on failure.
		const poolRes = await docsApi.list(`audience=${encodeURIComponent(item.audience)}&limit=0`);
		const pool: DocPage[] = poolRes.ok ? (poolRes.data?.data ?? docs) : docs;
		const sibs = siblingsOf(item, pool);
		const idx = sibs.findIndex((d) => d.id === item.id);
		const neighbor = sibs[idx + dir];
		if (!neighbor) {
			// Edge of the bucket — say why nothing happened instead of a
			// silent no-op.
			errorMessage = t('docs.already_at_edge', 'This page is already at the edge of its section.');
			return;
		}
		// One atomic swap server-side — the old two-PATCH move could leave a
		// half-swapped order if the second update failed.
		const res = await docsApi.move(item.id, neighbor.id);
		if (!res.ok) errorMessage = res.error;
		loadDocs();
	}

	// Revision history — §44 amendment 5. Restoring creates a new forward
	// revision (never rewrites history); the latest snapshot can't be deleted.
	async function openHistory(item: DocPage) {
		historyRow = item;
		revisions = [];
		revisionsLoading = true;
		expandedRevId = '';
		revViewContent = null;
		showHistoryModal = true;
		const res = await docsApi.revisions(item.id);
		if (res.ok) {
			const raw = res.data as DocRevision[] | { data?: DocRevision[] };
			revisions = Array.isArray(raw) ? raw : (raw?.data ?? []);
		} else {
			errorMessage = res.error;
		}
		revisionsLoading = false;
	}

	async function restoreRevision(rev: DocRevision) {
		if (!historyRow || revisionBusy) return;
		revisionBusy = rev.id;
		errorMessage = '';
		const res = await docsApi.restoreRevision(historyRow.id, rev.id);
		if (res.ok) {
			await openHistory(historyRow);
			loadDocs();
		} else {
			errorMessage = res.error;
		}
		revisionBusy = '';
	}

	async function deleteRevision(rev: DocRevision) {
		if (revisionBusy) return;
		revisionBusy = rev.id;
		errorMessage = '';
		const res = await docsApi.deleteRevision(rev.id);
		if (res.ok) {
			revisions = revisions.filter((r) => r.id !== rev.id);
		} else {
			errorMessage = res.error;
		}
		revisionBusy = '';
	}

	// Inline preview of a revision's stored content — restores must not be
	// blind (GET /admin/docs/revisions/{revId} returns the full snapshot).
	async function viewRevision(rev: DocRevision) {
		if (expandedRevId === rev.id) {
			expandedRevId = '';
			revViewContent = null;
			return;
		}
		expandedRevId = rev.id;
		revViewContent = null;
		revViewLoading = true;
		const res = await docsApi.getRevision(rev.id);
		if (res.ok) {
			revViewContent = (res.data as any)?.content ?? null;
		} else {
			errorMessage = res.error;
		}
		revViewLoading = false;
	}

	async function clearHistory() {
		if (!historyRow || clearBusy) return;
		clearBusy = true;
		errorMessage = '';
		const res = await docsApi.clearRevisions(historyRow.id);
		showClearConfirm = false;
		if (res.ok) {
			revisions = [];
		} else {
			errorMessage = res.error;
		}
		clearBusy = false;
	}

	// Trash view: bulk actions swap to restore + permanent delete (Rule 11).
	// Reads the APPLIED filters — the panel draft doesn't reflect what's in
	// the table until Apply is clicked.
	const bulkActions = $derived.by((): BulkAction[] => {
		if (appliedFilters.deleted === 'true') {
			return [
				{ label: t('docs.bulk_restore', 'Restore Selected'), icon: RotateCcw, onclick: (ids) => bulkRestore(ids) },
				...(canHardDelete
					? [
							{
								label: t('docs.bulk_hard_delete', 'Delete Permanently'),
								icon: Trash2,
								variant: 'destructive' as const,
								onclick: (ids: Set<string>) => confirmDelete([...ids], 'hard')
							}
						]
					: [])
			];
		}
		return [
			{
				label: t('docs.bulk_delete', 'Move to Trash'),
				icon: Trash2,
				variant: 'destructive',
				onclick: (ids: Set<string>) => confirmDelete([...ids], 'soft')
			}
		];
	});

	function confirmDelete(ids: string[], mode: 'soft' | 'hard') {
		deleteTargetIds = ids;
		deleteMode = mode;
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (deleteTargetIds.length === 0 || deleteBusy) return;
		deleteBusy = true;
		errorMessage = '';
		const total = deleteTargetIds.length;
		const force = deleteMode === 'hard';
		const results = await Promise.all(deleteTargetIds.map((id) => docsApi.delete(id, { force })));
		const failed = results.filter((r) => !r.ok).length;
		deleteBusy = false;
		showDeleteConfirm = false;
		deleteTargetIds = [];
		selectedIds = new Set();
		if (failed > 0) {
			errorMessage = t('docs.bulk_action_partial', '{n} of {total} actions failed').replace('{n}', String(failed)).replace('{total}', String(total));
		}
		loadDocs();
	}

	async function restorePage(id: string) {
		if (restoreBusy) return;
		restoreBusy = true;
		errorMessage = '';
		try {
			const result = await docsApi.restore(id);
			if (result.ok) {
				loadDocs();
			} else {
				errorMessage = result.error;
			}
		} finally {
			restoreBusy = false;
		}
	}

	async function bulkRestore(ids: Set<string>) {
		const idArr = [...ids];
		if (idArr.length === 0 || restoreBusy) return;
		restoreBusy = true;
		errorMessage = '';
		try {
			const results = await Promise.all(idArr.map((id) => docsApi.restore(id)));
			const failed = results.filter((r) => !r.ok).length;
			selectedIds = new Set();
			if (failed > 0) {
				errorMessage = t('docs.bulk_action_partial', '{n} of {total} actions failed').replace('{n}', String(failed)).replace('{total}', String(idArr.length));
			}
			loadDocs();
		} finally {
			restoreBusy = false;
		}
	}

	function openGenerate() {
		generateTitle = '';
		generateSlug = '';
		generatePrompt = '';
		generateAudience = 'users';
		generateRelated = '';
		generateCategory = '';
		generateSubcategory = '';
		generateError = '';
		generateSuccess = '';
		showGenerateModal = true;
	}

	function slugifyGenerateTitle() {
		if (!generateSlug && generateTitle) {
			generateSlug = generateTitle.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
		}
	}

	function closeGenerate() {
		showGenerateModal = false;
	}

	async function submitGenerate() {
		if (!generateTitle.trim()) return;
		slugifyGenerateTitle();
		generateLoading = true;
		generateError = '';
		generateSuccess = '';
		const related = generateRelated
			.split(',')
			.map((s) => s.trim())
			.filter(Boolean);
		generateStartedAt = new Date().toISOString();
		const result = await docsApi.generate({
			category: 'docs',
			prompt: generatePrompt.trim(),
			context: {
				title: generateTitle.trim(),
				slug: generateSlug.trim(),
				audience: generateAudience,
				related_features: related,
				doc_category: generateCategory.trim(),
				doc_subcategory: generateSubcategory.trim()
			}
		});
		if (result.ok) {
			generateSuccess = t('docs.generate_accepted', 'Generation queued. A draft will appear for HITL review.');
			generatePrompt = '';
			generateRelated = '';
			// Poll for the generated draft — it lands in_review via the worker,
			// not the response. Watch the audience list for the slug.
			pollForGeneratedPage(generateSlug.trim(), generateAudience);
			setTimeout(() => {
				showGenerateModal = false;
				generateSuccess = '';
			}, 2000);
		} else {
			generateError = result.error;
		}
		generateLoading = false;
	}

	// After the queue accepts a generation, poll the admin list until the
	// draft materializes (worker latency is a few seconds) — no silent wait.
	// Only an AI-generated row created after our request counts: a
	// pre-existing page with the same slug is not our draft.
	async function pollForGeneratedPage(slug: string, audience: string) {
		const started = Date.parse(generateStartedAt || new Date().toISOString());
		for (let i = 0; i < 12; i++) {
			await new Promise((r) => setTimeout(r, 3000));
			const res = await docsApi.list(`audience=${encodeURIComponent(audience)}`);
			if (res.ok) {
				const rows = res.data?.data ?? [];
				if (rows.some((d) => d.slug === slug && d.ai_generated && Date.parse(d.created_at ?? '') >= started - 5000)) {
					loadDocs();
					return;
				}
			}
		}
	}

	$effect(() => {
		loadDocs();
	});
</script>

<svelte:head>
	<title>{t('docs.documentation')} — {siteName()} Admin</title>
</svelte:head>

<PageHeader
	title={t('docs.documentation')}
	description={t('docs.admin_documentation_pages_for_developer')}
	actions={[
		...(canManageCategories
			? [{ label: t('docs.categories_title', 'Categories'), icon: Tags, variant: 'outline' as const, onclick: () => goto('/docs/categories') }]
			: []),
		...(canEdit ? [
			{ label: t('docs.new_doc_page'), icon: Plus, onclick: openCreate },
			{ label: t('docs.generate_with_ai'), icon: Sparkles, onclick: openGenerate }
		] : [])
	]}
/>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<DataTable
	columns={columns}
	data={docs}
	{loading}
	searchable
	serverSide
	totalRows={serverTotal}
	totalPages={serverPages}
	bind:page={tblPage}
	bind:pageSize={tblPageSize}
	onpagechange={(p: number) => { tblPage = p; }}
	onpagesizechange={() => { tblPage = 0; }}
	onsortchange={onSort}
	onsearch={(q: string) => { searchQuery = q; tblPage = 0; }}
	filters={[
		{ key: 'audience', label: t('docs.audience'), options: audienceOptions },
		{ key: 'status', label: t('docs.status'), options: statusFilterOptions },
		{ key: 'deleted', label: t('docs.trash', 'Trash'), options: [{ value: 'true', label: t('docs.deleted_only', 'In trash') }] }
	]}
	bind:filterValue
	onfilterapply={applyFilters}
	onrowclick={(row: DocPage) => { if (row.deleted_at) return; if (canEdit) openEdit(row); else openView(row); }}
	pageSizeOptions={[10, 25, 50, 100]}
	emptyMessage={t('docs.no_documentation_pages_found')}
	actions={rowActions}
	selectable={canDelete}
	bind:selectedIds
	bulkActionsList={bulkActions}
/>


<!-- AI Generation Modal (DOC-010) -->
<Modal bind:open={showGenerateModal} title={t('docs.generate_with_ai', 'Generate with AI')} size="lg">
	<div class="space-y-3">
		{#if generateError}
			<div class="rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
				{generateError}
			</div>
		{/if}
		{#if generateSuccess}
			<div class="rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-success); background-color: color-mix(in srgb, var(--color-success) 8%, transparent); color: var(--color-success)">
				{generateSuccess}
			</div>
		{/if}
		<div class="grid gap-3 sm:grid-cols-2">
			<div>
				<label for="generate-title" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.title_2')}</label>
				<input id="generate-title" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={generateTitle} oninput={slugifyGenerateTitle} />
			</div>
			<div>
				<label for="generate-slug" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.slug')}</label>
				<input id="generate-slug" type="text" class="w-full rounded-md border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={generateSlug} />
			</div>
		</div>
		<div>
			<label for="generate-prompt" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.generation_prompt', 'What should the page cover?')}</label>
			<textarea id="generate-prompt" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" rows="6" bind:value={generatePrompt}></textarea>
		</div>
		<div class="grid gap-3 sm:grid-cols-2">
			<div>
				<label for="generate-audience" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.audience')}</label>
				<Select options={audienceOptions} bind:value={generateAudience} />
			</div>
			<div>
				<label for="generate-related" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.related_features', 'Related features (comma separated)')}</label>
				<input id="generate-related" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={generateRelated} />
			</div>
		</div>
		<div class="grid gap-3 sm:grid-cols-2">
			<div>
				<label for="generate-category" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.category', 'Category')}</label>
				<Autocomplete
					id="generate-category"
					placeholder={t('docs.category_ph', 'Category (optional)')}
					emptyMessage={t('docs.no_matches', 'No matches — type to use a new value')}
					fetcher={categoryFetcher}
					initialFetcher={categoryInitial}
					selectedId={generateCategory || null}
					selectedLabel={generateCategory || null}
					freeText
					allowFreeText
					onSelect={(item) => (generateCategory = item.id)}
					onInput={(v) => (generateCategory = v)}
					onFreeTextBlur={(v) => (generateCategory = v.trim())}
					onClear={() => (generateCategory = '')}
				/>
			</div>
			<div>
				<label for="generate-subcategory" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.subcategory', 'Subcategory')}</label>
				<Autocomplete
					id="generate-subcategory"
					disabled={!generateCategory.trim()}
					placeholder={generateCategory ? t('docs.subcategory_ph', 'Subcategory (optional)') : t('docs.subcategory_ph_pick', 'Pick a category first')}
					emptyMessage={t('docs.no_matches', 'No matches — type to use a new value')}
					fetcher={generateSubFetcher}
					initialFetcher={generateSubInitial}
					selectedId={generateSubcategory || null}
					selectedLabel={generateSubcategory || null}
					freeText
					allowFreeText
					onSelect={(item) => (generateSubcategory = item.id)}
					onInput={(v) => (generateSubcategory = v)}
					onFreeTextBlur={(v) => (generateSubcategory = v.trim())}
					onClear={() => (generateSubcategory = '')}
				/>
			</div>
		</div>
	</div>
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={closeGenerate}>{t('docs.cancel')}</Button>
			<Button onclick={submitGenerate} disabled={generateLoading}>
				{generateLoading ? t('docs.generating', 'Generating...') : t('docs.generate', 'Generate')}
			</Button>
		</div>
	{/snippet}
</Modal>

<!-- Delete Confirmation Modal -->
<Modal bind:open={showDeleteConfirm} title={deleteMode === 'hard' ? t('docs.confirm_hard_delete', 'Delete permanently') : t('docs.confirm_delete')} size="sm">
	{#if deleteMode === 'hard'}
		<p class="text-sm" style="color: var(--color-muted)">{deleteTargetIds.length === 1 ? t('docs.hard_delete_warning', 'This permanently deletes this page — including its revision history — and cannot be undone.') : t('docs.hard_delete_warning_many', 'This permanently deletes {n} pages — including their revision history — and cannot be undone.').replace('{n}', String(deleteTargetIds.length))}</p>
	{:else}
		<p class="text-sm" style="color: var(--color-muted)">{deleteTargetIds.length === 1 ? t('docs.soft_delete_warning', 'This moves this page to the trash. You can restore it later from the Deleted filter.') : t('docs.soft_delete_warning_many', 'This moves {n} pages to the trash. You can restore them later from the Deleted filter.').replace('{n}', String(deleteTargetIds.length))}</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => { showDeleteConfirm = false; }}>{t('docs.cancel')}</Button>
		<Button variant="destructive" onclick={executeDelete} disabled={deleteBusy}>{deleteMode === 'hard' ? t('docs.delete_permanently', 'Delete Permanently') : t('docs.confirm_delete')}</Button>
	</div>
</Modal>

<!-- View Modal — read-only detail for admins without docs.admin -->
<Modal bind:open={showViewModal} title={viewRow?.title ?? t('common.view', 'View')} size="lg">
	{#if viewRow}
		<div class="space-y-4">
			<dl class="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2">
				<div><dt class="text-xs font-medium" style="color: var(--color-muted)">{t('docs.slug')}</dt><dd class="font-mono text-xs">{viewRow.slug}</dd></div>
				<div><dt class="text-xs font-medium" style="color: var(--color-muted)">{t('docs.audience')}</dt><dd>{viewRow.audience}</dd></div>
				<div><dt class="text-xs font-medium" style="color: var(--color-muted)">{t('docs.status')}</dt><dd><span class="inline-block rounded-full px-2 py-0.5 text-xs {STATUS_BADGE[viewRow.status] ?? 'bg-black/5'}">{viewRow.status}</span></dd></div>
				<div><dt class="text-xs font-medium" style="color: var(--color-muted)">{t('docs.position', 'Position')}</dt><dd>{viewRow.position ?? 0}</dd></div>
				<div><dt class="text-xs font-medium" style="color: var(--color-muted)">{t('docs.category', 'Category')}</dt><dd>{viewRow.category || '—'}</dd></div>
				<div><dt class="text-xs font-medium" style="color: var(--color-muted)">{t('docs.subcategory', 'Subcategory')}</dt><dd>{viewRow.subcategory || '—'}</dd></div>
				<div><dt class="text-xs font-medium" style="color: var(--color-muted)">{t('docs.updated')}</dt><dd>{fmtDate(viewRow.updated_at)}</dd></div>
				{#if viewRow.last_reviewed_at}
					<div><dt class="text-xs font-medium" style="color: var(--color-muted)">{t('docs.last_reviewed', 'Last reviewed')}</dt><dd>{fmtDate(viewRow.last_reviewed_at)}</dd></div>
				{/if}
			</dl>
			{#if viewRow.feature_image_url}
				<img src={viewRow.feature_image_url} alt="" class="h-24 w-40 rounded-md border object-cover" style="border-color: var(--color-border)" />
			{/if}
			{#if viewLoading}
				<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading', 'Loading…')}</p>
			{:else if viewRow.content}
				<div class="rounded-md border p-3" style="border-color: var(--color-border)">
					<!-- pointer-events-none keeps the shared editor read-only for preview. -->
					<div class="pointer-events-none">
						<RichTextEditor content={viewRow.content} {t} modes={['wysiwyg']} minHeight={120} features={{ image: true, youtube: true, markdown: false, suggestions: false, coupon: false, linkPreview: false, video: false }} />
					</div>
				</div>
			{/if}
		</div>
	{/if}
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={() => { showViewModal = false; }}>{t('common.close', 'Close')}</Button>
			{#if canEdit && viewRow && !viewRow.deleted_at}
				<Button onclick={() => { showViewModal = false; if (viewRow) openEdit(viewRow); }}>{t('common.edit', 'Edit')}</Button>
			{/if}
		</div>
	{/snippet}
</Modal>

<!-- Revision History Modal (§44 amendment 5) -->
<Modal bind:open={showHistoryModal} title={t('docs.history', 'History')} size="lg">
	{#if errorMessage}
		<div class="mb-3 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
			{errorMessage}
		</div>
	{/if}
	{#if revisionsLoading}
		<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading', 'Loading…')}</p>
	{:else if revisions.length === 0}
		<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('docs.no_revisions', 'No revisions recorded for this page.')}</p>
	{:else}
		<ul class="divide-y" style="border-color: var(--color-border)">
			{#each revisions as rev (rev.id)}
				<li class="py-3">
					<div class="flex items-start justify-between gap-3">
						<div class="min-w-0">
							<div class="flex items-center gap-2 text-sm font-medium">
								<span>v{rev.version} — {rev.title}</span>
								{#if rev.is_latest}
									<span class="rounded-full px-1.5 py-0.5 text-[10px] font-semibold uppercase" style="background: color-mix(in srgb, var(--color-success) 15%, transparent); color: var(--color-success)">{t('docs.latest', 'latest')}</span>
								{/if}
							</div>
							{#if rev.summary}
								<p class="mt-0.5 text-xs italic" style="color: var(--color-muted)">{rev.summary}</p>
							{/if}
							{#if rev.excerpt}
								<p class="mt-0.5 line-clamp-2 text-xs" style="color: var(--color-muted)">{rev.excerpt}</p>
							{/if}
							<p class="mt-0.5 text-[11px]" style="color: var(--color-muted)">{new Date(rev.created_at).toLocaleString()}</p>
						</div>
						<div class="flex shrink-0 items-center gap-1.5">
							<Button variant="outline" onclick={() => viewRevision(rev)}>
								<Eye size={13} class="mr-1 inline-block" />{expandedRevId === rev.id ? t('common.hide', 'Hide') : t('common.view', 'View')}
							</Button>
							{#if !rev.is_latest}
								<Button variant="outline" onclick={() => restoreRevision(rev)} disabled={revisionBusy === rev.id}>
									{t('docs.restore', 'Restore')}
								</Button>
								<Button variant="destructive" onclick={() => deleteRevision(rev)} disabled={revisionBusy === rev.id}>
									{t('common.delete', 'Delete')}
								</Button>
							{/if}
						</div>
					</div>
					{#if expandedRevId === rev.id}
						<div class="mt-3 max-h-80 overflow-y-auto rounded-md border p-3" style="border-color: var(--color-border)">
							{#if revViewLoading}
								<p class="py-4 text-center text-xs" style="color: var(--color-muted)">{t('common.loading', 'Loading…')}</p>
							{:else}
								<div class="pointer-events-none">
									<RichTextEditor content={revViewContent} {t} modes={['wysiwyg']} minHeight={100} features={{ image: true, youtube: true, markdown: false, suggestions: false, coupon: false, linkPreview: false, video: false }} />
								</div>
							{/if}
						</div>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
	{#snippet footer()}
		<div class="flex w-full items-center justify-between">
			{#if revisions.length > 0}
				{#if showClearConfirm}
					<span class="flex items-center gap-2 text-xs" style="color: var(--color-danger)">
						{t('docs.clear_history_confirm', 'Delete all revisions for this page?')}
						<Button variant="destructive" onclick={clearHistory} disabled={clearBusy}>{t('common.confirm', 'Confirm')}</Button>
						<Button variant="outline" onclick={() => { showClearConfirm = false; }}>{t('docs.cancel')}</Button>
					</span>
				{:else}
					<Button variant="outline" onclick={() => { showClearConfirm = true; }}>
						<Eraser size={13} class="mr-1 inline-block" />{t('docs.clear_history', 'Clear history')}
					</Button>
				{/if}
			{:else}
				<span></span>
			{/if}
			<Button variant="outline" onclick={() => { showHistoryModal = false; showClearConfirm = false; }}>{t('common.close', 'Close')}</Button>
		</div>
	{/snippet}
</Modal>
