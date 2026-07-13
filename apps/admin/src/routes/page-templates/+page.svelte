<script lang="ts">
	import { onMount } from 'svelte';
	import { api, pageTemplatesAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Plus, Pencil, Trash2 } from 'lucide-svelte';
	import Modal from '$lib/components/Modal.svelte';

	type PageTemplate = {
		id: string;
		name: string;
		slug: string;
		description?: string;
		block_count: number;
		is_active: boolean;
		created_at: string;
	};

	let templates = $state<PageTemplate[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	let showCreateModal = $state(false);
	let showEditModal = $state(false);
	let editingTemplate = $state<PageTemplate | null>(null);
	let saving = $state(false);

	let modalOpen = $state(false);

	let name = $state('');
	let slug = $state('');
	let description = $state('');
	let isActive = $state(true);

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<PageTemplate[]>('/admin/page-templates');
			if (result.ok) {
				templates = result.data ?? [];
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
		isActive = true;
		modalOpen = true;
	}

	function openEdit(tpl: PageTemplate) {
		editingTemplate = tpl;
		name = tpl.name;
		slug = tpl.slug;
		description = tpl.description || '';
		isActive = tpl.is_active;
		modalOpen = true;
	}

	async function createTemplate() {
		if (!name.trim() || !slug.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const result = await api('/admin/page-templates', {
				method: 'POST',
				body: { name: name.trim(), slug: slug.trim(), description: description.trim() || undefined, is_active: isActive }
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

	async function updateTemplate() {
		if (!editingTemplate || !name.trim() || !slug.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const result = await api(`/admin/page-templates/${editingTemplate.id}`, {
				method: 'PATCH',
				body: { name: name.trim(), slug: slug.trim(), description: description.trim() || undefined, is_active: isActive }
			});
			if (result.ok) {
				modalOpen = false;
				editingTemplate = null;
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

	async function deleteTemplate(id: string) {
		errorMessage = '';
		try {
			await api(`/admin/page-templates/${id}`, { method: 'DELETE' });
			templates = templates.filter((t) => t.id !== id);
		} catch {
			errorMessage = t('common.error');
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('pageTemplates.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('pageTemplates.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('pageTemplates.description')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<button class="btn btn-primary flex items-center gap-1.5 text-xs" onclick={openCreate}>
		<Plus class="h-3 w-3" />
		{t('pageTemplates.create')}
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('pageTemplates.name')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('pageTemplates.slug')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('pageTemplates.blocks')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('pageTemplates.status')}</th>
					<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('common.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each templates as tpl (tpl.id)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-medium">{tpl.name}</td>
						<td class="px-4 py-3 text-xs font-mono" style="color: var(--color-muted)">{tpl.slug}</td>
						<td class="px-4 py-3 text-xs">{tpl.block_count}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {tpl.is_active ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' : 'bg-slate-500/15 text-slate-700 dark:text-slate-400'}">
								{tpl.is_active ? t('pageTemplates.active') : t('pageTemplates.inactive')}
							</span>
						</td>
						<td class="px-4 py-3 text-right">
							<div class="flex items-center justify-end gap-1">
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" onclick={() => openEdit(tpl)}>
									<Pencil class="h-3 w-3" style="color: var(--color-primary)" />
								</button>
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" onclick={() => deleteTemplate(tpl.id)}>
									<Trash2 class="h-3 w-3" style="color: var(--color-danger)" />
								</button>
							</div>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('pageTemplates.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<!-- Create/Edit Modal -->
<Modal bind:open={modalOpen} title={editingTemplate ? t('pageTemplates.edit') : t('pageTemplates.create')} onConfirm={editingTemplate ? updateTemplate : createTemplate} confirmLabel={saving ? t('common.saving') : editingTemplate ? t('common.save') : t('common.create')}>
	<div class="space-y-4">
		<div>
			<label for="pt-name" class="mb-1 block text-sm font-medium">{t('pageTemplates.name')}</label>
			<input id="pt-name" type="text" bind:value={name} required class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
		</div>
		<div>
			<label for="pt-slug" class="mb-1 block text-sm font-medium">{t('pageTemplates.slug')}</label>
			<input id="pt-slug" type="text" bind:value={slug} required class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
		</div>
		<div>
			<label for="pt-desc" class="mb-1 block text-sm font-medium">{t('pageTemplates.description')}</label>
			<textarea id="pt-desc" bind:value={description} rows="3" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"></textarea>
		</div>
		<div class="flex items-center gap-2">
			<input id="pt-active" type="checkbox" bind:checked={isActive} class="rounded" />
			<label for="pt-active" class="text-sm">{t('pageTemplates.active')}</label>
		</div>
	</div>
</Modal>
