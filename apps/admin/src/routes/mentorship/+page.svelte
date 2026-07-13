<script lang="ts">
	import { onMount } from 'svelte';
	import { api, mentorshipAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Plus, Pencil, Trash2 } from 'lucide-svelte';
	import Modal from '$lib/components/Modal.svelte';

	type Program = {
		id: string;
		name: string;
		description?: string;
		is_active: boolean;
		mentor_count: number;
		mentee_count: number;
		created_at: string;
	};

	let programs = $state<Program[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	let showCreateModal = $state(false);
	let showEditModal = $state(false);
	let editingProgram = $state<Program | null>(null);
	let saving = $state(false);

	let modalOpen = $state(false);

	let name = $state('');
	let description = $state('');
	let isActive = $state(true);

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<Program[]>('/admin/mentorship/programs');
			if (result.ok) {
				programs = result.data ?? [];
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
		description = '';
		isActive = true;
		modalOpen = true;
	}

	function openEdit(program: Program) {
		editingProgram = program;
		name = program.name;
		description = program.description || '';
		isActive = program.is_active;
		modalOpen = true;
	}

	async function createProgram() {
		if (!name.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const result = await api('/admin/mentorship/programs', {
				method: 'POST',
				body: { name: name.trim(), description: description.trim() || undefined, is_active: isActive }
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

	async function updateProgram() {
		if (!editingProgram || !name.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const result = await api(`/admin/mentorship/programs/${editingProgram.id}`, {
				method: 'PATCH',
				body: { name: name.trim(), description: description.trim() || undefined, is_active: isActive }
			});
			if (result.ok) {
				modalOpen = false;
				editingProgram = null;
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

	async function deleteProgram(id: string) {
		errorMessage = '';
		try {
			await api(`/admin/mentorship/programs/${id}`, { method: 'DELETE' });
			programs = programs.filter((p) => p.id !== id);
		} catch {
			errorMessage = t('common.error');
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('mentorship.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('mentorship.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('mentorship.description')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<button class="btn btn-primary flex items-center gap-1.5 text-xs" onclick={openCreate}>
		<Plus class="h-3 w-3" />
		{t('mentorship.create')}
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('mentorship.name')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('mentorship.mentors')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('mentorship.mentees')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('mentorship.status')}</th>
					<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('common.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each programs as program (program.id)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-medium">{program.name}</td>
						<td class="px-4 py-3 text-xs">{program.mentor_count}</td>
						<td class="px-4 py-3 text-xs">{program.mentee_count}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {program.is_active ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' : 'bg-slate-500/15 text-slate-700 dark:text-slate-400'}">
								{program.is_active ? t('mentorship.active') : t('mentorship.inactive')}
							</span>
						</td>
						<td class="px-4 py-3 text-right">
							<div class="flex items-center justify-end gap-1">
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" onclick={() => openEdit(program)}>
									<Pencil class="h-3 w-3" style="color: var(--color-primary)" />
								</button>
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" onclick={() => deleteProgram(program.id)}>
									<Trash2 class="h-3 w-3" style="color: var(--color-danger)" />
								</button>
							</div>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('mentorship.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<!-- Create/Edit Modal -->
<Modal bind:open={modalOpen} title={editingProgram ? t('mentorship.edit') : t('mentorship.create')} onConfirm={editingProgram ? updateProgram : createProgram} confirmLabel={saving ? t('common.saving') : editingProgram ? t('common.save') : t('common.create')}>
	<div class="space-y-4">
		<div>
			<label for="mp-name" class="mb-1 block text-sm font-medium">{t('mentorship.name')}</label>
			<input id="mp-name" type="text" bind:value={name} required class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
		</div>
		<div>
			<label for="mp-desc" class="mb-1 block text-sm font-medium">{t('mentorship.description')}</label>
			<textarea id="mp-desc" bind:value={description} rows="3" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"></textarea>
		</div>
		<div class="flex items-center gap-2">
			<input id="mp-active" type="checkbox" bind:checked={isActive} class="rounded" />
			<label for="mp-active" class="text-sm">{t('mentorship.active')}</label>
		</div>
	</div>
</Modal>
