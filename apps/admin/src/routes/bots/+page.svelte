<script lang="ts">
	import { onMount } from 'svelte';
	import { botsAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent, CardHeader } from '$lib/components/ui/card';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { Bot, MessageSquare, Shield, Settings, Plus, RefreshCw } from 'lucide-svelte';

	type Bot = {
		id: string;
		name: string;
		type: string;
		description: string;
		status: 'active' | 'inactive' | 'error';
		last_active: string;
		created_at: string;
		config?: Record<string, unknown>;
	};

	let bots = $state<Bot[]>([]);
	let loading = $state(true);
	let error = $state('');

	// Modal state
	let showBotModal = $state(false);
	let editingBot = $state<Bot | null>(null);
	let botForm = $state({ name: '', type: 'chatbot', description: '', status: 'active' as 'active' | 'inactive' | 'error' });
	let saving = $state(false);

	// Delete confirmation
	let showDeleteModal = $state(false);
	let deletingBot = $state<Bot | null>(null);

	const botColumns = [
		{
			key: 'name',
			label: 'Bot',
			sortable: true,
			render: (_: any, row: any) => `
				<div class="flex items-center gap-3">
					<div class="h-10 w-10 rounded-lg ${row.type === 'chatbot' ? 'bg-primary-100' : row.type === 'moderation' ? 'bg-warning-100' : 'bg-info-100'} flex items-center justify-center">
						<svg class="h-5 w-5 ${row.type === 'chatbot' ? 'text-primary-600' : row.type === 'moderation' ? 'text-warning-600' : 'text-info-600'}" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"></path></svg>
					</div>
					<div>
						<p class="font-medium text-gray-900">${row.name}</p>
						<p class="text-sm text-gray-500">${row.description || 'No description'}</p>
					</div>
				</div>
			`
		},
		{
			key: 'type',
			label: 'Type',
			sortable: true,
			render: (value: string) => {
				const classes: Record<string, string> = {
					chatbot: 'bg-primary-100 text-primary-800 border-primary-200',
					moderation: 'bg-warning-100 text-warning-800 border-warning-200',
					utility: 'bg-info-100 text-info-800 border-info-200',
					custom: 'bg-gray-100 text-gray-800 border-gray-200'
				};
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${classes[value] || 'bg-gray-100 text-gray-800 border-gray-200'}">${value}</span>`;
			}
		},
		{
			key: 'status',
			label: 'Status',
			sortable: true,
			render: (value: string) => {
				const classes: Record<string, string> = {
					active: 'bg-success-100 text-success-800 border-success-200',
					inactive: 'bg-gray-100 text-gray-800 border-gray-200',
					error: 'bg-danger-100 text-danger-800 border-danger-200'
				};
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${classes[value] || 'bg-gray-100 text-gray-800 border-gray-200'}">${value}</span>`;
			}
		},
		{
			key: 'last_active',
			label: 'Last Active',
			sortable: true,
			render: (value: string) => `<span class="text-sm text-gray-500">${value ? new Date(value).toLocaleString() : 'Never'}</span>`
		},
		{
			key: 'created_at',
			label: 'Created',
			sortable: true,
			render: (value: string) => `<span class="text-sm text-gray-500">${new Date(value).toLocaleDateString()}</span>`
		},
		{
			key: 'actions',
			label: '',
			class: 'w-24',
			render: (_: any, row: any) => `
				<div class="flex items-center gap-1">
					<button onclick="window.__editBot('${row.id}')" class="inline-flex items-center justify-center rounded-md px-2 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-100" title="Edit">
						<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"></path></svg>
					</button>
					<button onclick="window.__deleteBot('${row.id}')" class="inline-flex items-center justify-center rounded-md px-2 py-1.5 text-sm font-medium text-danger-700 hover:bg-danger-50" title="Delete">
						<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"></path></svg>
					</button>
				</div>
			`
		}
	];

	async function load() {
		loading = true;
		error = '';
		try {
			const result = await botsAdminApiExtended.list();
			if (result.ok) {
				const data = result.data as any;
				bots = data?.bots ?? data ?? [];
			} else {
				error = t('common.error');
			}
		} catch {
			error = t('common.error');
		} finally {
			loading = false;
		}
	}

	function openCreateBot() {
		editingBot = null;
		botForm = { name: '', type: 'chatbot', description: '', status: 'active' };
		showBotModal = true;
	}

	function openEditBot(botId: string) {
		const bot = bots.find((b) => b.id === botId);
		if (!bot) return;
		editingBot = bot;
		botForm = {
			name: bot.name,
			type: bot.type,
			description: bot.description,
			status: bot.status
		};
		showBotModal = true;
	}

	async function saveBot() {
		saving = true;
		try {
			if (editingBot) {
				const result = await botsAdminApiExtended.update(editingBot.id, botForm);
				if (result.ok) {
					const updated = result.data as Bot;
					const idx = bots.findIndex((b) => b.id === editingBot!.id);
					if (idx !== -1) bots[idx] = updated;
					bots = bots;
				}
			} else {
				const result = await botsAdminApiExtended.create(botForm);
				if (result.ok) {
					load();
				}
			}
			showBotModal = false;
		} catch {
			error = t('common.error');
		} finally {
			saving = false;
		}
	}

	function confirmDelete(botId: string) {
		const bot = bots.find((b) => b.id === botId);
		if (!bot) return;
		deletingBot = bot;
		showDeleteModal = true;
	}

	async function deleteBot() {
		if (!deletingBot) return;
		try {
			const result = await botsAdminApiExtended.delete(deletingBot.id);
			if (result.ok) {
				bots = bots.filter((b) => b.id !== deletingBot!.id);
			}
		} catch {
			error = t('common.error');
		} finally {
			showDeleteModal = false;
			deletingBot = null;
		}
	}

	// Expose functions for DataTable inline actions
	if (typeof window !== 'undefined') {
		(window as any).__editBot = openEditBot;
		(window as any).__deleteBot = confirmDelete;
	}

	onMount(load);
