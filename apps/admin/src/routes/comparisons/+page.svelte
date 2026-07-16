<script lang="ts">
	import { api, comparisonsAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Plus, Pencil, Trash2 } from 'lucide-svelte';
	import Modal from '$lib/components/Modal.svelte';

	type ComparisonType = {
		id: string;
		name: string;
		slug: string;
		description?: string;
		is_public: boolean;
		created_at: string;
	};

	let types = $state<ComparisonType[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	let showCreateModal = $state(false);
	let showEditModal = $state(false);
	let editingType = $state<ComparisonType | null>(null);
	let saving = $state(false);

	let modalOpen = $state(false);

	let name = $state('');
	let slug = $state('');
	let description = $state('');
	let isPublic = $state(true);

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<ComparisonType[]>('/admin/comparison-types');
			if (result.ok) {
				types = result.data ?? [];
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
		name = '';
		slug = '';
		description = '';
		isPublic = true;
		modalOpen = true;
	}

	function openEdit(ct: ComparisonType) {
		editingType = ct;
		name = ct.name;
		slug = ct.slug;
		description = ct.description || '';
		isPublic = ct.is_public;
		modalOpen = true;
	}

	async function createType() {
		if (!name.trim() || !slug.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const result = await api('/admin/comparison-types', {
				method: 'POST',
				body: { name: name.trim(), slug: slug.trim(), description: description.trim() || undefined, is_public: isPublic }
			});
			if (result.ok) {
				modalOpen = false;
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

	async function updateType() {
		if (!editingType || !name.trim() || !slug.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const result = await api(`/admin/comparison-types/${editingType.id}`, {
				method: 'PATCH',
				body: { name: name.trim(), slug: slug.trim(), description: description.trim() || undefined, is_public: isPublic }
			});
			if (result.ok) {
				modalOpen = false;
				editingType = null;
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

	async function deleteType(id: string) {
		errorMessage = '';
		try {
			const result = await api(`/admin/comparison-types/${id}`, { method: 'DELETE' });
			if (result.ok) {
				types = types.filter((t) => t.id !== id);
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
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
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('comparisons.description')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<button class="btn btn-primary flex items-center gap-1.5 text-xs" onclick={openCreate}>
		<Plus class="h-3 w-3" />
		{t('comparisons.create_type')}
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('comparisons.name')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('comparisons.slug')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('comparisons.visibility')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('common.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each types as ct (ct.id)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-medium">{ct.name}</td>
						<td class="px-4 py-3 text-xs font-mono" style="color: var(--color-muted)">{ct.slug}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {ct.is_public ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' : 'bg-slate-500/15 text-slate-700 dark:text-slate-400'}">
								{ct.is_public ? t('comparisons.public') : t('comparisons.private')}
							</span>
						</td>
						<td class="px-4 py-3 text-right">
							<div class="flex items-center justify-end gap-1">
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" onclick={() => openEdit(ct)}>
									<Pencil class="h-3 w-3" style="color: var(--color-primary)" />
								</button>
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" onclick={() => deleteType(ct.id)}>
									<Trash2 class="h-3 w-3" style="color: var(--color-danger)" />
								</button>
							</div>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="4" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('comparisons.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<!-- Create/Edit Modal -->
<Modal bind:open={modalOpen} title={editingType ? t('comparisons.edit_type') : t('comparisons.create_type')} onConfirm={editingType ? updateType : createType} confirmLabel={saving ? t('common.saving') : editingType ? t('common.save') : t('common.create')}>
	<div class="space-y-4">
		<div>
			<label for="ct-name" class="mb-1 block text-sm font-medium">{t('comparisons.name')}</label>
			<input id="ct-name" type="text" bind:value={name} required class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
		</div>
		<div>
			<label for="ct-slug" class="mb-1 block text-sm font-medium">{t('comparisons.slug')}</label>
			<input id="ct-slug" type="text" bind:value={slug} required class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
		</div>
		<div>
			<label for="ct-desc" class="mb-1 block text-sm font-medium">{t('comparisons.description')}</label>
			<textarea id="ct-desc" bind:value={description} rows="3" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"></textarea>
		</div>
		<div class="flex items-center gap-2">
			<input id="ct-public" type="checkbox" bind:checked={isPublic} class="rounded" />
			<label for="ct-public" class="text-sm">{t('comparisons.is_public')}</label>
		</div>
	</div>
</Modal>
