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
	import MediaPickerModal from '#lib/components/ui/MediaPickerModal.svelte';
	import { RichTextEditor } from '@japaconnect/ui';
	import { Pencil, Trash2, Send, FileDown, Plus, Sparkles, CheckCircle2, Tags, ArrowUp, ArrowDown, History, Image as ImageIcon, RotateCcw } from 'lucide-svelte';
	import { onMount } from 'svelte';

	const canEdit = $derived(hasPermission('docs.manage'));
	const canDelete = $derived(hasPermission('docs.manage'));
	const canPublish = $derived(hasPermission('docs.manage'));
	const canGenerate = $derived(hasPermission('docs.manage'));
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
	let filterValue = $state<Record<string, string>>({});
	let searchQuery = $state('');

	// Create/Edit Modal
	let showFormModal = $state(false);
	let editingId = $state<string | null>(null);
	let editingOriginalStatus = $state('');
	// Optimistic lock token — sent as expected_updated_at so a stale admin
	// form can't silently overwrite a concurrent docs-site save.
	let editingUpdatedAt = $state('');
	// True once openEdit's detail fetch populated the body — guards submit from
	// writing an empty doc over stored content when the fetch failed.
	let editContentReady = $state(false);
	// The shared TipTap editor edits every stored content shape directly:
	// TipTap docs seed as-is, markdown/plain string bodies parse into
	// structure — no flatten-then-rewrap lossy round-trip (§44 content=jsonb).
	let rte: RichTextEditor | undefined = $state(undefined);
	let editSeed = $state<unknown>(null);
	let editorJson = $state<unknown>(null);
	let pendingImageInsert: ((src: string, alt?: string) => void) | null = null;
	let bodyPickerOpen = $state(false);
	let featurePickerOpen = $state(false);
	let saving = $state(false);
	let form = $state({
		title: '',
		slug: '',
		audience: 'users',
		status: 'draft' as string,
		category: '',
		subcategory: '',
		parentId: '',
		position: 0,
		featureImageId: '',
		featureImageUrl: '',
		summary: ''
	});

	// Revision history modal (§44 amendment 5).
	let showHistoryModal = $state(false);
	let historyRow = $state<DocPage | null>(null);
	let revisions = $state<DocRevision[]>([]);
	let revisionsLoading = $state(false);
	let revisionBusy = $state('');

	// Delete confirmation Modal — shared by single-row and bulk deletes.
	// 'soft' moves to the trash (restorable); 'hard' permanently removes a
	// trashed page (admin.hard_delete-gated ?force=true, Rule 11).
	let showDeleteConfirm = $state(false);
	let deleteMode = $state<'soft' | 'hard'>('soft');
	let deleteTargetIds = $state<string[]>([]);
	let deleteBusy = $state(false);
	// Bulk selection — bound to the DataTable checkboxes.
	let selectedIds = $state<Set<string>>(new Set());

	// DOC-009: audience/status options are loaded from the API.
	let audiences = $state<string[]>(['users', 'premium', 'organizations', 'moderators', 'api', 'admin', 'superadmin']);
	let statuses = $state<string[]>(['draft', 'in_review', 'approved', 'published']);

	// §44 amendment 2: PATCH may only set draft/in_review — approve/publish are
	// dedicated endpoints (and record the reviewer). Editors with docs.manage
	// can still *select* approved/published here — submit() chains the
	// dedicated endpoints after the write so a logged-in admin can approve or
	// publish directly from the form. Backward transitions (e.g. published →
	// approved) are not expressible via PATCH, so they're not offered.
	const statusOptions = $derived.by(() => {
		const current = editingId ? editingOriginalStatus : '';
		const forward: string[] = [];
		if (canPublish) {
			if (current !== 'published' && current !== 'approved') forward.push('approved');
			if (current !== 'published') forward.push('published');
		}
		const all = [...new Set([...statuses.filter((s) => s === 'draft' || s === 'in_review'), ...(current ? [current] : []), ...forward])];
		return all.map((s) => ({
			value: s,
			label: t(`docs.status.${s}`, s.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase()))
		}));
	});

	const audienceOptions = $derived(audiences.map((a) => ({ value: a, label: a })));
	// Filter panel — all four statuses (the form's statusOptions is a
	// reduced, forward-only list; the review-queue filter needs all of them).
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
	// the union of all known subs. Shared by the create/edit form and the
	// AI-generate form — pass the currently selected category value.
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
	const subcategorySuggestions = $derived(subSuggestionsFor(form.category));
	const generateSubSuggestions = $derived(subSuggestionsFor(generateCategory));

	const toItems = (list: string[]): AutocompleteItem[] => list.map((s) => ({ id: s, label: s }));
	const filterItems = (list: string[], q: string) =>
		Promise.resolve(toItems(list.filter((s) => s.toLowerCase().includes(q.toLowerCase()))));

	const categoryFetcher = (q: string) => filterItems(categorySuggestions, q);
	const categoryInitial = () => Promise.resolve(toItems(categorySuggestions));
	const subcategoryFetcher = (q: string) => filterItems(subcategorySuggestions, q);
	const subcategoryInitial = () => Promise.resolve(toItems(subcategorySuggestions));
	const generateSubFetcher = (q: string) => filterItems(generateSubSuggestions, q);
	const generateSubInitial = () => Promise.resolve(toItems(generateSubSuggestions));

	// Parent page select — siblings in the same audience (the backend enforces
	// audience match in validateParent), minus the page being edited.
	const parentPages = $derived(docs.filter((d) => d.audience === form.audience && d.id !== editingId));
	const parentFetcher = (q: string) =>
		Promise.resolve(
			parentPages
				.filter((d) => d.title.toLowerCase().includes(q.toLowerCase()))
				.map((d) => ({ id: d.id, label: d.title, sublabel: d.slug }))
		);
	const parentInitial = () =>
		Promise.resolve(parentPages.map((d) => ({ id: d.id, label: d.title, sublabel: d.slug })));

	// Changing the audience invalidates the parent choice — the backend
	// requires the parent to live in the same audience (validateParent).
	$effect(() => {
		if (form.parentId && !parentPages.some((p) => p.id === form.parentId)) {
			form.parentId = '';
		}
	});

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
		{ key: 'status', label: t('docs.status'), sortable: true, render: (v: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs ${v === 'published' ? 'bg-success/15 text-success dark:text-success' : 'bg-warning/15 text-warning dark:text-warning'}">${v}</span>` },
		{ key: 'updated_at', label: t('docs.updated'), sortable: true, render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v}</span>` }
	];

	const rowActions: ActionItem[] = [
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
			label: t('docs.move_up', 'Move up'),
			icon: ArrowUp,
			onclick: (row) => movePage(row, -1),
			show: (row: DocPage) => canEdit && !row.deleted_at && siblingsOf(row).findIndex((d) => d.id === row.id) > 0
		},
		{
			label: t('docs.move_down', 'Move down'),
			icon: ArrowDown,
			onclick: (row) => movePage(row, 1),
			show: (row: DocPage) => {
				const sibs = siblingsOf(row);
				return canEdit && !row.deleted_at && sibs.findIndex((d) => d.id === row.id) < sibs.length - 1;
			}
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

	const filteredDocs = $derived.by(() => {
		const source = docs;
		const q = searchQuery.toLowerCase().trim();
		let result = source;
		if (filterValue.audience) result = result.filter((d) => d.audience === filterValue.audience);
		if (filterValue.status) result = result.filter((d) => d.status === filterValue.status);
		if (q) result = result.filter((d) => d.title.toLowerCase().includes(q) || d.slug.toLowerCase().includes(q));
		return result;
	});

	function slugify(text: string): string {
		return text.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
	}

	// Audience/status filters are URL-synced like the table's q/sort/page —
	// a filtered review queue stays shareable and back-nav restores it.
	function syncFilterUrl() {
		const url = new URL(pageState.url.href);
		for (const key of ['audience', 'status', 'deleted'] as const) {
			if (filterValue[key]) url.searchParams.set(key, filterValue[key]);
			else url.searchParams.delete(key);
		}
		replaceState(url, pageState.state);
	}

	function applyFilters() {
		// The $effect below re-runs loadDocs when filterValue changes — only
		// the URL sync is needed here.
		syncFilterUrl();
	}

	onMount(async () => {
		// Seed filters from the URL (DataTable restores q/sort/page itself;
		// filters are page-managed state).
		const sp = pageState.url.searchParams;
		filterValue = {
			...(sp.get('audience') ? { audience: sp.get('audience')! } : {}),
			...(sp.get('status') ? { status: sp.get('status')! } : {}),
			...(sp.get('deleted') === '1' || sp.get('deleted') === 'true' ? { deleted: 'true' } : {})
		};

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

	async function loadDocs() {
		loading = true;
		errorMessage = '';
		const params = new URLSearchParams();
		if (filterValue.audience) params.set('audience', filterValue.audience);
		if (filterValue.status) params.set('status', filterValue.status);
		if (filterValue.deleted === 'true') params.set('deleted', '1');
		const result = await docsApi.list(params.toString());
		if (result.ok) {
			const raw = result.data as any[] | { data: any[] };
			const rows = Array.isArray(raw) ? raw : (raw?.data ?? []);
			docs = rows.map((d: any) => ({ ...d, status: d.review_status ?? d.status ?? 'draft', body: d.body ?? d.content ?? '' }));
			// Selection can't outlive the dataset — stale ids would bulk-act on
			// rows the admin can no longer see.
			selectedIds = new Set();
		} else {
			errorMessage = result.error;
		}
		loading = false;
	}

	function openCreate() {
		editingId = null;
		editingOriginalStatus = '';
		editingUpdatedAt = '';
		editContentReady = true; // nothing to fetch
		editSeed = null;
		editorJson = null;
		form = { title: '', slug: '', audience: 'users', status: 'draft', category: '', subcategory: '', parentId: '', position: 0, featureImageId: '', featureImageUrl: '', summary: '' };
		showFormModal = true;
	}

	async function openEdit(item: DocPage) {
		editingId = item.id;
		editingOriginalStatus = item.status;
		editingUpdatedAt = item.updated_at;
		editContentReady = false;
		editSeed = null;
		editorJson = null;
		form = {
			title: item.title,
			slug: item.slug,
			audience: item.audience,
			status: item.status,
			category: item.category ?? '',
			subcategory: item.subcategory ?? '',
			parentId: item.parent_id ?? '',
			position: item.position ?? 0,
			featureImageId: item.feature_image_id ?? '',
			featureImageUrl: item.feature_image_url ?? '',
			summary: ''
		};
		showFormModal = true;
		// The list row is a summary (no content body, §44) — fetch the full page
		// so editing doesn't clobber stored TipTap content with an empty doc.
		const detail = await docsApi.get(item.id);
		if (detail.ok && detail.data && editingId === item.id) {
			const d = detail.data as any;
			// Seed the TipTap editor with the stored body as-is — TipTap docs,
			// markdown strings, and plain text all parse natively.
			editSeed = d.content ?? null;
			form.category = d.category ?? form.category;
			form.subcategory = d.subcategory ?? form.subcategory;
			form.parentId = d.parent_id ?? '';
			form.position = d.position ?? form.position;
			form.featureImageId = d.feature_image_id ?? '';
			form.featureImageUrl = d.feature_image_url ?? '';
			editingUpdatedAt = d.updated_at ?? editingUpdatedAt;
			editContentReady = true;
		}
	}

	async function submit() {
		if (!form.title.trim()) return;
		saving = true;
		errorMessage = '';
		// Flush any pending editor transaction so the JSON reflects the latest
		// keystroke before it ships.
		rte?.flush();
		const slug = form.slug.trim() || slugify(form.title);
		// §44 amendment 2: only draft/in_review are writable via create/PATCH.
		// Selecting approved/published omits review_status from the write and
		// chains the dedicated approve/publish endpoints afterwards so reviewer
		// metadata stays truthful. Empty category/subcategory/parent_id clear
		// the stored value (preserve-on-absent → send "" to clear).
		const desired = form.status;
		const writable = desired === 'approved' || desired === 'published' ? undefined : desired;
		// editorJson tracks every edit (the editor also fires it on seed) —
		// falling back to editSeed covers the untouched case.
		const content = editingId
			? editContentReady
				? (editorJson ?? editSeed ?? '')
				: undefined
			: (editorJson ?? '');
		const payload = {
			title: form.title.trim(),
			slug,
			audience: form.audience,
			// On update without a successful detail fetch, omit content so the
			// stored document is preserved rather than overwritten with an
			// empty doc.
			content,
			review_status: writable,
			category: form.category.trim(),
			subcategory: form.subcategory.trim(),
			parent_id: form.parentId,
			position: Math.max(0, Math.trunc(form.position) || 0),
			feature_image_id: form.featureImageId,
			...(form.summary.trim() ? { summary: form.summary.trim() } : {}),
			// Optimistic lock — the docs-site editor uses the same contract.
			...(editingId && editingUpdatedAt ? { expected_updated_at: editingUpdatedAt } : {})
		};
		const result = editingId ? await docsApi.update(editingId, payload) : await docsApi.create(payload);
		if (result.ok) {
			// Chain the dedicated status endpoints for forward transitions.
			// Both endpoints are idempotent for their target state, so call only
			// when the transition is actually needed.
			const newId = editingId ?? (result.data as { id?: string })?.id;
			const prior = editingId ? editingOriginalStatus : '';
			if (newId && desired === 'approved' && prior !== 'approved' && prior !== 'published') {
				const r = await docsApi.approve(newId);
				if (!r.ok) errorMessage = t('docs.saved_transition_failed', `Saved, but approve failed: ${r.error}`);
			} else if (newId && desired === 'published' && prior !== 'published') {
				if (prior !== 'approved') {
					const r = await docsApi.approve(newId);
					if (!r.ok) errorMessage = t('docs.saved_transition_failed', `Saved, but approve failed: ${r.error}`);
				}
				if (!errorMessage) {
					const r = await docsApi.publish(newId);
					if (!r.ok) errorMessage = t('docs.saved_transition_failed', `Saved, but publish failed: ${r.error}`);
				}
			}
			showFormModal = false;
			loadDocs();
		} else {
			errorMessage = result.error;
		}
		saving = false;
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
	function siblingsOf(item: DocPage): DocPage[] {
		const sameBucket = (d: DocPage) =>
			d.audience === item.audience &&
			(d.parent_id ?? null) === (item.parent_id ?? null) &&
			(d.category ?? null) === (item.category ?? null) &&
			(d.subcategory ?? null) === (item.subcategory ?? null);
		return docs
			.filter(sameBucket)
			.sort((a, b) => (a.position ?? 0) - (b.position ?? 0) || a.title.localeCompare(b.title));
	}

	async function movePage(item: DocPage, dir: -1 | 1) {
		const sibs = siblingsOf(item);
		const idx = sibs.findIndex((d) => d.id === item.id);
		const neighbor = sibs[idx + dir];
		if (!neighbor) return;
		errorMessage = '';
		const a = item.position ?? idx;
		const b = neighbor.position ?? idx + dir;
		// Ties need a synthesized gap — but never write a negative position:
		// a tied move-up at position 0 instead pushes the neighbor down.
		let itemPos = a === b ? a + dir : b;
		let neighborPos = a;
		if (itemPos < 0) {
			itemPos = a;
			neighborPos = a + 1;
		}
		const [r1, r2] = await Promise.all([
			docsApi.update(item.id, { position: itemPos }),
			docsApi.update(neighbor.id, { position: neighborPos })
		]);
		if (!r1.ok) errorMessage = r1.error;
		else if (!r2.ok) errorMessage = r2.error;
		loadDocs();
	}

	// Revision history — §44 amendment 5. Restoring creates a new forward
	// revision (never rewrites history); the latest snapshot can't be deleted.
	async function openHistory(item: DocPage) {
		historyRow = item;
		revisions = [];
		revisionsLoading = true;
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

	// Trash view: bulk actions swap to restore + permanent delete (Rule 11).
	const bulkActions = $derived.by((): BulkAction[] => {
		if (filterValue.deleted === 'true') {
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
		errorMessage = '';
		const result = await docsApi.restore(id);
		if (result.ok) {
			loadDocs();
		} else {
			errorMessage = result.error;
		}
	}

	async function bulkRestore(ids: Set<string>) {
		const idArr = [...ids];
		if (idArr.length === 0) return;
		errorMessage = '';
		const results = await Promise.all(idArr.map((id) => docsApi.restore(id)));
		const failed = results.filter((r) => !r.ok).length;
		selectedIds = new Set();
		if (failed > 0) {
			errorMessage = t('docs.bulk_action_partial', '{n} of {total} actions failed').replace('{n}', String(failed)).replace('{total}', String(idArr.length));
		}
		loadDocs();
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
	async function pollForGeneratedPage(slug: string, audience: string) {
		for (let i = 0; i < 12; i++) {
			await new Promise((r) => setTimeout(r, 3000));
			const res = await docsApi.list(`audience=${encodeURIComponent(audience)}`);
			if (res.ok) {
				const raw = res.data as any[] | { data?: any[] };
				const rows = Array.isArray(raw) ? raw : (raw?.data ?? []);
				if (rows.some((d) => d.slug === slug)) {
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
	description={t('docs.manage_documentation_pages_for_developer')}
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
	data={filteredDocs}
	{loading}
	searchable
	onsearch={(q: string) => { searchQuery = q; }}
	filters={[
		{ key: 'audience', label: t('docs.audience'), options: audienceOptions },
		{ key: 'status', label: t('docs.status'), options: statusFilterOptions },
		{ key: 'deleted', label: t('docs.deleted', 'Deleted'), options: [{ value: 'true', label: t('docs.deleted_only', 'In trash') }] }
	]}
	bind:filterValue
	onfilterapply={applyFilters}
	onrowclick={(row: DocPage) => { if (!row.deleted_at) openEdit(row); }}
	pageSize={25}
	emptyMessage={t('docs.no_documentation_pages_found')}
	actions={rowActions}
	selectable={canDelete}
	bind:selectedIds
	bulkActionsList={bulkActions}
/>

<!-- Create/Edit Modal -->
<Modal bind:open={showFormModal} title={editingId ? t('docs.edit_doc_page') : t('docs.new_doc_page')} size="lg">
	<!-- Submit errors must render inside the modal — the page-level banner
	     sits behind the dialog and is invisible while the form is open. -->
	{#if errorMessage}
		<div class="mb-3 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
			{errorMessage}
		</div>
	{/if}
	<div class="space-y-3">
		<div class="grid gap-3 sm:grid-cols-2">
			<div>
				<label for="doc-title" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.title_2')}</label>
				<input id="doc-title" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.title} oninput={() => { if (!editingId) form.slug = slugify(form.title); }} />
			</div>
			<div>
				<label for="doc-slug" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.slug')}</label>
				<input id="doc-slug" type="text" class="w-full rounded-md border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.slug} />
			</div>
		</div>
		<div class="grid gap-3 sm:grid-cols-2">
			<div>
				<label for="doc-audience" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.audience')}</label>
				<Select options={audienceOptions} bind:value={form.audience} />
			</div>
			<div>
				<label for="doc-status" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.status')}</label>
				<Select options={statusOptions} bind:value={form.status} />
			</div>
		</div>
		<div class="grid gap-3 sm:grid-cols-2">
			<div>
				<label for="doc-category" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.category', 'Category')}</label>
				<Autocomplete
					id="doc-category"
					placeholder={t('docs.category_ph', 'Category (optional)')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={categoryFetcher}
					initialFetcher={categoryInitial}
					selectedId={form.category || null}
					selectedLabel={form.category || null}
					freeText
					allowFreeText
					onSelect={(item) => { form.category = item.label; }}
					onInput={(q) => { form.category = q; }}
					onFreeTextBlur={(q) => { form.category = q.trim(); }}
				/>
			</div>
			<div>
				<label for="doc-subcategory" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.subcategory', 'Subcategory')}</label>
				<Autocomplete
					id="doc-subcategory"
					disabled={!form.category.trim()}
					placeholder={form.category.trim() ? t('docs.subcategory_ph', 'Subcategory (optional)') : t('docs.subcategory_pick_cat', 'Pick a category first')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={subcategoryFetcher}
					initialFetcher={subcategoryInitial}
					selectedId={form.subcategory || null}
					selectedLabel={form.subcategory || null}
					freeText
					allowFreeText
					onSelect={(item) => { form.subcategory = item.label; }}
					onInput={(q) => { form.subcategory = q; }}
					onFreeTextBlur={(q) => { form.subcategory = q.trim(); }}
				/>
			</div>
		</div>
		<div class="grid gap-3 sm:grid-cols-2">
			<div>
				<label for="doc-parent" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.parent_page', 'Parent page')}</label>
				<Autocomplete
					id="doc-parent"
					placeholder={t('docs.parent_ph', '— Top level —')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={parentFetcher}
					initialFetcher={parentInitial}
					selectedId={form.parentId || null}
					selectedLabel={parentPages.find((p) => p.id === form.parentId)?.title ?? null}
					onSelect={(item) => { form.parentId = item.id; }}
					onClear={() => { form.parentId = ''; }}
				/>
			</div>
			<div>
				<label for="doc-position" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.position', 'Position')}</label>
				<input id="doc-position" type="number" min="0" step="1" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.position} />
			</div>
		</div>
		<!-- Feature image (§44 amendment 5) — media-library backed, URL is
		     computed server-side. -->
		<div>
			<span class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.feature_image', 'Feature image')}</span>
			<div class="flex items-center gap-3">
				{#if form.featureImageUrl}
					<img src={form.featureImageUrl} alt="" class="h-14 w-24 rounded-md border object-cover" style="border-color: var(--color-border)" />
				{/if}
				<Button variant="outline" onclick={() => { featurePickerOpen = true; }}>
					<ImageIcon size={14} class="mr-1 inline-block" />
					{form.featureImageId ? t('docs.change_image', 'Change image') : t('docs.choose_image', 'Choose image')}
				</Button>
				{#if form.featureImageId}
					<button type="button" class="text-xs" style="color: var(--color-muted)" onclick={() => { form.featureImageId = ''; form.featureImageUrl = ''; }}>
						{t('common.clear', 'Clear')}
					</button>
				{/if}
			</div>
		</div>
		<div>
			<span class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.body_label', 'Body')}</span>
			{#if editingId && !editContentReady}
				<div class="rounded-md border px-3 py-8 text-center text-xs" style="border-color: var(--color-border); color: var(--color-muted)">
					{t('docs.body_loading', 'Loading page content…')}
				</div>
			{:else}
				<RichTextEditor
					bind:this={rte}
					content={editSeed}
					{t}
					placeholder={t('docs.body_placeholder', 'Write the documentation…')}
					ariaLabel={t('docs.body_label', 'Body')}
					defaultMode="wysiwyg"
					modes={['wysiwyg', 'markdown', 'block']}
					minHeight={320}
					features={{
						image: true,
						youtube: true,
						markdown: true,
						suggestions: false,
						coupon: false,
						linkPreview: false,
						video: false
					}}
					onjson={(json) => { editorJson = json; }}
					onPickImage={(insert) => {
						pendingImageInsert = insert;
						bodyPickerOpen = true;
					}}
				/>
			{/if}
		</div>
		{#if editingId}
			<div>
				<label for="doc-summary" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.summary_label', 'Edit summary (optional)')}</label>
				<input id="doc-summary" type="text" maxlength="200" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.summary} placeholder={t('docs.summary_placeholder', 'Briefly describe your changes')} />
			</div>
		{/if}
	</div>
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={() => { showFormModal = false; }}>{t('docs.cancel')}</Button>
			<Button onclick={submit} disabled={saving}>
				{saving ? t('docs.saving') : (editingId ? t('docs.update') : t('docs.create'))}
			</Button>
		</div>
	{/snippet}
</Modal>

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
		<p class="text-sm" style="color: var(--color-muted)">{t('docs.hard_delete_warning', 'This permanently deletes {n} page(s) — including their revision history — and cannot be undone.').replace('{n}', String(deleteTargetIds.length))}</p>
	{:else}
		<p class="text-sm" style="color: var(--color-muted)">{t('docs.soft_delete_warning', 'This moves {n} page(s) to the trash. You can restore them later from the Deleted filter.').replace('{n}', String(deleteTargetIds.length))}</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => { showDeleteConfirm = false; }}>{t('docs.cancel')}</Button>
		<Button variant="destructive" onclick={executeDelete} disabled={deleteBusy}>{deleteMode === 'hard' ? t('docs.delete_permanently', 'Delete Permanently') : t('docs.confirm_delete')}</Button>
	</div>
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
				<li class="flex items-start justify-between gap-3 py-3">
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
						{#if !rev.is_latest}
							<Button variant="outline" onclick={() => restoreRevision(rev)} disabled={revisionBusy === rev.id}>
								{t('docs.restore', 'Restore')}
							</Button>
							<Button variant="destructive" onclick={() => deleteRevision(rev)} disabled={revisionBusy === rev.id}>
								{t('common.delete', 'Delete')}
							</Button>
						{/if}
					</div>
				</li>
			{/each}
		</ul>
	{/if}
	{#snippet footer()}
		<div class="flex justify-end">
			<Button variant="outline" onclick={() => { showHistoryModal = false; }}>{t('common.close', 'Close')}</Button>
		</div>
	{/snippet}
</Modal>

<!-- In-body image insert (RichTextEditor onPickImage) and feature-image pick -->
<MediaPickerModal
	bind:open={bodyPickerOpen}
	onselect={(media) => {
		pendingImageInsert?.(media.url, media.filename);
		pendingImageInsert = null;
		bodyPickerOpen = false;
	}}
/>
<MediaPickerModal
	bind:open={featurePickerOpen}
	onselect={(media) => {
		form.featureImageId = media.id;
		form.featureImageUrl = media.url;
		featurePickerOpen = false;
	}}
/>
