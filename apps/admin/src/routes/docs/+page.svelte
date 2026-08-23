<script lang="ts">
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
	import { docsApi } from '$lib/api';
	import { Pencil, Trash2, Send, FileDown, Plus, Sparkles } from 'lucide-svelte';
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
	};

	let docs = $state<DocPage[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let filterValue = $state<Record<string, string>>({});
	let searchQuery = $state('');

	// Create/Edit SlideOver
	let showFormSlideOver = $state(false);
	let editingId = $state<string | null>(null);
	let saving = $state(false);
	let form = $state({ title: '', slug: '', audience: 'users', status: 'draft' as string, body: '' });

	// Delete confirmation Modal
	let showDeleteConfirm = $state(false);
	let deleteTargetId = $state<string | null>(null);

	// DOC-009: audience/status options are loaded from the API.
	let audiences = $state<string[]>(['users', 'premium', 'organizations', 'moderators', 'api', 'admin', 'superadmin']);
	let statuses = $state<string[]>(['draft', 'in_review', 'approved', 'published']);

	const statusOptions = $derived(
		statuses.map((s) => ({
			value: s,
			label: t(`docs.status.${s}`, s.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase()))
		}))
	);

	const audienceOptions = $derived(audiences.map((a) => ({ value: a, label: a })));

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
			label: t('docs.publish', 'Publish'),
			icon: Send,
			onclick: (row) => togglePublish(row),
			show: (row: DocPage) => canPublish && row.status !== 'published'
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
		form = { title: '', slug: '', audience: 'users', status: 'draft', body: '' };
		showFormSlideOver = true;
	}

	function openEdit(item: DocPage) {
		editingId = item.id;
		form = { title: item.title, slug: item.slug, audience: item.audience, status: item.status, body: item.body ?? '' };
		showFormSlideOver = true;
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
		const payload = { title: form.title.trim(), slug, audience: form.audience, review_status: form.status, content: tiptapContent };
		const result = editingId ? await docsApi.update(editingId, payload) : await docsApi.create(payload);
		if (result.ok) {
			showFormSlideOver = false;
			loadDocs();
		} else {
			errorMessage = result.error;
		}
		saving = false;
	}

	async function togglePublish(item: DocPage) {
		errorMessage = '';
		if (item.status === 'published') {
			// Unpublish: revert to draft via PATCH (no dedicated unpublish endpoint).
			const result = await docsApi.update(item.id, { review_status: 'draft' });
			if (result.ok) loadDocs();
			else errorMessage = result.error;
		} else {
			// Publish: use the dedicated publish endpoint (§44 — validates approved status).
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
			context: { title: generateTitle.trim(), slug: generateSlug.trim(), audience: generateAudience, related_features: related }
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
	<title>{t('docs.documentation')} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('docs.documentation')}
	description={t('docs.manage_documentation_pages_for_developer')}
	actions={canEdit ? [
		{ label: t('docs.new_doc_page'), icon: Plus, onclick: openCreate },
		{ label: t('docs.generate_with_ai'), icon: Sparkles, onclick: openGenerate }
	] : []}
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
		<div>
			<label for="doc-body" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.body_markdown')}</label>
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
