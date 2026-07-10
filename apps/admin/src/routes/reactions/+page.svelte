<script lang="ts">
	import { onMount } from 'svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type ReactionType = {
		id: string;
		name: string;
		emoji: string;
		category: string;
		usage_count: number;
		is_active: boolean;
		created_at: string;
	};

	let reactions = $state<ReactionType[]>([]);
	let page = $state(0);
	let totalPages = $state(1);
	let loading = $state(true);
	let errorMessage = $state('');

	// Create/Edit modal
	let showModal = $state(false);
	let editingReaction = $state<ReactionType | null>(null);
	let form = $state({ name: '', emoji: '', category: 'general', is_active: true });
	let saving = $state(false);

	// Delete modal
	let showDeleteModal = $state(false);
	let deleteTarget = $state<ReactionType | null>(null);

	const categories = ['general', 'positive', 'negative', 'fun'];

	async function load() {
		loading = true;
		errorMessage = '';
		const params = new URLSearchParams({ page: String(page) });
		try {
			const result = await api<{ items: ReactionType[]; page: number; total_pages: number }>(`/admin/reaction-types?${params}`);
			if (result.ok) {
				reactions = result.data.items;
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

	function openCreate() {
		editingReaction = null;
		form = { name: '', emoji: '', category: 'general', is_active: true };
		showModal = true;
	}

	function openEdit(reaction: ReactionType) {
		editingReaction = reaction;
		form = {
			name: reaction.name,
			emoji: reaction.emoji,
			category: reaction.category,
			is_active: reaction.is_active
		};
		showModal = true;
	}

	async function submit() {
		if (!form.name.trim() || !form.emoji.trim()) return;
		saving = true;
		errorMessage = '';
		const body = {
			name: form.name.trim(),
			emoji: form.emoji.trim(),
			category: form.category,
			is_active: form.is_active
		};
		const result = editingReaction
			? await api(`/admin/reaction-types/${editingReaction.id}`, { method: 'PATCH', body })
			: await api('/admin/reaction-types', { method: 'POST', body });
		if (result.ok) {
			showModal = false;
			load();
		} else {
			errorMessage = result.error;
		}
		saving = false;
	}

	function confirmDelete(reaction: ReactionType) {
		deleteTarget = reaction;
		showDeleteModal = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		errorMessage = '';
		const result = await api(`/admin/reaction-types/${deleteTarget.id}`, { method: 'DELETE' });
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

	function categoryBadge(category: string) {
		switch (category) {
			case 'positive': return 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400';
			case 'negative': return 'bg-red-500/15 text-red-700 dark:text-red-400';
			case 'fun': return 'bg-amber-500/15 text-amber-700 dark:text-amber-400';
			default: return 'bg-slate-500/15 text-slate-700 dark:text-slate-400';
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('reactions.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('reactions.title')}</h1>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}
<p class="mb-4 text-sm" style="color: var(--color-muted)">Manage reaction types used across the platform</p>

<div class="mb-4 flex justify-end">
	<button
		class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
		style="background-color: var(--color-primary)"
		onclick={openCreate}
	>
		{t('reactions.create')}
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('reactions.emoji')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('reactions.name')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('reactions.category')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('reactions.usage')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('reactions.active')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
				</tr>
			</thead>
			<tbody>
				{#each reactions as reaction}
					<tr class="border-t hover:bg-black/[0.02] dark:hover:bg-white/[0.02]" style="border-color: var(--color-border)">
						<td class="px-4 py-3 text-2xl">{reaction.emoji}</td>
						<td class="px-4 py-3 font-medium">{reaction.name}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs {categoryBadge(reaction.category)}">
								{reaction.category}
							</span>
						</td>
						<td class="px-4 py-3 text-xs font-medium">{reaction.usage_count.toLocaleString()}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs {reaction.is_active ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' : 'bg-slate-500/15 text-slate-700 dark:text-slate-400'}">
								{reaction.is_active ? 'Active' : 'Inactive'}
							</span>
						</td>
						<td class="px-4 py-3">
							<div class="flex items-center gap-1">
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-info)"
									onclick={() => openEdit(reaction)}
								>
									{t('common.edit')}
								</button>
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-danger)"
									onclick={() => confirmDelete(reaction)}
								>
									{t('common.delete')}
								</button>
							</div>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="6" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							No reaction types found
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
	title={editingReaction ? t('reactions.edit') : t('reactions.create')}
	onConfirm={submit}
	confirmLabel={saving ? t('common.saving') : editingReaction ? t('common.save') : t('reactions.create')}
>
	<div class="space-y-3">
		<div>
			<label for="rx-emoji" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('reactions.emoji')}</label>
			<input
				id="rx-emoji"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				placeholder="👍"
				bind:value={form.emoji}
			/>
		</div>
		<div>
			<label for="rx-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('reactions.name')}</label>
			<input
				id="rx-name"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				placeholder="Like"
				bind:value={form.name}
			/>
		</div>
		<div>
			<label for="rx-category" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('reactions.category')}</label>
			<select
				id="rx-category"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				bind:value={form.category}
			>
				{#each categories as cat}
					<option value={cat}>{cat}</option>
				{/each}
			</select>
		</div>
		<div class="flex items-center gap-2">
			<input id="rx-active" type="checkbox" bind:checked={form.is_active} />
			<label for="rx-active" class="text-sm">Active</label>
		</div>
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteModal} title={t('common.confirm')} onConfirm={executeDelete} confirmLabel={t('common.delete')}>
	<p class="text-sm" style="color: var(--color-muted)">
		Are you sure you want to delete the reaction type <strong>{deleteTarget?.emoji} {deleteTarget?.name}</strong>?
	</p>
</Modal>
