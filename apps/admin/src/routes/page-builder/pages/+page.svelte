<script lang="ts">
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { Button } from '$lib/components/ui/button';

	// RBAC: mutations require pages.manage; publishing additionally requires pages.publish
	const canManagePages = $derived(hasPermission('pages.manage'));
	const canPublishPages = $derived(hasPermission('pages.publish'));

	// Delete confirmation
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

	// Search term wired into the pages load
	let searchTerm = $state('');
	let showCreateModal = $state(false);
	let newPage = $state({ title: '', slug: '' });
	let creating = $state(false);

	let showTemplateModal = $state(false);
	let templatePageId = $state('');
	let templateName = $state('');
	let savingTemplate = $state(false);

	let showSeoModal = $state(false);
	let seoIssues = $state<string[]>([]);
	let seoLoading = $state(false);

	// Debounced search handler that refetches the pages list
	let searchTimer: ReturnType<typeof setTimeout> | undefined;
	function onSearchInput(e: Event) {
		searchTerm = (e.currentTarget as HTMLInputElement).value;
		if (searchTimer) clearTimeout(searchTimer);
		searchTimer = setTimeout(() => load(), 300);
	}

	$effect(() => {
		load();
	});

	async function load() {
		loading = true;
		error = '';
		try {
			const params = new URLSearchParams();
			if (searchTerm.trim()) params.set('q', searchTerm.trim());
			const qs = params.toString();
			const result = await api<Page[]>(`/admin/pages${qs ? `?${qs}` : ''}`);
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
		if (!newPage.title || !newPage.slug) return;
		creating = true;
		try {
			const result = await api('/admin/pages', {
				method: 'POST',
				body: newPage
			});
			if (result.ok) {
				showCreateModal = false;
				newPage = { title: '', slug: '' };
				load();
			}
		} catch {
			error = t('common.error');
		} finally {
			creating = false;
		}
	}

	async function publishPage(id: string) {
		try {
			await api(`/admin/pages/${id}/publish`, { method: 'POST' });
			load();
		} catch {
			error = t('common.error');
		}
	}

	async function archivePage(id: string) {
		try {
			await api(`/admin/pages/${id}/archive`, { method: 'POST' });
			load();
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
		try {
			await api(`/admin/pages/${deleteTarget.id}`, { method: 'DELETE' });
			load();
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

	function statusBadge(status: string) {
		switch (status) {
			case 'published': return 'bg-success/15 text-success dark:text-success';
			case 'draft': return 'bg-muted/15 text-muted dark:text-muted';
			case 'archived': return 'bg-warning/15 text-warning dark:text-warning';
			default: return 'bg-muted/15 text-muted dark:text-muted';
		}
	}
</script>

<svelte:head>
	<title>{t('page_builder.pages_title')} — JapaConnect Admin</title>
</svelte:head>

<div class="mb-6 flex items-center justify-between gap-2">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">{t('page_builder.pages_title')}</h1>
		<p class="text-sm" style="color: var(--color-muted)">{t('page_builder.pages_description')}</p>
	</div>
	<div class="flex items-center gap-2">
		<!-- Debounced search input wired to the pages load -->
		<input
			type="search"
			placeholder={t('common.search')}
			oninput={onSearchInput}
			aria-label={t('common.search')}
			class="w-full max-w-xs rounded-lg border p-2 text-sm outline-none focus:ring-2"
			style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground); --tw-ring-color: var(--color-primary)"
		/>
		{#if canManagePages}
		<button
			class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
			style="background-color: var(--color-primary)"
			onclick={() => { showCreateModal = true; }}
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

{#if loading}
	<p class="text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium">{t('page_builder.title')}</th>
					<th class="px-4 py-3 text-left font-medium">{t('page_builder.slug')}</th>
					<th class="px-4 py-3 text-left font-medium">{t('page_builder.status')}</th>
					<th class="px-4 py-3 text-left font-medium">{t('page_builder.updated')}</th>
					<th class="px-4 py-3 text-right font-medium">{t('common.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each pages as page (page.id)}
					<tr style="border-bottom: 1px solid var(--color-border)">
						<td class="px-4 py-3 font-medium">{page.title}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">/{page.slug}</td>
						<td class="px-4 py-3">
							<span class="rounded-full px-2 py-0.5 text-xs font-medium {statusBadge(page.status)}">
								{page.status}
							</span>
						</td>
						<td class="px-4 py-3">{new Date(page.updated_at).toLocaleDateString()}</td>
						<td class="px-4 py-3 text-right">
							<div class="flex items-center justify-end gap-2">
								<button
									class="text-xs text-info hover:text-info"
									onclick={() => validateSeo(page.id)}
								>
									SEO
								</button>
							{#if canManagePages}
							<button
								class="text-xs text-info hover:text-info"
								onclick={() => openSaveAsTemplate(page.id, page.title)}
							>
								{t('page_builder.save_as_template')}
							</button>
							{/if}
							{#if page.status !== 'published' && canPublishPages}
								<button
									class="text-xs text-success hover:text-success"
									onclick={() => publishPage(page.id)}
								>
									{t('page_builder.publish')}
								</button>
							{/if}
							{#if page.status === 'published' && canManagePages}
								<button
									class="text-xs text-warning hover:text-warning"
									onclick={() => archivePage(page.id)}
								>
									{t('page_builder.archive')}
								</button>
							{/if}
							{#if canManagePages}
							<Button variant="ghost" size="sm" onclick={() => confirmDelete(page)} style="color: var(--color-danger)">
								{t('common.delete')}
							</Button>
							{/if}
							</div>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('page_builder.no_pages')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

{#if showCreateModal}
	<Modal title={t('page_builder.create_page')} onclose={() => { showCreateModal = false; }}>
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
	<Modal title={t('page_builder.save_as_template')} onclose={() => { showTemplateModal = false; }}>
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
	<Modal title="SEO Validation" onclose={() => { showSeoModal = false; }}>
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
