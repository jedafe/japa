<script lang="ts">
	import { api, pageBuilderAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Plus, Pencil, Trash2 } from 'lucide-svelte';
	import Modal from '$lib/components/Modal.svelte';

	type Block = {
		id: string;
		name: string;
		block_type: string;
		category: string;
		is_enabled: boolean;
		created_at: string;
	};

	let blocks = $state<Block[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	// Search term wired into the blocks load
	let searchTerm = $state('');

	let showCreateModal = $state(false);
	let showEditModal = $state(false);
	let editingBlock = $state<Block | null>(null);
	let saving = $state(false);

	let modalOpen = $state(false);

	let name = $state('');
	let blockType = $state('');
	let category = $state('');
	let isEnabled = $state(true);

	const canManageBlocks = $derived(hasPermission('pages.manage'));

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const params = new URLSearchParams();
			if (searchTerm.trim()) params.set('q', searchTerm.trim());
			const qs = params.toString();
			const result = await api<Block[]>(`/admin/pagebuilder/blocks${qs ? `?${qs}` : ''}`);
			if (result.ok) {
				blocks = result.data ?? [];
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
		blockType = '';
		category = '';
		isEnabled = true;
		modalOpen = true;
	}

	function openEdit(block: Block) {
		editingBlock = block;
		name = block.name;
		blockType = block.block_type;
		category = block.category;
		isEnabled = block.is_enabled;
		modalOpen = true;
	}

	async function createBlock() {
		if (!name.trim() || !blockType.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const result = await api('/admin/pagebuilder/blocks', {
				method: 'POST',
				body: { name: name.trim(), block_type: blockType.trim(), category: category.trim() || undefined, is_enabled: isEnabled }
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

	async function updateBlock() {
		if (!editingBlock || !name.trim() || !blockType.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const result = await api(`/admin/pagebuilder/blocks/${editingBlock.id}`, {
				method: 'PATCH',
				body: { name: name.trim(), block_type: blockType.trim(), category: category.trim() || undefined, is_enabled: isEnabled }
			});
			if (result.ok) {
				modalOpen = false;
				editingBlock = null;
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

	async function deleteBlock(id: string) {
		errorMessage = '';
		try {
			await api(`/admin/pagebuilder/blocks/${id}`, { method: 'DELETE' });
			blocks = blocks.filter((b) => b.id !== id);
		} catch {
			errorMessage = t('common.error');
		}
	}

	// Debounced search handler that refetches the blocks list
	let searchTimer: ReturnType<typeof setTimeout> | undefined;
	function onSearchInput(e: Event) {
		searchTerm = (e.currentTarget as HTMLInputElement).value;
		if (searchTimer) clearTimeout(searchTimer);
		searchTimer = setTimeout(() => load(), 300);
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('pageBuilder.blocks_title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('pageBuilder.blocks_title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('pageBuilder.blocks_description')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex items-center justify-between gap-2">
	<!-- Debounced search input wired to the blocks load -->
	<input
		type="search"
		placeholder={t('common.search')}
		oninput={onSearchInput}
		aria-label={t('common.search')}
		class="w-full max-w-xs rounded-lg border p-2 text-sm outline-none focus:ring-2"
		style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground); --tw-ring-color: var(--color-primary)"
	/>
	{#if canManageBlocks}
		<button class="btn btn-primary flex items-center gap-1.5 text-xs" onclick={openCreate}>
			<Plus class="h-3 w-3" />
			{t('pageBuilder.create_block')}
		</button>
	{/if}
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('pageBuilder.name')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('pageBuilder.type')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('pageBuilder.category')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('pageBuilder.status')}</th>
					<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('common.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each blocks as block (block.id)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-medium">{block.name}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{block.block_type}</td>
						<td class="px-4 py-3 text-xs">{block.category || '—'}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {block.is_enabled ? 'bg-success/15 text-success dark:text-success' : 'bg-muted/15 text-muted dark:text-muted'}">
								{block.is_enabled ? t('pageBuilder.enabled') : t('pageBuilder.disabled')}
							</span>
						</td>
						<td class="px-4 py-3 text-right">
							{#if canManageBlocks}
								<div class="flex items-center justify-end gap-1">
									<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10" onclick={() => openEdit(block)}>
										<Pencil class="h-3 w-3" style="color: var(--color-primary)" />
									</button>
									<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10" onclick={() => deleteBlock(block.id)}>
										<Trash2 class="h-3 w-3" style="color: var(--color-danger)" />
									</button>
								</div>
							{/if}
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('pageBuilder.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<!-- Create/Edit Modal -->
<Modal bind:open={modalOpen} title={editingBlock ? t('pageBuilder.edit_block') : t('pageBuilder.create_block')} onConfirm={editingBlock ? updateBlock : createBlock} confirmLabel={saving ? t('common.saving') : editingBlock ? t('common.save') : t('common.create')}>
	<div class="space-y-4">
		<div>
			<label for="pb-name" class="mb-1 block text-sm font-medium">{t('pageBuilder.name')}</label>
			<input id="pb-name" type="text" bind:value={name} required class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
		</div>
		<div>
			<label for="pb-type" class="mb-1 block text-sm font-medium">{t('pageBuilder.type')}</label>
			<input id="pb-type" type="text" bind:value={blockType} required class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
		</div>
		<div>
			<label for="pb-cat" class="mb-1 block text-sm font-medium">{t('pageBuilder.category')}</label>
			<input id="pb-cat" type="text" bind:value={category} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
		</div>
		<div class="flex items-center gap-2">
			<input id="pb-enabled" type="checkbox" bind:checked={isEnabled} class="rounded" />
			<label for="pb-enabled" class="text-sm">{t('pageBuilder.enabled')}</label>
		</div>
	</div>
</Modal>
