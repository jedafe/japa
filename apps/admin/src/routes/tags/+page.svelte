<script lang="ts">
	import { onMount } from 'svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type Tag = {
		id: string;
		name: string;
		slug: string;
		post_count: number;
		follower_count: number;
		created_at: string;
	};

	let tags = $state<Tag[]>([]);
	let page = $state(0);
	let totalPages = $state(1);
	let loading = $state(true);
	let errorMessage = $state('');

	// Create/Edit modal
	let showModal = $state(false);
	let editingTag = $state<Tag | null>(null);
	let form = $state({ name: '', slug: '' });
	let saving = $state(false);

	// Delete modal
	let showDeleteModal = $state(false);
	let deleteTarget = $state<Tag | null>(null);

	async function load() {
		loading = true;
		errorMessage = '';
		const params = new URLSearchParams({ page: String(page) });
		try {
			const result = await api<{ items: Tag[]; page: number; total_pages: number }>(`/admin/tags?${params}`);
			if (result.ok) {
				tags = result.data.items;
				totalPages = result.data.total_pages;
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	function slugify(text: string): string {
		return text.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
	}

	function openCreate() {
		editingTag = null;
		form = { name: '', slug: '' };
		showModal = true;
	}

	function openEdit(tag: Tag) {
		editingTag = tag;
		form = { name: tag.name, slug: tag.slug };
		showModal = true;
	}

	async function submit() {
		if (!form.name.trim()) return;
		saving = true;
		errorMessage = '';
		const body = {
			name: form.name.trim(),
			slug: form.slug.trim() || slugify(form.name)
		};
		const result = editingTag
			? await api(`/admin/tags/${editingTag.id}`, { method: 'PATCH', body })
			: await api('/admin/tags', { method: 'POST', body });
		if (result.ok) {
			showModal = false;
			load();
		} else {
			errorMessage = result.error;
		}
		saving = false;
	}

	function confirmDelete(tag: Tag) {
		deleteTarget = tag;
		showDeleteModal = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		errorMessage = '';
		const result = await api(`/admin/tags/${deleteTarget.id}`, { method: 'DELETE' });
		if (result.ok) {
			showDeleteModal = false;
			load();
		} else {
			errorMessage = result.error;
		}
	}

	function handlePageChange(p: number) {
		page = p;
		load();
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('tags.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('tags.title')}</h1>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}
<p class="mb-4 text-sm" style="color: var(--color-muted)">Create, edit, and manage tags used across the platform</p>

<div class="mb-4 flex justify-end">
	<button
		class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
		style="background-color: var(--color-primary)"
		onclick={openCreate}
	>
		{t('tags.create')}
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('tags.name')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('tags.slug')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('tags.posts')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('tags.followers')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('tags.created')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
				</tr>
			</thead>
			<tbody>
				{#each tags as tag}
					<tr class="border-t hover:bg-black/[0.02] dark:hover:bg-white/[0.02]" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-medium">
							<span class="inline-block rounded-full bg-indigo-500/15 px-2 py-0.5 text-xs text-indigo-700 dark:text-indigo-400">#{tag.name}</span>
						</td>
						<td class="px-4 py-3 font-mono text-xs" style="color: var(--color-muted)">{tag.slug}</td>
						<td class="px-4 py-3 text-xs font-medium">{tag.post_count.toLocaleString()}</td>
						<td class="px-4 py-3 text-xs font-medium">{tag.follower_count.toLocaleString()}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{tag.created_at}</td>
						<td class="px-4 py-3">
							<div class="flex items-center gap-1">
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-info)"
									onclick={() => openEdit(tag)}
								>
									{t('common.edit')}
								</button>
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-danger)"
									onclick={() => confirmDelete(tag)}
								>
									{t('common.delete')}
								</button>
							</div>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="6" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							No tags found
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>

	<div class="mt-3 flex items-center justify-between">
		<p class="text-xs" style="color: var(--color-muted)">Page {page + 1} of {totalPages}</p>
		<div class="flex gap-1">
			<button class="rounded p-1 hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-40" disabled={page === 0} onclick={() => handlePageChange(page - 1)}>&larr;</button>
			<button class="rounded p-1 hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-40" disabled={page >= totalPages - 1} onclick={() => handlePageChange(page + 1)}>&rarr;</button>
		</div>
	</div>
{/if}

<!-- Create/Edit Modal -->
<Modal
	bind:open={showModal}
	title={editingTag ? t('tags.edit') : t('tags.create')}
	onConfirm={submit}
	confirmLabel={saving ? t('common.saving') : editingTag ? t('common.save') : t('tags.create')}
>
	<div class="space-y-3">
		<div>
			<label for="tag-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('tags.name')}</label>
			<input
				id="tag-name"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				placeholder="e.g. javascript"
				bind:value={form.name}
				oninput={() => { if (!editingTag) form.slug = slugify(form.name); }}
			/>
		</div>
		<div>
			<label for="tag-slug" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('tags.slug')}</label>
			<input
				id="tag-slug"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm font-mono"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				placeholder="javascript"
				bind:value={form.slug}
			/>
		</div>
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteModal} title={t('common.confirm')} onConfirm={executeDelete} confirmLabel={t('common.delete')}>
	<p class="text-sm" style="color: var(--color-muted)">
		Are you sure you want to delete the tag <strong>#{deleteTarget?.name}</strong>? This action cannot be undone.
	</p>
</Modal>
