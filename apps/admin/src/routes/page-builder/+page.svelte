<script lang="ts">
	import { api, pageBuilderApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Plus, Pencil, Trash2, Eye } from 'lucide-svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';

	type Block = {
		id: string;
		name: string;
		slug: string;
		category: string;
		component: string;
		default_config: Record<string, unknown> | null;
		config_schema: Record<string, unknown> | null;
		preview_image: string | null;
		is_enabled: boolean;
		is_system: boolean;
		created_at: string;
		updated_at: string;
	};

	type BlockTypeOption = {
		type: string;
		name: string;
		description: string;
		category: string;
	};

	// slugify derives a URL-safe slug from a free-form name so the backend
	// (CreateBlockInput.Slug — required) always receives a valid value even
	// when the admin leaves the Slug field blank.
	function slugify(value: string): string {
		return value.toLowerCase().trim()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-+|-+$/g, '');
	}

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
	let slug = $state('');
	let component = $state('');
	let category = $state('');
	let isEnabled = $state(true);

	// Delete confirmation state
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<Block | null>(null);
	let deleting = $state(false);

	// Preview modal state
	let showPreviewModal = $state(false);
	let previewBlock = $state<Block | null>(null);

	const canManageBlocks = $derived(hasPermission('pagebuilder.blocks.manage'));

	// Block type options fetched from the API registry (Rule 16 — no hardcoded data).
	// The /page-builder/block-types endpoint returns the static block template
	// registry defined in internal/content/block_templates.go.
	let blockTypeOptions = $state<BlockTypeOption[]>([]);

	// Derive categories dynamically from the fetched block types.
	const blockCategories = $derived(
		[...new Set(blockTypeOptions.map((bt) => bt.category))].filter(Boolean).sort()
	);

	const columns = [
		{ key: 'name', label: t('pageBuilder.name'), sortable: true },
		{ key: 'component', label: t('pageBuilder.type'), sortable: true, class: 'text-xs', render: (v: string) => `<span style="color: var(--color-muted)">${v}</span>` },
		{ key: 'category', label: t('pageBuilder.category'), sortable: true, class: 'text-xs', render: (v: string) => v || '—' },
		{ key: 'is_enabled', label: t('pageBuilder.status'), sortable: true, render: (v: boolean) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${v ? 'bg-success/15 text-success dark:text-success' : 'bg-muted/15 text-muted dark:text-muted'}">${v ? t('pageBuilder.enabled') : t('pageBuilder.disabled')}</span>` },
		{ key: 'actions', label: t('common.actions'), class: 'w-32 text-right', componentSnippet: BlockActionsCell }
	];

	function filteredBlocks(): Block[] {
		const q = searchQuery.toLowerCase().trim();
		if (!q) return blocks;
		return blocks.filter(b =>
			b.name.toLowerCase().includes(q) ||
			b.component.toLowerCase().includes(q) ||
			(b.category ?? '').toLowerCase().includes(q)
		);
	}

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			// Fetch both the CRUD blocks and the static block type registry in parallel.
			const [blocksResult, typesResult] = await Promise.all([
				api<Block[]>('/admin/pagebuilder/blocks'),
				pageBuilderApi.getBlockTypes(),
			]);
			if (blocksResult.ok) {
				blocks = blocksResult.data ?? [];
			} else {
				errorMessage = blocksResult.error;
			}
			if (typesResult.ok) {
				blockTypeOptions = typesResult.data?.block_types ?? [];
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
		component = '';
		category = '';
		isEnabled = true;
		modalOpen = true;
	}

	function openEdit(block: Block) {
		editingBlock = block;
		name = block.name;
		slug = block.slug;
		component = block.component;
		category = block.category;
		isEnabled = block.is_enabled;
		modalOpen = true;
	}

	async function createBlock() {
		if (!name.trim() || !component.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const result = await api('/admin/pagebuilder/blocks', {
				method: 'POST',
				body: {
					name: name.trim(),
					slug: slug.trim() || slugify(name),
					component: component.trim(),
					category: category.trim() || undefined,
					is_enabled: isEnabled
				}
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
		if (!editingBlock || !name.trim() || !component.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const result = await api(`/admin/pagebuilder/blocks/${editingBlock.id}`, {
				method: 'PATCH',
				body: {
					name: name.trim(),
					slug: slug.trim() || slugify(name),
					component: component.trim(),
					category: category.trim() || undefined,
					is_enabled: isEnabled
				}
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
		const target = blocks.find((b) => b.id === id);
		if (!target) return;
		deleteTarget = target;
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		const targetId = deleteTarget.id;
		deleting = true;
		errorMessage = '';
		try {
			const result = await api(`/admin/pagebuilder/blocks/${targetId}`, { method: 'DELETE' });
			if (result.ok) {
				blocks = blocks.filter((b) => b.id !== targetId);
				showDeleteConfirm = false;
				deleteTarget = null;
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			deleting = false;
		}
	}

	function openPreview(block: Block) {
		previewBlock = block;
		showPreviewModal = true;
	}

	$effect(() => {
		load();
	});
</script>

{#snippet BlockActionsCell({ value, row }: { value: any; row: Block })}
	<div class="flex items-center justify-end gap-1">
		<Button variant="ghost" size="sm" onclick={() => openPreview(row)}>
			<Eye class="h-3 w-3" style="color: var(--color-muted)" />
		</Button>
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
			<label for="pb-slug" class="mb-1 block text-sm font-medium">{t('pageBuilder.slug')}</label>
			<input id="pb-slug" type="text" bind:value={slug} placeholder={t('pageBuilder.slug_placeholder')} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
		</div>
		<div>
			<label for="pb-type" class="mb-1 block text-sm font-medium">{t('pageBuilder.type')}</label>
			<select id="pb-type" bind:value={component} required class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)">
				<option value="" disabled>{t('pageBuilder.select_type', 'Select a block type')}</option>
				{#each blockTypeOptions as bt}
					<option value={bt.type}>{bt.name}</option>
				{/each}
			</select>
		</div>
		<div>
			<label for="pb-cat" class="mb-1 block text-sm font-medium">{t('pageBuilder.category')}</label>
			<select id="pb-cat" bind:value={category} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)">
				<option value="">{t('pageBuilder.select_category', 'Select a category')}</option>
				{#each blockCategories as cat}
					<option value={cat}>{cat}</option>
				{/each}
			</select>
		</div>
		<div class="flex items-center gap-2">
			<input id="pb-enabled" type="checkbox" bind:checked={isEnabled} class="rounded" />
			<label for="pb-enabled" class="text-sm">{t('pageBuilder.enabled')}</label>
		</div>
	</div>
</Modal>

<!-- Delete Confirmation Modal -->
<Modal bind:open={showDeleteConfirm} title={t('pageBuilder.confirm_delete', 'Delete Block Type')}>
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('pageBuilder.delete_warning', 'Delete block type')} <strong>{deleteTarget.name}</strong>?
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-danger)">{t('common.cannotUndo', 'This action cannot be undone.')}</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteConfirm = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={executeDelete} disabled={deleting}>
			{deleting ? t('common.deleting', 'Deleting...') : t('common.delete', 'Delete')}
		</Button>
	</div>
</Modal>

<!-- Preview Modal -->
{#if showPreviewModal && previewBlock}
	<Modal bind:open={showPreviewModal} title={t('pageBuilder.preview_title', 'Block Type Preview')}>
		<div class="space-y-4">
			<!-- Preview header -->
			<div class="flex items-center gap-3">
				<div class="flex h-12 w-12 items-center justify-center rounded-lg border" style="border-color: var(--color-border); background: var(--color-surface)">
					<span class="text-lg font-bold" style="color: var(--color-primary)">{previewBlock.component.charAt(0).toUpperCase()}</span>
				</div>
				<div>
					<p class="text-sm font-semibold" style="color: var(--color-foreground)">{previewBlock.name}</p>
					<p class="text-xs" style="color: var(--color-muted)">{previewBlock.component}</p>
				</div>
				<div class="ml-auto">
					{#if previewBlock.is_enabled}
						<span class="inline-block rounded-full bg-success/15 px-2 py-0.5 text-xs font-medium text-success">{t('pageBuilder.enabled')}</span>
					{:else}
						<span class="inline-block rounded-full bg-muted/15 px-2 py-0.5 text-xs font-medium text-muted">{t('pageBuilder.disabled')}</span>
					{/if}
				</div>
			</div>

			<!-- Details grid -->
			<div class="grid grid-cols-2 gap-3">
				<div>
					<p class="mb-0.5 text-xs font-medium" style="color: var(--color-muted)">{t('pageBuilder.name')}</p>
					<p class="text-sm" style="color: var(--color-foreground)">{previewBlock.name}</p>
				</div>
				<div>
					<p class="mb-0.5 text-xs font-medium" style="color: var(--color-muted)">{t('pageBuilder.slug')}</p>
					<p class="font-mono text-xs" style="color: var(--color-foreground)">{previewBlock.slug}</p>
				</div>
				<div>
					<p class="mb-0.5 text-xs font-medium" style="color: var(--color-muted)">{t('pageBuilder.type')}</p>
					<p class="font-mono text-xs" style="color: var(--color-foreground)">{previewBlock.component}</p>
				</div>
				<div>
					<p class="mb-0.5 text-xs font-medium" style="color: var(--color-muted)">{t('pageBuilder.category')}</p>
					<p class="text-sm" style="color: var(--color-foreground)">{previewBlock.category || '—'}</p>
				</div>
				<div>
					<p class="mb-0.5 text-xs font-medium" style="color: var(--color-muted)">{t('pageBuilder.created', 'Created')}</p>
					<p class="text-xs" style="color: var(--color-foreground)">{new Date(previewBlock.created_at).toLocaleDateString()}</p>
				</div>
				<div>
					<p class="mb-0.5 text-xs font-medium" style="color: var(--color-muted)">{t('pageBuilder.updated', 'Updated')}</p>
					<p class="text-xs" style="color: var(--color-foreground)">{new Date(previewBlock.updated_at).toLocaleDateString()}</p>
				</div>
			</div>

			<!-- Default config -->
			{#if previewBlock.default_config && Object.keys(previewBlock.default_config).length > 0}
				<div>
					<p class="mb-1 text-xs font-medium" style="color: var(--color-muted)">{t('pageBuilder.default_config', 'Default Config')}</p>
					<pre class="overflow-x-auto rounded-lg border p-3 text-xs" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)">{JSON.stringify(previewBlock.default_config, null, 2)}</pre>
				</div>
			{/if}

			<!-- Config schema -->
			{#if previewBlock.config_schema && Object.keys(previewBlock.config_schema).length > 0}
				<div>
					<p class="mb-1 text-xs font-medium" style="color: var(--color-muted)">{t('pageBuilder.config_schema', 'Config Schema')}</p>
					<pre class="overflow-x-auto rounded-lg border p-3 text-xs" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)">{JSON.stringify(previewBlock.config_schema, null, 2)}</pre>
				</div>
			{/if}

			<!-- Manage actions -->
			{#if canManageBlocks}
				<div class="flex justify-end gap-2 border-t pt-3" style="border-color: var(--color-border)">
					<Button variant="ghost" size="sm" onclick={() => { if (previewBlock) { showPreviewModal = false; openEdit(previewBlock); } }}>
						<Pencil class="h-3 w-3" />
						{t('pageBuilder.edit_block')}
					</Button>
				</div>
			{/if}
		</div>
	</Modal>
{/if}
