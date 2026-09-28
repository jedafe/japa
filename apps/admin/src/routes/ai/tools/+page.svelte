<script lang="ts">
	import { aiAdminApi, type AiTool, type AiUsageSummary } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Modal } from '$lib/components/ui/modal';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import { Button } from '$lib/components/ui/button';
	import { DataTable } from '$lib/components/ui/data-table';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { Pencil, Trash2, Loader2, AlertCircle } from 'lucide-svelte';
	import { initTabParam, bindTabParam } from '$lib/utils/url-sync.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';

	// Backend admin AI tools routes (GET/PUT/DELETE /admin/ai/tools*) are
	// gated by ai.tools.manage (server.go:4002-4011). The frontend must
	// match so users without the permission see an access-denied message
	// instead of triggering 403 API calls.
	const canManage = $derived(hasPermission('ai.tools.manage'));

	let tools = $state<AiTool[]>([]);
	let usage = $state<AiUsageSummary | null>(null);
	let loading = $state(true);
	let error = $state('');
	const AI_TOOLS_TABS = ['tools', 'usage'] as const;
	let activeTab = $state<'tools' | 'usage'>(initTabParam('tools', { valid: AI_TOOLS_TABS }));
	bindTabParam(() => activeTab, (v) => (activeTab = v), { valid: AI_TOOLS_TABS });
	// ?tab=generate|models deep-links live on /ai — forward them there.
	$effect(() => {
		const t = page.url.searchParams.get('tab');
		if (t === 'generate' || t === 'models') void goto(`/ai?tab=${t}`);
	});
	let showEditSlideOver = $state(false);
	let editingTool = $state<AiTool | null>(null);
	let saving = $state(false);
	let searchQuery = $state('');

	// Delete state
	let showDeleteConfirm = $state(false);
	let deletingTool = $state<AiTool | null>(null);
	let deleting = $state(false);

	const toolColumns = [
		{ key: 'name', label: t('ai.tool_name'), sortable: true },
		{ key: 'description', label: t('ai.description'), render: (v: string) => v || '-' },
		{ key: 'category', label: t('ai.category'), render: (v: string) => `<span class="font-mono text-xs">${v}</span>` },
		{ key: 'is_enabled', label: t('ai.status'), render: (v: boolean) => `<span class="rounded-full px-2 py-0.5 text-xs font-medium ${v ? 'bg-success/15 text-success dark:text-success' : 'bg-muted/15 text-muted dark:text-muted'}">${v ? t('ai.enabled') : t('ai.disabled')}</span>` }
	];

	const rowActions: ActionItem[] = [
		{ label: t('common.edit', 'Edit'), icon: Pencil, onclick: (row: AiTool) => editTool(row) },
		{ label: t('common.delete', 'Delete'), icon: Trash2, variant: 'destructive', onclick: (row: AiTool) => confirmDeleteTool(row) }
	];

	const usageColumns = [
		{ key: 'task_type', label: t('ai.category'), render: (v: string) => `<span class="font-mono">${v}</span>` },
		{ key: 'calls', label: t('ai.requests'), render: (v: number) => v.toLocaleString() },
		{ key: 'tokens_used', label: t('ai.tokens'), render: (v: number) => v.toLocaleString() },
		{ key: 'total_cost_usd', label: t('ai.cost'), render: (v: number) => `$${(v ?? 0).toFixed(2)}` }
	];

	const filteredTools = $derived(
		searchQuery
			? tools.filter((t) => t.name.toLowerCase().includes(searchQuery.toLowerCase()) || (t.description || '').toLowerCase().includes(searchQuery.toLowerCase()))
			: tools
	);

	$effect(() => {
		load();
	});

	async function load() {
		loading = true;
		error = '';
		try {
			if (activeTab === 'tools') {
				const result = await aiAdminApi.listTools();
				if (result.ok) {
					tools = result.data ?? [];
				} else {
					error = result.error;
				}
			} else {
				const result = await aiAdminApi.getAiUsage();
				if (result.ok) {
					usage = result.data;
				} else {
					error = result.error;
				}
			}
		} catch {
			error = t('common.error');
		} finally {
			loading = false;
		}
	}

	function editTool(tool: AiTool) {
		editingTool = { ...tool };
		showEditSlideOver = true;
	}

	async function saveTool() {
		if (!editingTool) return;
		saving = true;
		try {
			// Send only the fields the backend UpdateTool accepts
			// (description, is_enabled, required_permission). Name and
			// category are not editable here.
			const result = await aiAdminApi.updateTool(editingTool.id, {
				description: editingTool.description,
				is_enabled: editingTool.is_enabled,
				required_permission: editingTool.required_permission ?? null
			});
			if (result.ok) {
				showEditSlideOver = false;
				load();
			} else {
				error = result.error;
			}
		} catch {
			error = t('common.error');
		} finally {
			saving = false;
		}
	}

	function confirmDeleteTool(tool: AiTool) {
		deletingTool = tool;
		showDeleteConfirm = true;
	}

	async function deleteTool() {
		if (!deletingTool) return;
		deleting = true;
		error = '';
		try {
			const result = await aiAdminApi.deleteTool(deletingTool.id);
			if (result.ok) {
				showDeleteConfirm = false;
				deletingTool = null;
				load();
			} else {
				error = result.error;
			}
		} catch {
			error = t('common.error');
		} finally {
			deleting = false;
		}
	}
</script>