</script>

<svelte:head>
	<title>Bots — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title="Bots"
	description="Manage chatbots, moderation bots, and automated agents"
	actions={[
		{ label: 'Refresh', icon: RefreshCw, variant: 'outline', onclick: load },
		{ label: 'Add Bot', icon: Plus, variant: 'default', onclick: openCreateBot }
	]}
/>

<div class="space-y-6">
	{#if error}
		<div class="rounded-lg border border-danger-200 bg-danger-50 px-4 py-3 text-sm text-danger-700">
			{error}
		</div>
	{/if}

	{#if loading}
		<p class="py-8 text-center text-sm text-gray-500">{t('common.loading')}</p>
	{:else}
		<Card>
			<CardContent class="p-0">
				<DataTable
					columns={botColumns}
					data={bots}
					pageSize={20}
					emptyMessage="No bots configured"
				/>
			</CardContent>
		</Card>
	{/if}
</div>

<!-- Bot Edit Modal -->
<Modal bind:open={showBotModal} title={editingBot ? 'Edit Bot' : 'Create Bot'} size="lg">
	<div class="space-y-4">
		<div>
			<label for="bot-name" class="mb-1 block text-sm font-medium text-gray-700">Name</label>
			<input
				id="bot-name"
				type="text"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				bind:value={botForm.name}
				placeholder="e.g., Support Assistant"
			/>
		</div>
		<div>
			<label for="bot-type" class="mb-1 block text-sm font-medium text-gray-700">Type</label>
			<select
				id="bot-type"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				bind:value={botForm.type}
			>
				<option value="chatbot">Chatbot</option>
				<option value="moderation">Moderation</option>
				<option value="utility">Utility</option>
				<option value="custom">Custom</option>
			</select>
		</div>
		<div>
			<label for="bot-desc" class="mb-1 block text-sm font-medium text-gray-700">Description</label>
			<input
				id="bot-desc"
				type="text"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				bind:value={botForm.description}
				placeholder="What this bot does"
			/>
		</div>
		<div>
			<label for="bot-status" class="mb-1 block text-sm font-medium text-gray-700">Status</label>
			<select
				id="bot-status"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				bind:value={botForm.status}
			>
				<option value="active">Active</option>
				<option value="inactive">Inactive</option>
			</select>
		</div>
	</div>
	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => showBotModal = false}>Cancel</Button>
		<Button onclick={saveBot} disabled={saving || !botForm.name}>
			{saving ? 'Saving...' : editingBot ? 'Save Changes' : 'Create Bot'}
		</Button>
	</div>
</Modal>

<!-- Delete Confirmation Modal -->
<Modal bind:open={showDeleteModal} title="Delete Bot" size="sm">
	<p class="text-sm text-gray-600">
		Are you sure you want to delete <strong>{deletingBot?.name}</strong>? This action cannot be undone.
	</p>
	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => showDeleteModal = false}>Cancel</Button>
		<Button variant="destructive" onclick={deleteBot}>Delete</Button>
	</div>
</Modal>
