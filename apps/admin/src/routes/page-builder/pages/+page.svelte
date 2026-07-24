<script lang="ts">
	import { api, pageTemplatesAdminApiExtended, pageBuilderAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { goto } from '$app/navigation';
	import Modal from '$lib/components/Modal.svelte';
	import { Button } from '$lib/components/ui/button';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Pencil } from 'lucide-svelte';

	const canManagePages = $derived(hasPermission('pages.manage'));
	const canPublishPages = $derived(hasPermission('pages.publish'));

	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<Page | null>(null);

	type Page = {
		id: string;
		title: string;
		slug: string;
		status: string;
		created_at: string;
		updated_at: string;
	};

	let pages = $state<Page[]>([]);
	let loading = $state(true);
	let error = $state('');

	let searchQuery = $state('');
	let showCreateModal = $state(false);
	let newPage = $state({ title: '', slug: '', template_id: '', theme_id: '' });
	let creating = $state(false);
	let templates = $state<{ id: string; name: string }[]>([]);
	let themes = $state<{ id: string; name: string; is_default: boolean }[]>([]);

	let showTemplateModal = $state(false);
	let templatePageId = $state('');
	let templateName = $state('');
	let savingTemplate = $state(false);

	let showSeoModal = $state(false);
	let seoIssues = $state<string[]>([]);
	let seoLoading = $state(false);

	const columns = [
		{ key: 'title', label: t('page_builder.title'), sortable: true },
		{ key: 'slug', label: t('page_builder.slug'), sortable: true, class: 'text-xs', render: (v: string) => `<span style="color: var(--color-muted)">/${v}</span>` },
		{ key: 'status', label: t('page_builder.status'), sortable: true, render: (v: string) => `<span class="rounded-full px-2 py-0.5 text-xs font-medium ${statusBadge(v)}">${v}</span>` },
		{ key: 'updated_at', label: t('page_builder.updated'), sortable: true, render: (v: string) => new Date(v).toLocaleDateString() },
		{ key: 'actions', label: t('common.actions'), class: 'w-64 text-right', componentSnippet: PageActionsCell }
	];

	function statusBadge(status: string) {
		switch (status) {
			case 'published': return 'bg-success/15 text-success dark:text-success';
			case 'draft': return 'bg-muted/15 text-muted dark:text-muted';
			case 'archived': return 'bg-warning/15 text-warning dark:text-warning';
			default: return 'bg-muted/15 text-muted dark:text-muted';
		}
	}

	function filteredPages(): Page[] {
		const q = searchQuery.toLowerCase().trim();
		if (!q) return pages;
		return pages.filter(p =>
			p.title.toLowerCase().includes(q) ||
			p.slug.toLowerCase().includes(q) ||
			p.status.toLowerCase().includes(q)
		);
	}

	$effect(() => {
		load();
	});

	async function load() {
		loading = true;
		error = '';
		try {
			const result = await api<Page[]>('/admin/pages');
			if (result.ok) {
				pages = result.data ?? [];
			} else {
				error = result.error;
			}
		} catch {
			error = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function createPage() {
		if (!newPage.title) return;
		creating = true;
		try {
			const body: Record<string, unknown> = { title: newPage.title, slug: newPage.slug };
			if (newPage.template_id) body.template_id = newPage.template_id;
			if (newPage.theme_id) body.theme_id = newPage.theme_id;
			const result = await api<{ id: string }>('/admin/pages', { method: 'POST', body });
			if (result.ok && result.data) {
				showCreateModal = false;
				newPage = { title: '', slug: '', template_id: '', theme_id: '' };
				goto(`/page-builder/pages/${result.data.id}`);
			}
		} catch {
			error = t('common.error');
		} finally {
			creating = false;
		}
	}

	async function loadTemplatesAndThemes() {
		const [tplRes, themeRes] = await Promise.all([
			pageTemplatesAdminApiExtended.list(),
			pageBuilderAdminApiExtended.listThemes()
		]);
		if (tplRes.ok) templates = (tplRes.data as any[])?.map((t) => ({ id: t.id, name: t.name })) ?? [];
		if (themeRes.ok) themes = (themeRes.data as any[])?.map((t) => ({ id: t.id, name: t.name, is_default: t.is_default })) ?? [];
	}

	function openCreateModal() {
		newPage = { title: '', slug: '', template_id: '', theme_id: '' };
		loadTemplatesAndThemes();
		showCreateModal = true;
	}

	async function publishPage(id: string) {
		error = '';
		try {
			const result = await api(`/admin/pages/${id}/publish`, { method: 'POST' });
			if (result.ok) {
				load();
			} else {
				error = result.error;
			}
		} catch {
			error = t('common.error');
		}
	}

	async function archivePage(id: string) {
		error = '';
		try {
			const result = await api(`/admin/pages/${id}/archive`, { method: 'POST' });
			if (result.ok) {
				load();
			} else {
				error = result.error;
			}
		} catch {
			error = t('common.error');
		}
	}

	function confirmDelete(page: Page) {
		deleteTarget = page;
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		error = '';
		try {
			const result = await api(`/admin/pages/${deleteTarget.id}`, { method: 'DELETE' });
			if (result.ok) {
				load();
			} else {
				error = result.error;
			}
		} catch {
			error = t('common.error');
		} finally {
			showDeleteConfirm = false;
			deleteTarget = null;
		}
	}

	function openSaveAsTemplate(pageId: string, pageTitle: string) {
		templatePageId = pageId;
		templateName = `${pageTitle} Template`;
		showTemplateModal = true;
	}

	async function saveAsTemplate() {
		if (!templatePageId || !templateName.trim()) return;
		savingTemplate = true;
		try {
			const result = await api(`/admin/pages/${templatePageId}/save-as-template`, {
				method: 'POST',
				body: { name: templateName.trim() }
			});
			if (result.ok) {
				showTemplateModal = false;
			} else {
				error = result.error;
			}
		} catch {
			error = t('common.error');
		} finally {
			savingTemplate = false;
		}
	}

	async function validateSeo(id: string) {
		showSeoModal = true;
		seoLoading = true;
		seoIssues = [];
		try {
			const result = await api<{ issues: string[] }>(`/admin/seo/validate/${id}`);
			if (result.ok) {
				seoIssues = result.data?.issues ?? [];
			} else {
				seoIssues = [result.error];
			}
		} catch {
			seoIssues = [t('common.error')];
		} finally {
			seoLoading = false;
		}
	}
</script>

{#snippet PageActionsCell({ value, row }: { value: any; row: Page })}
	<div class="flex items-center justify-end gap-2">
		<Button variant="ghost" size="sm" onclick={() => goto(`/page-builder/pages/${row.id}`)}>
			<Pencil class="mr-1 h-3 w-3" />{t('common.edit', 'Edit')}
		</Button>
		<Button variant="ghost" size="sm" onclick={() => validateSeo(row.id)}>
			SEO
		</Button>
		{#if canManagePages}
			<Button variant="ghost" size="sm" onclick={() => openSaveAsTemplate(row.id, row.title)}>
				{t('page_builder.save_as_template')}
			</Button>
		{/if}
		{#if row.status !== 'published' && canPublishPages}
			<Button variant="ghost" size="sm" onclick={() => publishPage(row.id)} style="color: var(--color-success)">
				{t('page_builder.publish')}
			</Button>
		{/if}
		{#if row.status === 'published' && canManagePages}
			<Button variant="ghost" size="sm" onclick={() => archivePage(row.id)} style="color: var(--color-warning)">
				{t('page_builder.archive')}
			</Button>
		{/if}
		{#if canManagePages}
			<Button variant="ghost" size="sm" onclick={() => confirmDelete(row)} style="color: var(--color-danger)">
				{t('common.delete')}
			</Button>
		{/if}
	</div>
{/snippet}

<svelte:head>
	<title>{t('page_builder.pages_title')} — JapaConnect Admin</title>
</svelte:head>

<div class="mb-6 flex items-center justify-between gap-2">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">{t('page_builder.pages_title')}</h1>
		<p class="text-sm" style="color: var(--color-muted)">{t('page_builder.pages_description')}</p>
	</div>
	<div class="flex items-center gap-2">
		{#if canManagePages}
		<button
			class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
			style="background-color: var(--color-primary)"
			onclick={() => { openCreateModal(); }}
		>
			{t('page_builder.create_page')}
		</button>
		{/if}
	</div>
</div>

{#if error}
	<div class="mb-4 rounded-md border p-3 text-sm" style="border-color: var(--color-danger); background-color: var(--color-danger-bg)">
		{error}
	</div>
{/if}

<DataTable
	columns={columns}
	data={filteredPages()}
	{loading}
	pageSize={25}
	emptyMessage={t('page_builder.no_pages')}
	searchable
	searchPlaceholder={t('common.search')}
	onsearch={(q) => { searchQuery = q; }}
/>

{#if showCreateModal}
	<Modal bind:open={showCreateModal} title={t('page_builder.create_page')} onclose={() => { showCreateModal = false; }}>
		<div class="space-y-4">
			<div>
				<label for="page-title" class="mb-1 block text-sm font-medium">{t('page_builder.title')}</label>
				<input
					id="page-title"
					type="text"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					bind:value={newPage.title}
				/>
			</div>
			<div>
				<label for="page-slug" class="mb-1 block text-sm font-medium">{t('page_builder.slug')}</label>
				<input
					id="page-slug"
					type="text"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					placeholder="my-page"
					bind:value={newPage.slug}
				/>
			</div>
		<div>
			<label for="page-template" class="mb-1 block text-sm font-medium">{t('page_builder.template', 'Template')}</label>
			<select id="page-template" bind:value={newPage.template_id} class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)">
				<option value="">{t('page_builder.blank_template', 'Blank (no template)')}</option>
				{#each templates as tpl}<option value={tpl.id}>{tpl.name}</option>{/each}
			</select>
			<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('page_builder.template_hint', 'Start from a pre-built template')}</p>
		</div>
		<div>
			<label for="page-theme" class="mb-1 block text-sm font-medium">{t('page_builder.theme', 'Theme')}</label>
			<select id="page-theme" bind:value={newPage.theme_id} class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)">
				<option value="">{t('page_builder.default_theme', 'Default')}</option>
				{#each themes as thm}<option value={thm.id}>{thm.name}{thm.is_default ? ' (default)' : ''}</option>{/each}
			</select>
		</div>
			<div class="flex justify-end gap-2">
				<button
					class="rounded-md px-4 py-2 text-sm font-medium"
					style="border: 1px solid var(--color-border)"
					onclick={() => { showCreateModal = false; }}
				>
					{t('common.cancel')}
				</button>
				<button
					class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
					style="background-color: var(--color-primary)"
					disabled={creating || !newPage.title || !newPage.slug}
					onclick={createPage}
				>
					{creating ? t('common.creating') : t('common.create')}
				</button>
			</div>
		</div>
	</Modal>
{/if}

{#if showTemplateModal}
	<Modal bind:open={showTemplateModal} title={t('page_builder.save_as_template')} onclose={() => { showTemplateModal = false; }}>
		<div class="space-y-4">
			<div>
				<label for="template-name" class="mb-1 block text-sm font-medium">{t('page_builder.template_name')}</label>
				<input
					id="template-name"
					type="text"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					bind:value={templateName}
				/>
			</div>
			<div class="flex justify-end gap-2">
				<button
					class="rounded-md px-4 py-2 text-sm font-medium"
					style="border: 1px solid var(--color-border)"
					onclick={() => { showTemplateModal = false; }}
				>
					{t('common.cancel')}
				</button>
				<button
					class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
					style="background-color: var(--color-primary)"
					disabled={savingTemplate || !templateName.trim()}
					onclick={saveAsTemplate}
				>
					{savingTemplate ? t('common.saving') : t('common.save')}
				</button>
			</div>
		</div>
	</Modal>
{/if}

{#if showSeoModal}
	<Modal bind:open={showSeoModal} title="SEO Validation" onclose={() => { showSeoModal = false; }}>
		{#if seoLoading}
			<p class="text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
		{:else if seoIssues.length === 0}
			<div class="rounded-md border border-success bg-success p-4 text-sm text-success dark:dark:border-success dark:dark:bg-success dark:dark:text-success">
				No SEO issues found.
			</div>
		{:else}
			<ul class="space-y-2">
				{#each seoIssues as issue}
					<li class="rounded-md border border-warning bg-warning px-3 py-2 text-sm text-warning dark:border-warning dark:bg-warning dark:dark:text-warning">
						{issue}
					</li>
				{/each}
			</ul>
		{/if}
	</Modal>
{/if}

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title={t('common.confirmDelete', 'Confirm Delete')}>
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('page_builder.confirm_delete', 'Delete page')} <strong>{deleteTarget.title}</strong>?
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-danger)">{t('common.cannotUndo', 'This action cannot be undone.')}</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteConfirm = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={executeDelete}>{t('common.delete', 'Delete')}</Button>
	</div>
</Modal>