<svelte:head>
	<title>{t('ai.tools_title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('ai.tools_title')}</h1>
<p class="mb-6 text-sm" style="color: var(--color-muted)">{t('ai.tools_description')}</p>

{#if !canManage}
	<div class="flex items-center gap-2 rounded-md border p-3 text-sm" style="border-color: var(--color-danger); color: var(--color-danger)">
		<AlertCircle class="size-4" />
		{t('ai.tools_no_permission', 'You do not have permission to manage AI tools.')}
	</div>
{:else}
<!-- Tabs -->
<div class="mb-6 flex gap-1 rounded-lg border p-1" style="border-color: var(--color-border)">
	<button
		class="rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'tools' ? 'background-color: var(--color-primary); color: white' : ''}
		onclick={() => { activeTab = 'tools'; }}
	>
		{t('ai.tools')}
	</button>
	<button
		class="rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'usage' ? 'background-color: var(--color-primary); color: white' : ''}
		onclick={() => { activeTab = 'usage'; }}
	>
		{t('ai.usage')}
	</button>
</div>

{#if error}
	<div class="mb-4 rounded-md border p-3 text-sm" style="border-color: var(--color-danger); background-color: var(--color-danger-bg)">
		{error}
	</div>
{/if}

{#if loading}
	<p class="text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else if activeTab === 'tools'}
	<DataTable
		columns={toolColumns}
		data={filteredTools}
		searchable={true}
		onsearch={(q: string) => { searchQuery = q; }}
		pageSize={25}
		actions={rowActions}
		emptyMessage={t('ai.no_tools')}
	/>
{:else if activeTab === 'usage' && usage}
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-3 mb-6">
		<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
			<p class="text-sm" style="color: var(--color-muted)">{t('ai.total_tokens')}</p>
			<p class="mt-1 text-2xl font-bold">{(usage.tokens_used ?? 0).toLocaleString()}</p>
		</div>
		<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
			<p class="text-sm" style="color: var(--color-muted)">{t('ai.total_cost')}</p>
			<p class="mt-1 text-2xl font-bold">${(usage.total_cost_usd ?? 0).toFixed(2)}</p>
		</div>
		<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
			<p class="text-sm" style="color: var(--color-muted)">{t('ai.requests')}</p>
			<p class="mt-1 text-2xl font-bold">{(usage.by_task_type ?? []).reduce((s, r) => s + (r.calls ?? 0), 0).toLocaleString()}</p>
		</div>
	</div>

	<DataTable
		columns={usageColumns}
		data={usage.by_task_type ?? []}
		pageSize={25}
		emptyMessage={t('ai.no_usage_data')}
		showPagination={false}
	/>
{:else if activeTab === 'usage' && !usage}
	<p class="text-sm" style="color: var(--color-muted)">{t('ai.no_usage_data')}</p>
{/if}

{#if showEditSlideOver && editingTool}
	<SlideOver
		bind:open={showEditSlideOver}
		title={t('ai.edit_tool')}
	>
		<div class="space-y-4">
			<div>
				<label for="tool-name" class="mb-1 block text-sm font-medium">{t('ai.tool_name')}</label>
				<input
					id="tool-name"
					type="text"
					class="w-full rounded-md border px-3 py-2 text-sm opacity-60"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					value={editingTool.name}
					disabled
					aria-label={t('ai.tool_name')}
				/>
				<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('ai.tools.name_readonly', 'Tool name cannot be changed after creation.')}</p>
			</div>
			<div>
				<label for="tool-desc" class="mb-1 block text-sm font-medium">{t('ai.description')}</label>
				<textarea
					id="tool-desc"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					rows="2"
					bind:value={editingTool.description}
				></textarea>
			</div>
			<div>
				<label for="tool-perm" class="mb-1 block text-sm font-medium">{t('ai.required_permission') ?? 'Required permission'}</label>
				<input
					id="tool-perm"
					type="text"
					placeholder={t('ai.tools.ph_eg_aigenerate', 'e.g. ai.generate')}
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					bind:value={editingTool.required_permission}
				/>
			</div>
			<div class="flex items-center gap-2">
				<input
					type="checkbox"
					id="tool-enabled"
					bind:checked={editingTool.is_enabled}
					class="rounded"
				/>
				<label for="tool-enabled" class="text-sm">{t('ai.enabled')}</label>
			</div>
		</div>
		{#snippet footer()}
			<div class="flex justify-end gap-2">
				<Button variant="outline" onclick={() => { showEditSlideOver = false; }}>
					{t('common.cancel')}
				</Button>
				<Button onclick={saveTool} disabled={saving}>
					{saving ? t('common.saving') : t('common.save')}
				</Button>
			</div>
		{/snippet}
	</SlideOver>
{/if}

{#if showDeleteConfirm && deletingTool}
	<Modal
		open={showDeleteConfirm}
		onclose={() => { showDeleteConfirm = false; deletingTool = null; }}
		title={t('ai.tools.delete_title', 'Delete Tool')}
	>
		<p class="text-sm" style="color: var(--color-foreground-alt)">
			{t('ai.tools.delete_confirm', 'Are you sure you want to delete')} <strong>{deletingTool.name}</strong>?
		</p>
		<p class="mt-2 text-xs" style="color: var(--color-muted)">
			{t('ai.tools.delete_warning', 'This action cannot be undone.')}
		</p>
		{#if error}
			<div class="mt-3 rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-danger); color: var(--color-danger)">
				{error}
			</div>
		{/if}
		<div class="mt-6 flex justify-end gap-2">
			<Button variant="outline" onclick={() => { showDeleteConfirm = false; deletingTool = null; }}>
				{t('common.cancel')}
			</Button>
			<Button variant="destructive" onclick={deleteTool} disabled={deleting}>
				{#if deleting}
					<Loader2 class="h-3.5 w-3.5 animate-spin" />
				{:else}
					<Trash2 class="h-3.5 w-3.5" />
				{/if}
				{t('common.delete', 'Delete')}
			</Button>
		</div>
	</Modal>
{/if}
{/if}

