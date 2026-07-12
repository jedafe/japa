<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import Modal from '$lib/components/Modal.svelte';

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
	let showCreateModal = $state(false);
	let newPage = $state({ title: '', slug: '' });
	let creating = $state(false);

	onMount(load);

	async function load() {
		loading = true;
		error = '';
		try {
			const result = await api<{ data: Page[] }>('/admin/pages');
			if (result.ok) {
				pages = result.data.data ?? [];
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

	async function deletePage(id: string) {
		if (!confirm(t('page_builder.confirm_delete'))) return;
		try {
			await api(`/admin/pages/${id}`, { method: 'DELETE' });
			load();
		} catch {
			error = t('common.error');
		}
	}

	function statusBadge(status: string) {
		switch (status) {
			case 'published': return 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400';
			case 'draft': return 'bg-slate-500/15 text-slate-700 dark:text-slate-400';
			case 'archived': return 'bg-amber-500/15 text-amber-700 dark:text-amber-400';
			default: return 'bg-slate-500/15 text-slate-700 dark:text-slate-400';
		}
	}
</script>

<svelte:head>
	<title>{t('page_builder.pages_title')} — JapaConnect Admin</title>
</svelte:head>

<div class="mb-6 flex items-center justify-between">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">{t('page_builder.pages_title')}</h1>
		<p class="text-sm" style="color: var(--color-muted)">{t('page_builder.pages_description')}</p>
	</div>
	<button
		class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
		style="background-color: var(--color-primary)"
		onclick={() => { showCreateModal = true; }}
	>
		{t('page_builder.create_page')}
	</button>
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
								{#if page.status !== 'published'}
									<button
										class="text-xs text-emerald-500 hover:text-emerald-700"
										onclick={() => publishPage(page.id)}
									>
										{t('page_builder.publish')}
									</button>
								{/if}
								{#if page.status === 'published'}
									<button
										class="text-xs text-amber-500 hover:text-amber-700"
										onclick={() => archivePage(page.id)}
									>
										{t('page_builder.archive')}
									</button>
								{/if}
								<button
									class="text-xs text-red-500 hover:text-red-700"
									onclick={() => deletePage(page.id)}
								>
									{t('common.delete')}
								</button>
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
