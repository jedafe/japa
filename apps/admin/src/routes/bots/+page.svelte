<script lang="ts">
	import { onMount } from 'svelte';
	import { botsAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Plus, Pencil, Trash2, Power } from 'lucide-svelte';
	import Modal from '$lib/components/Modal.svelte';

	type Bot = {
		id: string;
		name: string;
		description?: string;
		status: string;
		is_active: boolean;
		created_at: string;
	};

	let bots = $state<Bot[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	let showCreateModal = $state(false);
	let showEditModal = $state(false);
	let editingBot = $state<Bot | null>(null);
	let saving = $state(false);

	let modalOpen = $state(false);
	let showDeleteModal = $state(false);
	let deleteTarget = $state<Bot | null>(null);

	let name = $state('');
	let description = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await botsAdminApiExtended.list();
			if (result.ok) {
				bots = (result.data as Bot[]) ?? [];
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
		editingBot = null;
		name = '';
		description = '';
		modalOpen = true;
	}

	function openEdit(bot: Bot) {
		editingBot = bot;
		name = bot.name;
		description = bot.description || '';
		modalOpen = true;
	}

	function confirmDelete(bot: Bot) {
		deleteTarget = bot;
		showDeleteModal = true;
	}

	async function createBot() {
		if (!name.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const result = await botsAdminApiExtended.create({
				name: name.trim(),
				description: description.trim() || undefined
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

	async function updateBot() {
		if (!editingBot || !name.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const result = await botsAdminApiExtended.update(editingBot.id, {
				name: name.trim(),
				description: description.trim() || undefined
			});
			if (result.ok) {
				modalOpen = false;
				editingBot = null;
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

	async function deleteBot() {
		if (!deleteTarget) return;
		saving = true;
		errorMessage = '';
		try {
			const result = await botsAdminApiExtended.delete(deleteTarget.id);
			if (result.ok) {
				showDeleteModal = false;
				deleteTarget = null;
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

	async function toggleBot(bot: Bot) {
		errorMessage = '';
		try {
			await botsAdminApiExtended.update(bot.id, { is_active: !bot.is_active });
			load();
		} catch {
			errorMessage = t('common.error');
		}
	}

	function statusColor(status: string) {
		return status === 'active'
			? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400'
			: 'bg-slate-500/15 text-slate-700 dark:text-slate-400';
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('bots.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('bots.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('bots.description')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<button class="btn btn-primary flex items-center gap-1.5 text-xs" onclick={openCreate}>
		<Plus class="h-3 w-3" />
		{t('bots.create')}
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('bots.name')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('bots.description')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('bots.status')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('bots.created')}</th>
					<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('common.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each bots as bot (bot.id)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-medium">{bot.name}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{bot.description || '—'}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {statusColor(bot.is_active ? 'active' : 'inactive')}">
								{bot.is_active ? t('bots.active') : t('bots.inactive')}
							</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{new Date(bot.created_at).toLocaleDateString()}</td>
						<td class="px-4 py-3 text-right">
							<div class="flex items-center justify-end gap-1">
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" onclick={() => toggleBot(bot)} title={bot.is_active ? t('bots.deactivate') : t('bots.activate')}>
									<Power class="h-3 w-3" style="color: {bot.is_active ? 'var(--color-warning)' : 'var(--color-success)'}" />
								</button>
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" onclick={() => openEdit(bot)} title={t('common.edit')}>
									<Pencil class="h-3 w-3" style="color: var(--color-primary)" />
								</button>
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" onclick={() => confirmDelete(bot)} title={t('common.delete')}>
									<Trash2 class="h-3 w-3" style="color: var(--color-danger)" />
								</button>
							</div>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('bots.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<!-- Create/Edit Modal -->
<Modal bind:open={modalOpen} title={editingBot ? t('bots.edit') : t('bots.create')} onConfirm={editingBot ? updateBot : createBot} confirmLabel={saving ? t('common.saving') : editingBot ? t('common.save') : t('common.create')}>
	<div class="space-y-4">
		<div>
			<label for="bot-name" class="mb-1 block text-sm font-medium">{t('bots.name')}</label>
			<input id="bot-name" type="text" bind:value={name} required class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
		</div>
		<div>
			<label for="bot-desc" class="mb-1 block text-sm font-medium">{t('bots.description')}</label>
			<textarea id="bot-desc" bind:value={description} rows="3" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"></textarea>
		</div>
	</div>
</Modal>

<!-- Delete Confirmation Modal -->
<Modal bind:open={showDeleteModal} title={t('bots.delete_confirm_title')} onConfirm={deleteBot} confirmLabel={saving ? t('common.saving') : t('common.delete')}>
	<p class="text-sm" style="color: var(--color-muted)">{t('bots.delete_confirm_message')}</p>
</Modal>
