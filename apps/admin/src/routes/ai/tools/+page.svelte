<script lang="ts">
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import Modal from '$lib/components/Modal.svelte';

	type AITool = {
		id: string;
		name: string;
		description: string;
		model: string;
		enabled: boolean;
		created_at: string;
	};

	type AIUsage = {
		total_requests: number;
		total_tokens: number;
		total_cost: number;
		by_model: Array<{ model: string; requests: number; tokens: number; cost: number }>;
	};

	let tools = $state<AITool[]>([]);
	let usage = $state<AIUsage | null>(null);
	let loading = $state(true);
	let error = $state('');
	let activeTab = $state<'tools' | 'usage'>('tools');
	let showEditModal = $state(false);
	let editingTool = $state<AITool | null>(null);
	let saving = $state(false);

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
				const result = await api<AIUsage>('/admin/ai/usage');
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
			await api(`/admin/ai/tools/${editingTool.id}`, {
				method: 'PUT',
				body: editingTool
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
		onclick={() => { activeTab = 'tools'; load(); }}
	>
		{t('ai.tools')}
	</button>
	<button
		class="rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'usage' ? 'background-color: var(--color-primary); color: white' : ''}
		onclick={() => { activeTab = 'usage'; load(); }}
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
	<div class="rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium">{t('ai.tool_name')}</th>
					<th class="px-4 py-3 text-left font-medium">{t('ai.description')}</th>
					<th class="px-4 py-3 text-left font-medium">{t('ai.model')}</th>
					<th class="px-4 py-3 text-left font-medium">{t('ai.status')}</th>
					<th class="px-4 py-3 text-right font-medium">{t('common.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each tools as tool (tool.id)}
					<tr style="border-bottom: 1px solid var(--color-border)">
						<td class="px-4 py-3 font-medium">{tool.name}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{tool.description || '-'}</td>
						<td class="px-4 py-3 text-xs font-mono">{tool.model}</td>
						<td class="px-4 py-3">
							<span class="rounded-full px-2 py-0.5 text-xs font-medium {tool.enabled ? 'bg-success/15 text-success dark:text-success' : 'bg-muted/15 text-muted dark:text-muted'}">
								{tool.enabled ? t('ai.enabled') : t('ai.disabled')}
							</span>
						</td>
						<td class="px-4 py-3 text-right">
							<button
								class="text-xs text-info hover:text-info"
								onclick={() => editTool(tool)}
							>
								{t('common.edit')}
							</button>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('ai.no_tools')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{:else if activeTab === 'usage' && usage}
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-3 mb-6">
		<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
			<p class="text-sm" style="color: var(--color-muted)">{t('ai.total_requests')}</p>
			<p class="mt-1 text-2xl font-bold">{usage.total_requests.toLocaleString()}</p>
		</div>
		<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
			<p class="text-sm" style="color: var(--color-muted)">{t('ai.total_tokens')}</p>
			<p class="mt-1 text-2xl font-bold">{usage.total_tokens.toLocaleString()}</p>
		</div>
		<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
			<p class="text-sm" style="color: var(--color-muted)">{t('ai.total_cost')}</p>
			<p class="mt-1 text-2xl font-bold">${usage.total_cost.toFixed(2)}</p>
		</div>
	</div>

	<div class="rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium">{t('ai.model')}</th>
					<th class="px-4 py-3 text-right font-medium">{t('ai.requests')}</th>
					<th class="px-4 py-3 text-right font-medium">{t('ai.tokens')}</th>
					<th class="px-4 py-3 text-right font-medium">{t('ai.cost')}</th>
				</tr>
			</thead>
			<tbody>
				{#each usage.by_model as model (model.model)}
					<tr style="border-bottom: 1px solid var(--color-border)">
						<td class="px-4 py-3 font-mono">{model.model}</td>
						<td class="px-4 py-3 text-right">{model.requests.toLocaleString()}</td>
						<td class="px-4 py-3 text-right">{model.tokens.toLocaleString()}</td>
						<td class="px-4 py-3 text-right">${model.cost.toFixed(2)}</td>
					</tr>
				{:else}
					<tr>
						<td colspan="4" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('ai.no_usage_data')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
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
				<label for="tool-model" class="mb-1 block text-sm font-medium">{t('ai.model')}</label>
				<input
					id="tool-model"
					type="text"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					bind:value={editingTool.model}
				/>
			</div>
			<div class="flex items-center gap-2">
				<input
					type="checkbox"
					id="tool-enabled"
					bind:checked={editingTool.enabled}
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
