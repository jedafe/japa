<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	import { siteName } from '$lib/stores/site.svelte';
	// /docs — migrated to shared components (ActionMenu, FilterPanel, Modal,
	// SlideOver). Replaces the legacy FilterBar.svelte and hand-rolled bits-ui
	// Dialog with shared Modal/SlideOver. State action: toggle publish.
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Select } from '$lib/components/ui/select';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { api, docsApi } from '$lib/api';
	import { goto } from '$app/navigation';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import { Pencil, Trash2, Send, FileDown, Plus, Sparkles, CheckCircle2, Tags } from 'lucide-svelte';
	import { onMount } from 'svelte';

	const canEdit = $derived(hasPermission('docs.manage'));
	const canDelete = $derived(hasPermission('docs.manage'));
	const canPublish = $derived(hasPermission('docs.manage'));
	const canGenerate = $derived(hasPermission('docs.manage'));

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
	};

	let docs = $state<DocPage[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let filterValue = $state<Record<string, string>>({});
	let searchQuery = $state('');

	// Create/Edit SlideOver
	let showFormSlideOver = $state(false);
	let editingId = $state<string | null>(null);
	let editingOriginalStatus = $state('');
	// True once openEdit's detail fetch populated the body — guards submit from
	// writing an empty doc over stored content when the fetch failed.
	let editContentReady = $state(false);
	let saving = $state(false);
	let form = $state({
		title: '',
		slug: '',
		audience: 'users',
		status: 'draft' as string,
		body: '',
		category: '',
		subcategory: '',
		parentId: '',
		position: 0
	});

	// Delete confirmation Modal
	let showDeleteConfirm = $state(false);
	let deleteTargetId = $state<string | null>(null);

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

	// AI generation slide-over state (DOC-010)
	let showGenerateSlideOver = $state(false);
	let generateTitle = $state('');
	let generateSlug = $state('');
	let generatePrompt = $state('');
	let generateAudience = $state('users');
	let generateRelated = $state('');
	let generateLoading = $state(false);
	let generateError = $state('');
	let generateSuccess = $state('');

	const columns = [
		{ key: 'title', label: t('docs.title_2') },
		{ key: 'slug', label: t('docs.slug'), render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v}</span>` },
		{ key: 'audience', label: t('docs.audience'), render: (v: string) => `<span class="inline-block rounded-full bg-black/5 px-2 py-0.5 text-xs dark:bg-surface/10">${v}</span>` },
		{ key: 'status', label: t('docs.status'), render: (v: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs ${v === 'published' ? 'bg-success/15 text-success dark:text-success' : 'bg-warning/15 text-warning dark:text-warning'}">${v}</span>` },
		{ key: 'updated_at', label: t('docs.updated'), render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v}</span>` }
	];

	const rowActions: ActionItem[] = [
		{
			label: t('common.edit', 'Edit'),
			icon: Pencil,
			onclick: (row) => openEdit(row),
			show: () => canEdit
		},
		{
			label: t('docs.approve', 'Approve'),
			icon: CheckCircle2,
			onclick: (row) => approvePage(row),
			show: (row: DocPage) => canPublish && (row.status === 'draft' || row.status === 'in_review')
		},
		{
			label: t('docs.publish', 'Publish'),
			icon: Send,
			onclick: (row) => togglePublish(row),
			show: (row: DocPage) => canPublish && row.status === 'approved'
		},
		{
			label: t('docs.unpublish', 'Unpublish'),
			icon: FileDown,
			onclick: (row) => togglePublish(row),
			show: (row: DocPage) => canPublish && row.status === 'published'
		},
		{
			label: t('common.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onclick: (row) => confirmDelete(row.id),
			show: () => canDelete
		}
	];

	const filteredDocs = $derived.by(() => {
		const source = docs;
		const q = searchQuery.toLowerCase().trim();
		let result = source;
		if (filterValue.audience) result = result.filter((d) => d.audience === filterValue.audience);
		if (q) result = result.filter((d) => d.title.toLowerCase().includes(q) || d.slug.toLowerCase().includes(q));
		return result;
	});

	function slugify(text: string): string {
		return text.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
	}

	// wrapAsTipTap converts a plain-text body into a minimal TipTap JSON document
	// so the backend can store it as jsonb content (§44 — content is TipTap JSON).
	// Each non-empty line becomes a paragraph node; empty lines become empty paragraphs.
	function wrapAsTipTap(body: string): object {
		const lines = body.split('\n');
		const content = lines.map((line) => ({
			type: 'paragraph',
			content: line.trim()
				? [{ type: 'text', text: line }]
				: []
		}));
		return { type: 'doc', content };
	}

	onMount(async () => {
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
		const result = await docsApi.list(params.toString());
		if (result.ok) {
			const raw = result.data as any[] | { data: any[] };
			const rows = Array.isArray(raw) ? raw : (raw?.data ?? []);
			docs = rows.map((d: any) => ({ ...d, status: d.review_status ?? d.status ?? 'draft', body: d.body ?? d.content ?? '' }));
		} else {
			errorMessage = result.error;
		}
		loading = false;
	}

	function openCreate() {
		editingId = null;
		editingOriginalStatus = '';
		form = { title: '', slug: '', audience: 'users', status: 'draft', body: '', category: '', subcategory: '', parentId: '', position: 0 };
		showFormSlideOver = true;
	}

	// unwrapTipTap flattens stored TipTap JSON back to the plain-text lines the
	// body textarea edits (the inverse of wrapAsTipTap).
	function unwrapTipTap(content: unknown): string {
		try {
			const doc = typeof content === 'string' ? JSON.parse(content) : content;
			if (!doc || typeof doc !== 'object' || !Array.isArray((doc as any).content)) return '';
			return (doc as any).content
				.map((node: any) =>
					Array.isArray(node?.content)
						? node.content.map((c: any) => c?.text ?? '').join('')
						: ''
				)
				.join('\n');
		} catch {
			return '';
		}
	}

	async function openEdit(item: DocPage) {
		editingId = item.id;
		editingOriginalStatus = item.status;
		editContentReady = false;
		form = {
			title: item.title,
			slug: item.slug,
			audience: item.audience,
			status: item.status,
			body: '',
			category: item.category ?? '',
			subcategory: item.subcategory ?? '',
			parentId: item.parent_id ?? '',
			position: item.position ?? 0
		};
		showFormSlideOver = true;
		// The list row is a summary (no content body, §44) — fetch the full page
		// so editing doesn't clobber stored TipTap content with empty text.
		const detail = await docsApi.get(item.id);
		if (detail.ok && detail.data && editingId === item.id) {
			const d = detail.data as any;
			form.body = unwrapTipTap(d.content);
			form.category = d.category ?? form.category;
			form.subcategory = d.subcategory ?? form.subcategory;
			form.parentId = d.parent_id ?? '';
			form.position = d.position ?? form.position;
			editContentReady = true;
		}
	}

	async function submit() {
		if (!form.title.trim()) return;
		saving = true;
		errorMessage = '';
		const slug = form.slug.trim() || slugify(form.title);
		// Wrap the plain-text body in a minimal TipTap JSON document so the
		// backend (which expects jsonb TipTap content) can store and render it.
		// Ref: §44 "content jsonb NOT NULL DEFAULT '{}', -- TipTap JSON"
		const tiptapContent = wrapAsTipTap(form.body);
		// §44 amendment 2: only draft/in_review are writable via create/PATCH.
		// Selecting approved/published omits review_status from the write and
		// chains the dedicated approve/publish endpoints afterwards so reviewer
		// metadata stays truthful. Empty category/subcategory/parent_id clear
		// the stored value (preserve-on-absent → send "" to clear).
		const desired = form.status;
		const writable = desired === 'approved' || desired === 'published' ? undefined : desired;
		const payload = {
			title: form.title.trim(),
			slug,
			audience: form.audience,
			// On update without a successful detail fetch, omit content so the
			// stored document is preserved rather than overwritten with an
			// empty doc.
			content: editingId && !editContentReady ? undefined : tiptapContent,
			review_status: writable,
			category: form.category.trim(),
			subcategory: form.subcategory.trim(),
			parent_id: form.parentId,
			position: Math.max(0, Math.trunc(form.position) || 0)
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
			showFormSlideOver = false;
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

	function confirmDelete(id: string) {
		deleteTargetId = id;
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTargetId) return;
		errorMessage = '';
		const result = await docsApi.delete(deleteTargetId);
		if (result.ok) {
			showDeleteConfirm = false;
			deleteTargetId = null;
			loadDocs();
		} else {
			errorMessage = result.error;
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
		showGenerateSlideOver = true;
	}

	function slugifyGenerateTitle() {
		if (!generateSlug && generateTitle) {
			generateSlug = generateTitle.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
		}
	}

	function closeGenerate() {
		showGenerateSlideOver = false;
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
			setTimeout(() => {
				showGenerateSlideOver = false;
				generateSuccess = '';
			}, 2000);
		} else {
			generateError = result.error;
		}
		generateLoading = false;
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
	filters={[{ key: 'audience', label: t('docs.audience'), options: audienceOptions }]}
	bind:filterValue
	onfilterapply={loadDocs}
	onrowclick={(row: DocPage) => openEdit(row)}
	pageSize={25}
	emptyMessage={t('docs.no_documentation_pages_found')}
	showPagination={false}
	actions={rowActions}
/>

<!-- Create/Edit SlideOver -->
<SlideOver bind:open={showFormSlideOver} title={editingId ? t('docs.edit_doc_page') : t('docs.new_doc_page')} size="lg">
	<div class="space-y-3">
		<div class="grid gap-3 sm:grid-cols-2">
			<div>
				<label for="doc-title" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.title_2')}</label>
				<HintIcon text={t('docs.doc_title_hint', 'The page title shown in docs navigation.')} />
				<input id="doc-title" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.title} oninput={() => { if (!editingId) form.slug = slugify(form.title); }} />
			</div>
			<div>
				<label for="doc-slug" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.slug')}</label>
				<HintIcon text={t('docs.doc_slug_hint', 'URL-safe identifier — auto-generated from the title.')} />
				<input id="doc-slug" type="text" class="w-full rounded-md border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.slug} />
			</div>
		</div>
		<div class="grid gap-3 sm:grid-cols-2">
			<div>
				<label for="doc-audience" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.audience')}</label>
				<HintIcon text={t('docs.doc_audience_hint', 'Who can see this page.')} />
				<Select options={audienceOptions} bind:value={form.audience} />
			</div>
			<div>
				<label for="doc-status" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.status')}</label>
				<HintIcon text={t('docs.doc_status_hint', 'Draft or published state.')} />
				<Select options={statusOptions} bind:value={form.status} />
			</div>
		</div>
		<div class="grid gap-3 sm:grid-cols-2">
			<div>
				<label for="doc-category" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.category', 'Category')}</label>
				<HintIcon text={t('docs.doc_category_hint', 'The top-level section this page lives in.')} />
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
				<HintIcon text={t('docs.doc_subcategory_hint', 'Optional — a nested grouping under the category.')} />
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
				<HintIcon text={t('docs.doc_parent_hint', 'Optional — nest this page under another.')} />
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
				<HintIcon text={t('docs.doc_position_hint', 'Ordering position — lower numbers show first.')} />
				<input id="doc-position" type="number" min="0" step="1" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.position} />
			</div>
		</div>
		<div>
			<label for="doc-body" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.body_markdown')}</label>
			<HintIcon text={t('docs.doc_body_hint', 'The page content.')} />
			<textarea id="doc-body" class="w-full rounded-md border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background-color: var(--color-surface)" rows="12" bind:value={form.body}></textarea>
		</div>
	</div>
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={() => { showFormSlideOver = false; }}>{t('docs.cancel')}</Button>
			<Button onclick={submit} disabled={saving}>
				{saving ? t('docs.saving') : (editingId ? t('docs.update') : t('docs.create'))}
			</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- AI Generation SlideOver (DOC-010) -->
<SlideOver bind:open={showGenerateSlideOver} title={t('docs.generate_with_ai', 'Generate with AI')} size="lg">
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
				<HintIcon text={t('docs.generate_title_hint', 'Title for the generated page.')} />
				<input id="generate-title" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={generateTitle} oninput={slugifyGenerateTitle} />
			</div>
			<div>
				<label for="generate-slug" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.slug')}</label>
				<HintIcon text={t('docs.generate_slug_hint', 'URL-safe identifier for the page.')} />
				<input id="generate-slug" type="text" class="w-full rounded-md border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={generateSlug} />
			</div>
		</div>
		<div>
			<label for="generate-prompt" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.generation_prompt', 'What should the page cover?')}</label>
			<HintIcon text={t('docs.generate_prompt_hint', 'Describe the page the AI should write.')} />
			<textarea id="generate-prompt" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" rows="6" bind:value={generatePrompt}></textarea>
		</div>
		<div class="grid gap-3 sm:grid-cols-2">
			<div>
				<label for="generate-audience" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.audience')}</label>
				<HintIcon text={t('docs.generate_audience_hint', 'Who the generated page targets.')} />
				<Select options={audienceOptions} bind:value={generateAudience} />
			</div>
			<div>
				<label for="generate-related" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.related_features', 'Related features (comma separated)')}</label>
				<HintIcon text={t('docs.generate_related_hint', 'Related pages to link — comma-separated.')} />
				<input id="generate-related" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={generateRelated} />
			</div>
		</div>
		<div class="grid gap-3 sm:grid-cols-2">
			<div>
				<label for="generate-category" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.category', 'Category')}</label>
				<HintIcon text={t('docs.generate_category_hint', 'The section for the generated page.')} />
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
				<HintIcon text={t('docs.generate_subcategory_hint', 'Optional — a nested grouping.')} />
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
</SlideOver>

<!-- Delete Confirmation Modal -->
<Modal bind:open={showDeleteConfirm} title={t('docs.confirm_delete')} size="sm">
	<p class="text-sm" style="color: var(--color-muted)">{t('docs.are_you_sure_you_want_to_delete_this_doc')}</p>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => { showDeleteConfirm = false; }}>{t('docs.cancel')}</Button>
		<Button variant="destructive" onclick={executeDelete}>{t('docs.confirm_delete')}</Button>
	</div>
</Modal>
