<script lang="ts">
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Pencil } from 'lucide-svelte';

	type AITool = {
		id: string;
		name: string;
		description: string;
		category: string;
		is_enabled: boolean;
		required_permission?: string;
		created_at: string;
	};

	type AIUsageSummary = {
		total_cost_usd: number;
		tokens_used: number;
		by_task_type: Array<{ task_type: string; tokens_used: number; total_cost_usd: number; calls: number }>;
		by_provider?: Array<{ provider: string; tokens_used: number; total_cost_usd: number; calls: number }>;
	};

	let tools = $state<AITool[]>([]);
	let usage = $state<AIUsageSummary | null>(null);
	let loading = $state(true);
	let error = $state('');
	let activeTab = $state<'tools' | 'usage'>('tools');
	let showEditModal = $state(false);
	let editingTool = $state<AITool | null>(null);
	let saving = $state(false);
	let searchQuery = $state('');

	const toolColumns = [
		{ key: 'name', label: t('ai.tool_name'), sortable: true },
		{ key: 'description', label: t('ai.description'), render: (v: string) => v || '-' },
		{ key: 'category', label: t('ai.category'), render: (v: string) => `<span class="font-mono text-xs">${v}</span>` },
		{ key: 'is_enabled', label: t('ai.status'), render: (v: boolean) => `<span class="rounded-full px-2 py-0.5 text-xs font-medium ${v ? 'bg-success/15 text-success dark:text-success' : 'bg-muted/15 text-muted dark:text-muted'}">${v ? t('ai.enabled') : t('ai.disabled')}</span>` },
		{ key: 'actions', label: t('common.actions'), class: 'w-24', componentSnippet: ToolActionsCell }
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
				const result = await api<AITool[]>('/admin/ai/tools');
				if (result.ok) {
					tools = result.data ?? [];
				} else {
					error = result.error;
				}
			} else {
				const result = await api<AIUsageSummary>('/admin/ai/usage');
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

	function editTool(tool: AITool) {
		editingTool = { ...tool };
		showEditModal = true;
	}

	async function saveTool() {
		if (!editingTool) return;
		saving = true;
		try {
			// Send only the fields the backend UpdateTool accepts
			// (description, is_enabled, required_permission). Name and
			// category are not editable here.
			await api(`/admin/ai/tools/${editingTool.id}`, {
				method: 'PUT',
				body: {
					description: editingTool.description,
					is_enabled: editingTool.is_enabled,
					required_permission: editingTool.required_permission ?? null
				}
			});
			showEditModal = false;
			load();
		} catch {
			error = t('common.error');
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>
	<title>{t('ai.tools_title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('ai.tools_title')}</h1>
<p class="mb-6 text-sm" style="color: var(--color-muted)">{t('ai.tools_description')}</p>

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

{#if showEditModal && editingTool}
	<Modal title={t('ai.edit_tool')} onclose={() => { showEditModal = false; }}>
		<div class="space-y-4">
			<div>
				<label for="tool-name" class="mb-1 block text-sm font-medium">{t('ai.tool_name')}</label>
				<input
					id="tool-name"
					type="text"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					bind:value={editingTool.name}
				/>
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
					placeholder="e.g. ai.generate"
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
			<div class="flex justify-end gap-2">
				<button
					class="rounded-md px-4 py-2 text-sm font-medium"
					style="border: 1px solid var(--color-border)"
					onclick={() => { showEditModal = false; }}
				>
					{t('common.cancel')}
				</button>
				<button
					class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
					style="background-color: var(--color-primary)"
					disabled={saving}
					onclick={saveTool}
				>
					{saving ? t('common.saving') : t('common.save')}
				</button>
			</div>
		</div>
	</Modal>
{/if}

{#snippet ToolActionsCell({ row }: { value: any; row: AITool })}
	<Button variant="ghost" size="sm" onclick={() => editTool(row)} aria-label={t('common.edit', 'Edit')}>
		<Pencil class="h-3.5 w-3.5" />
	</Button>
{/snippet}
