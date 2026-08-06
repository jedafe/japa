<script lang="ts">
	import {
		comparisonAdminApi,
		type ComparisonEntry,
		type ComparisonType
	} from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Plus, Pencil, Trash2, Eye } from 'lucide-svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import { DataTable } from '$lib/components/ui/data-table';
	import type { ActionItem } from '$lib/components/ui/data-table';

	let comparisons = $state<ComparisonEntry[]>([]);
	let types = $state<ComparisonType[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let searchQuery = $state('');

	let showCreateSlideOver = $state(false);
	let showEditSlideOver = $state(false);
	let editingEntry = $state<ComparisonEntry | null>(null);
	let saving = $state(false);

	let showViewSlideOver = $state(false);
	let viewTarget = $state<ComparisonEntry | null>(null);
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<ComparisonEntry | null>(null);

	const canCreateComparison = $derived(hasPermission('comparison.create'));
	const canEditComparison = $derived(hasPermission('comparison.edit_any') || hasPermission('comparison.edit_own'));
	const canDeleteComparison = $derived(hasPermission('comparison.delete_any') || hasPermission('comparison.delete_own'));

	const visibilityOptions = [
		{ value: 'public', label: t('comparisons.vis_public') },
		{ value: 'members', label: t('comparisons.vis_members') },
		{ value: 'private', label: t('comparisons.vis_private') }
	];

	const columns = [
		{ key: 'title', label: t('comparisons.col_title'), sortable: true },
		{ key: 'slug', label: t('comparisons.col_slug'), render: (v: string) => `<span class="text-xs font-mono" style="color: var(--color-muted)">${v}</span>` },
		{
			key: 'visibility',
			label: t('comparisons.col_visibility'),
			render: (v: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${v === 'public' ? 'bg-success/15 text-success dark:text-success' : v === 'members' ? 'bg-muted/15 text-muted dark:text-muted' : 'bg-danger/15 text-danger dark:text-danger'}">${visibilityOptions.find((o) => o.value === v)?.label ?? v}</span>`
		},
		{
			key: 'status',
			label: t('comparisons.col_status'),
			render: (v: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${v === 'published' ? 'bg-success/15 text-success dark:text-success' : 'bg-muted/15 text-muted dark:text-muted'}">${v}</span>`
		},
		{
			key: 'created_at',
			label: t('comparisons.col_created'),
			sortable: true,
			render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v ? new Date(v).toLocaleDateString() : '—'}</span>`
		}
	];

	const rowActions: ActionItem[] = [
		{ label: t('common.view'), icon: Eye, onclick: (row: ComparisonEntry) => openView(row) },
		{ label: t('common.edit'), icon: Pencil, onclick: (row: ComparisonEntry) => openEdit(row), show: () => canEditComparison },
		{ label: t('common.delete'), icon: Trash2, variant: 'destructive', onclick: (row: ComparisonEntry) => confirmDelete(row), show: () => canDeleteComparison }
	];

	const filteredComparisons = $derived.by(() => {
		let items = comparisons;
		if (searchQuery) {
			const q = searchQuery.toLowerCase();
			items = items.filter((c) => c.title.toLowerCase().includes(q) || c.slug.toLowerCase().includes(q));
		}
		return items;
	});

	function openView(c: ComparisonEntry) {
		viewTarget = c;
		showViewSlideOver = true;
	}

	function confirmDelete(c: ComparisonEntry) {
		deleteTarget = c;
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		errorMessage = '';
		try {
			const result = await comparisonAdminApi.deleteComparison(deleteTarget.id);
			if (result.ok) {
				showDeleteConfirm = false;
				deleteTarget = null;
				await load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		}
	}

	let title = $state('');
	let slug = $state('');
	let body = $state('');
	let visibility = $state('members');
	let typeId = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const [compResult, typesResult] = await Promise.all([
				comparisonAdminApi.listComparisons(),
				comparisonAdminApi.listTypes()
			]);
			if (compResult.ok) {
				comparisons = compResult.data ?? [];
			} else {
				errorMessage = compResult.error;
			}
			if (typesResult.ok) {
				types = typesResult.data ?? [];
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	function slugify(text: string): string {
		return text
			.toLowerCase()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-|-$/g, '');
	}

	function openCreate() {
		title = '';
		slug = '';
		body = '';
		visibility = 'members';
		typeId = '';
		showCreateSlideOver = true;
	}

	function openEdit(c: ComparisonEntry) {
		editingEntry = c;
		title = c.title;
		slug = c.slug;
		body = c.body || '';
		visibility = c.visibility || 'members';
		typeId = '';
		showEditSlideOver = true;
	}

	async function createComparison() {
		if (!title.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const result = await comparisonAdminApi.createComparison({
				title: title.trim(),
				slug: slug.trim() || slugify(title),
				body: body.trim() || undefined,
				visibility,
				type_id: typeId || undefined
			});
			if (result.ok) {
				showCreateSlideOver = false;
				load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			saving = false;
		}
	}

	async function updateComparison() {
		if (!editingEntry || !title.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const result = await comparisonAdminApi.updateComparison(editingEntry.id, {
				title: title.trim(),
				slug: slug.trim() || slugify(title),
				body: body.trim() || undefined,
				visibility
			});
			if (result.ok) {
				showEditSlideOver = false;
				editingEntry = null;
				load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			saving = false;
		}
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('comparisons.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('comparisons.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('comparisons.subtitle')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	{#if canCreateComparison}
		<button class="btn btn-primary flex items-center gap-1.5 text-xs" onclick={openCreate}>
			<Plus class="h-3 w-3" />
			{t('comparisons.create')}
		</button>
	{/if}
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<DataTable
		columns={columns}
		data={filteredComparisons}
		actions={rowActions}
		pageSize={25}
		emptyMessage={t('comparisons.empty')}
		searchable
		onsearch={(q) => { searchQuery = q; }}
	/>
{/if}

<!-- View SlideOver -->
<SlideOver bind:open={showViewSlideOver} title={t('comparisons.view')}>
	{#if viewTarget}
		<div class="space-y-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('comparisons.col_title')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.title}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('comparisons.col_slug')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{viewTarget.slug}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('comparisons.col_visibility')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{visibilityOptions.find((o) => o.value === viewTarget!.visibility)?.label ?? viewTarget!.visibility}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('comparisons.col_status')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.status}</p>
			</div>
			{#if viewTarget.body}
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('comparisons.body')}</p>
					<p class="mt-1 text-sm" style="color: var(--color-foreground)">{viewTarget.body}</p>
				</div>
			{/if}
		</div>
	{/if}
	{#snippet footer()}
		<div class="mt-6 flex justify-end">
			<button class="rounded-md px-4 py-2 text-sm font-medium transition-colors" style="border: 1px solid var(--color-border)" onclick={() => (showViewSlideOver = false)}>{t('common.close')}</button>
		</div>
	{/snippet}
</SlideOver>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title={t('common.confirm')} confirmLabel={t('common.delete')} onConfirm={executeDelete}>
	<p class="text-sm" style="color: var(--color-muted)">
		{t('comparisons.delete_confirm')} <strong>{deleteTarget?.title}</strong>?
	</p>
</Modal>

<!-- Create SlideOver -->
<SlideOver bind:open={showCreateSlideOver} title={t('comparisons.create')}>
	<div class="space-y-4">
		<div>
			<label for="cmp-title" class="mb-1 block text-sm font-medium">{t('comparisons.col_title')}</label>
			<input id="cmp-title" type="text" bind:value={title} required class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" oninput={() => { if (!slug) slug = slugify(title); }} />
		</div>
		<div>
			<label for="cmp-slug" class="mb-1 block text-sm font-medium">{t('comparisons.col_slug')}</label>
			<input id="cmp-slug" type="text" bind:value={slug} class="w-full rounded-lg border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
		</div>
		<div>
			<label for="cmp-type" class="mb-1 block text-sm font-medium">{t('comparisons.type')}</label>
			<select id="cmp-type" bind:value={typeId} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)">
				<option value="">{t('comparisons.type_none')}</option>
				{#each types as tp (tp.id)}
					<option value={tp.id}>{tp.name}</option>
				{/each}
			</select>
		</div>
		<div>
			<label for="cmp-vis" class="mb-1 block text-sm font-medium">{t('comparisons.col_visibility')}</label>
			<select id="cmp-vis" bind:value={visibility} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)">
				{#each visibilityOptions as opt (opt.value)}
					<option value={opt.value}>{opt.label}</option>
				{/each}
			</select>
		</div>
		<div>
			<label for="cmp-body" class="mb-1 block text-sm font-medium">{t('comparisons.body')}</label>
			<textarea id="cmp-body" bind:value={body} rows="3" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"></textarea>
		</div>
	</div>
	{#snippet footer()}
		<div class="mt-6 flex justify-end gap-2">
			<button class="rounded-md px-4 py-2 text-sm font-medium transition-colors" style="border: 1px solid var(--color-border)" onclick={() => (showCreateSlideOver = false)}>{t('common.cancel')}</button>
			<button class="rounded-md px-4 py-2 text-sm font-medium transition-colors" style="background: var(--color-primary); color: var(--color-primary-foreground)" onclick={createComparison} disabled={saving}>{saving ? t('common.saving') : t('common.create')}</button>
		</div>
	{/snippet}
</SlideOver>

<!-- Edit SlideOver -->
<SlideOver bind:open={showEditSlideOver} title={t('comparisons.edit')}>
	<div class="space-y-4">
		<div>
			<label for="cmp-edit-title" class="mb-1 block text-sm font-medium">{t('comparisons.col_title')}</label>
			<input id="cmp-edit-title" type="text" bind:value={title} required class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
		</div>
		<div>
			<label for="cmp-edit-slug" class="mb-1 block text-sm font-medium">{t('comparisons.col_slug')}</label>
			<input id="cmp-edit-slug" type="text" bind:value={slug} class="w-full rounded-lg border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
		</div>
		<div>
			<label for="cmp-edit-vis" class="mb-1 block text-sm font-medium">{t('comparisons.col_visibility')}</label>
			<select id="cmp-edit-vis" bind:value={visibility} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)">
				{#each visibilityOptions as opt (opt.value)}
					<option value={opt.value}>{opt.label}</option>
				{/each}
			</select>
		</div>
		<div>
			<label for="cmp-edit-body" class="mb-1 block text-sm font-medium">{t('comparisons.body')}</label>
			<textarea id="cmp-edit-body" bind:value={body} rows="3" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"></textarea>
		</div>
	</div>
	{#snippet footer()}
		<div class="mt-6 flex justify-end gap-2">
			<button class="rounded-md px-4 py-2 text-sm font-medium transition-colors" style="border: 1px solid var(--color-border)" onclick={() => (showEditSlideOver = false)}>{t('common.cancel')}</button>
			<button class="rounded-md px-4 py-2 text-sm font-medium transition-colors" style="background: var(--color-primary); color: var(--color-primary-foreground)" onclick={updateComparison} disabled={saving}>{saving ? t('common.saving') : t('common.save')}</button>
		</div>
	{/snippet}
</SlideOver>
