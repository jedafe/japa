<script lang="ts">
	import { api, pageBuilderAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Plus, Pencil, Trash2 } from 'lucide-svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';

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

	let searchQuery = $state('');

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

	const columns = [
		{ key: 'name', label: t('pageBuilder.name'), sortable: true },
		{ key: 'block_type', label: t('pageBuilder.type'), sortable: true, class: 'text-xs', render: (v: string) => `<span style="color: var(--color-muted)">${v}</span>` },
		{ key: 'category', label: t('pageBuilder.category'), sortable: true, class: 'text-xs', render: (v: string) => v || '—' },
		{ key: 'is_enabled', label: t('pageBuilder.status'), sortable: true, render: (v: boolean) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${v ? 'bg-success/15 text-success dark:text-success' : 'bg-muted/15 text-muted dark:text-muted'}">${v ? t('pageBuilder.enabled') : t('pageBuilder.disabled')}</span>` },
		{ key: 'actions', label: t('common.actions'), class: 'w-24 text-right', component: BlockActionsCell }
	];

	function filteredBlocks(): Block[] {
		const q = searchQuery.toLowerCase().trim();
		if (!q) return blocks;
		return blocks.filter(b =>
			b.name.toLowerCase().includes(q) ||
			b.block_type.toLowerCase().includes(q) ||
			(b.category ?? '').toLowerCase().includes(q)
		);
	}

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<Block[]>('/admin/pagebuilder/blocks');
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

	$effect(() => {
		load();
	});
</script>

{#snippet BlockActionsCell({ value, row }: { value: any; row: Block })}
	<div class="flex items-center justify-end gap-1">
		{#if canManageBlocks}
			<Button variant="ghost" size="sm" onclick={() => openEdit(row)}>
				<Pencil class="h-3 w-3" style="color: var(--color-primary)" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => deleteBlock(row.id)}>
				<Trash2 class="h-3 w-3" style="color: var(--color-danger)" />
			</Button>
		{/if}
	</div>
{/snippet}

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

<div class="mb-4 flex items-center justify-end">
	{#if canManageBlocks}
		<button class="btn btn-primary flex items-center gap-1.5 text-xs" onclick={openCreate}>
			<Plus class="h-3 w-3" />
			{t('pageBuilder.create_block')}
		</button>
	{/if}
</div>

<DataTable
	columns={columns}
	data={filteredBlocks()}
	{loading}
	pageSize={25}
	emptyMessage={t('pageBuilder.empty')}
	searchable
	searchPlaceholder={t('common.search')}
	onsearch={(q) => { searchQuery = q; }}
/>

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
