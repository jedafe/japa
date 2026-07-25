<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import FilterBar from '$lib/components/FilterBar.svelte';
	import { Select } from '$lib/components/ui/select';
	import { DataTable } from '$lib/components/ui/data-table';
	import { docsApi } from '$lib/api';
	import { Dialog } from 'bits-ui';

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
	let filterAudience = $state('');
	let searchQuery = $state('');

	let showModal = $state(false);
	let editingId = $state<string | null>(null);
	let saving = $state(false);
	let showDeleteConfirm = $state(false);
	let deleteTargetId = $state<string | null>(null);

	let form = $state({
		title: '',
		slug: '',
		audience: 'users',
		status: 'draft' as string,
		body: ''
	});

	const audiences = ['users', 'premium', 'organizations', 'moderators', 'api', 'admin', 'superadmin'];
	
	const statusOptions = $derived([
		{ value: 'draft', label: 'Draft' },
		{ value: 'published', label: 'Published' }
	]);

	const filterDefs = [
		{ key: 'audience', label: t('docs.audience'), type: 'select' as const, options: audiences }
	];

	const columns = [
		{ key: 'title', label: t('docs.title_2'), sortable: true },
		{ key: 'slug', label: t('docs.slug'), render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v}</span>` },
		{ key: 'audience', label: t('docs.audience'), render: (v: string) => `<span class="inline-block rounded-full bg-black/5 px-2 py-0.5 text-xs dark:bg-surface/10">${v}</span>` },
		{ key: 'status', label: t('docs.status'), render: (v: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs ${v === 'published' ? 'bg-success/15 text-success dark:text-success' : 'bg-warning/15 text-warning dark:text-warning'}">${v}</span>` },
		{ key: 'updated_at', label: t('docs.updated'), render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v}</span>` },
		{ key: 'actions', label: '', class: 'w-48', render: (_: any, row: DocPage) => {
			const unpublishStyle = row.status === 'published' ? 'var(--color-warning)' : 'var(--color-success)';
			const unpublishLabel = row.status === 'published' ? t('docs.unpublish') : t('docs.publish');
			return `<div class="flex items-center gap-2"><button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10" style="color: var(--color-info)" onclick="window.__editDoc('${row.id}')">Edit</button><button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10" style="color: ${unpublishStyle}" onclick="window.__toggleDoc('${row.id}')">${unpublishLabel}</button><button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10" style="color: var(--color-danger)" onclick="window.__deleteDoc('${row.id}')">Delete</button></div>`;
		}}
	];

	const filteredDocs = $derived(
		searchQuery
			? docs.filter((d) => d.title.toLowerCase().includes(searchQuery.toLowerCase()) || d.slug.toLowerCase().includes(searchQuery.toLowerCase()))
			: docs
	);

	function slugify(text: string): string {
		return text
			.toLowerCase()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-|-$/g, '');
	}

	async function loadDocs() {
		loading = true;
		errorMessage = '';
		const params = new URLSearchParams();
		if (filterAudience) params.set('audience', filterAudience);
		const result = await docsApi.list(params.toString());
		if (result.ok) {
			const raw = result.data as any[] | { data: any[] };
			const rows = Array.isArray(raw) ? raw : (raw?.data ?? []);
			docs = rows.map((d: any) => ({
				...d,
				status: d.review_status ?? d.status ?? 'draft',
				body: d.body ?? d.content ?? ''
			}));
		} else {
			errorMessage = result.error;
		}
		loading = false;
	}

	function handleFilter(values: Record<string, string>) {
		filterAudience = values.audience ?? '';
		loadDocs();
	}

	function openCreate() {
		editingId = null;
		form = { title: '', slug: '', audience: 'users', status: 'draft', body: '' };
		showModal = true;
	}

	function openEdit(item: DocPage) {
		editingId = item.id;
		form = {
			title: item.title,
			slug: item.slug,
			audience: item.audience,
			status: item.status,
			body: item.body ?? ''
		};
		showModal = true;
	}

	async function submit() {
		if (!form.title.trim()) return;
		saving = true;
		errorMessage = '';

		const slug = form.slug.trim() || slugify(form.title);
		const payload = {
			title: form.title.trim(),
			slug,
			audience: form.audience,
			review_status: form.status,
			content: form.body
		};

		const result = editingId
			? await docsApi.update(editingId, payload)
			: await docsApi.create(payload);

		if (result.ok) {
			showModal = false;
			loadDocs();
		} else {
			errorMessage = result.error;
		}
		saving = false;
	}

	async function togglePublish(item: DocPage) {
		errorMessage = '';
		const newStatus = item.status === 'published' ? 'draft' : 'published';
		const result = await docsApi.update(item.id, { title: item.title, review_status: newStatus });
		if (result.ok) {
			loadDocs();
		} else {
			errorMessage = result.error;
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

	$effect(() => {
		loadDocs();
	});

	$effect(() => {
		(window as any).__editDoc = (id: string) => {
			const doc = docs.find((d) => d.id === id);
			if (doc) openEdit(doc);
		};
		(window as any).__toggleDoc = (id: string) => {
			const doc = docs.find((d) => d.id === id);
			if (doc) togglePublish(doc);
		};
		(window as any).__deleteDoc = (id: string) => confirmDelete(id);
	});
</script>

<svelte:head>
	<title>{t('docs.documentation')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('docs.documentation')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('docs.manage_documentation_pages_for_developer')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<FilterBar filters={filterDefs} onApply={handleFilter} />

<div class="mb-4 flex justify-end">
	<button
		class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
		style="background-color: var(--color-primary)"
		onclick={openCreate}
	>
		{t('docs.new_doc_page')}
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('docs.loading')}</p>
{:else}
	<DataTable
		columns={columns}
		data={filteredDocs}
		searchable={true}
		onsearch={(q: string) => { searchQuery = q; }}
		onrowclick={(row: DocPage) => openEdit(row)}
		pageSize={25}
		emptyMessage={t('docs.no_documentation_pages_found')}
		showPagination={false}
	/>
{/if}

<!-- Create/Edit Modal -->
{#if showModal}
<Dialog.Root open={true}>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 z-50 bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out" />
		<Dialog.Content class="fixed left-1/2 top-1/2 z-50 w-full max-w-2xl -translate-x-1/2 -translate-y-1/2 rounded-lg border p-6 shadow-lg data-[state=open]:animate-in data-[state=open]:fade-in data-[state=open]:zoom-in-95 data-[state=closed]:animate-out data-[state=closed]:fade-out data-[state=closed]:zoom-out-95" style="border-color: var(--color-border); background-color: var(--color-background)" onInteractOutside={() => { showModal = false; }}>
			<Dialog.Title class="text-lg font-semibold">{editingId ? t('docs.edit_doc_page') : t('docs.new_doc_page')}</Dialog.Title>
			<div class="mt-4 space-y-3">
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
						<Select
							options={audiences.map(a => ({ value: a, label: a }))}
							bind:value={form.audience}
						/>
					</div>
					<div>
						<label for="doc-status" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.status')}</label>
						<Select
							options={statusOptions}
							bind:value={form.status}
						/>
					</div>
				</div>
				<div>
					<label for="doc-body" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.body_markdown')}</label>
					<textarea id="doc-body" class="w-full rounded-md border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background-color: var(--color-surface)" rows="12" bind:value={form.body}></textarea>
				</div>
			</div>
			<div class="mt-6 flex justify-end gap-2">
				<button
					class="rounded-md border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-surface/10"
					style="border-color: var(--color-border)"
					onclick={() => { showModal = false; }}
				>
					{t('docs.cancel')}
				</button>
				<button
					class="rounded-md px-3 py-1.5 text-sm font-medium text-white transition-colors disabled:opacity-50"
					style="background-color: var(--color-primary)"
					disabled={saving}
					onclick={submit}
				>
					{saving ? t('docs.saving') : (editingId ? t('docs.update') : t('docs.create'))}
				</button>
			</div>
			<button
				class="absolute right-4 top-4 rounded-md p-1 hover:bg-black/5 dark:hover:bg-surface/10"
				style="color: var(--color-muted)"
				aria-label={t('common.close')}
				onclick={() => { showModal = false; }}
			>
				×
			</button>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
{/if}

<!-- Delete Confirmation -->
{#if showDeleteConfirm}
<Dialog.Root open={true}>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 z-50 bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out" />
		<Dialog.Content class="fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-lg border p-6 shadow-lg data-[state=open]:animate-in data-[state=open]:fade-in data-[state=open]:zoom-in-95 data-[state=closed]:animate-out data-[state=closed]:fade-out data-[state=closed]:zoom-out-95" style="border-color: var(--color-border); background-color: var(--color-background)">
			<Dialog.Title class="text-lg font-semibold">{t('docs.confirm_delete')}</Dialog.Title>
			<div class="mt-4">
				<p class="text-sm" style="color: var(--color-muted)">{t('docs.are_you_sure_you_want_to_delete_this_doc')}</p>
			</div>
			<div class="mt-6 flex justify-end gap-2">
				<button
					class="rounded-md border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-surface/10"
					style="border-color: var(--color-border)"
					onclick={() => { showDeleteConfirm = false; }}
				>
					{t('docs.cancel')}
				</button>
				<button
					class="rounded-md px-3 py-1.5 text-sm font-medium text-white transition-colors disabled:opacity-50"
					style="background-color: var(--color-danger)"
					onclick={executeDelete}
				>
				{t('docs.confirm_delete')}
			</button>
		</div>
	</Dialog.Content>
</Dialog.Portal>
</Dialog.Root>
{/if}
