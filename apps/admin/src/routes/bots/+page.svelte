<script lang="ts">
	import { botsAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent, CardHeader } from '$lib/components/ui/card';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { Select } from '$lib/components/ui/select';
	import { Bot, MessageSquare, Shield, Settings, Plus, RefreshCw, Pencil, Trash2, Eye } from 'lucide-svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';

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

	// View modal
	let showViewModal = $state(false);
	let viewBot = $state<Bot | null>(null);

	// Backend gates bot mutations on bots.admin
	const canManageBots = $derived(hasPermission("bots.admin"));

	const botColumns = [
		{
			key: 'name',
			label: 'Bot',
			sortable: true,
			render: (_: any, row: any) => `
				<div class="flex items-center gap-3">
					<div class="h-10 w-10 rounded-lg ${row.type === 'chatbot' ? 'bg-primary-100' : row.type === 'moderation' ? 'bg-warning/15' : 'bg-info-100'} flex items-center justify-center">
						<svg class="h-5 w-5 ${row.type === 'chatbot' ? 'text-primary-600' : row.type === 'moderation' ? 'text-warning-600' : 'text-info-600'}" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"></path></svg>
					</div>
					<div>
						<p class="font-medium text-foreground">${row.name}</p>
						<p class="text-sm text-muted">${row.description || 'No description'}</p>
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
					moderation: 'bg-warning/15 text-warning border-warning',
					utility: 'bg-info-100 text-info-800 border-info-200',
					custom: 'bg-surface text-foreground border'
				};
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${classes[value] || 'bg-surface text-foreground border'}">${value}</span>`;
			}
		},
		{
			key: 'status',
			label: 'Status',
			sortable: true,
			render: (value: string) => {
				const classes: Record<string, string> = {
					active: 'bg-success/15 text-success border-success',
					inactive: 'bg-surface text-foreground border',
					error: 'bg-danger/15 text-danger border-danger'
				};
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${classes[value] || 'bg-surface text-foreground border'}">${value}</span>`;
			}
		},
		{
			key: 'last_active',
			label: 'Last Active',
			sortable: true,
			render: (value: string) => `<span class="text-sm text-muted">${value ? new Date(value).toLocaleString() : 'Never'}</span>`
		},
		{
			key: 'created_at',
			label: 'Created',
			sortable: true,
			render: (value: string) => `<span class="text-sm text-muted">${new Date(value).toLocaleDateString()}</span>`
		},
		{
			key: 'actions',
			label: '',
			class: 'w-24',
			component: BotActionsCell
		}
	];

	const botTypeOptions = [
		{ value: 'chatbot', label: 'Chatbot' },
		{ value: 'moderation', label: 'Moderation' },
		{ value: 'utility', label: 'Utility' },
		{ value: 'custom', label: 'Custom' }
	];

	const botStatusOptions = [
		{ value: 'active', label: 'Active' },
		{ value: 'inactive', label: 'Inactive' }
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

	function openViewBot(bot: Bot) {
		viewBot = bot;
		showViewModal = true;
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

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>Bots — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title="Bots"
	description="Manage chatbots, moderation bots, and automated agents"
	actions={[
		{ label: 'Refresh', icon: RefreshCw, variant: 'outline' as const, onclick: load },
		...(canManageBots ? [{ label: 'Add Bot', icon: Plus, variant: 'default' as const, onclick: openCreateBot }] : [])
	]}
/>

<div class="space-y-6">
	{#if error}
		<div class="rounded-lg border border-danger bg-danger-50 px-4 py-3 text-sm text-danger-700">
			{error}
		</div>
	{/if}

	{#if loading}
		<p class="py-8 text-center text-sm text-muted">{t('common.loading')}</p>
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
			<label for="bot-name" class="mb-1 block text-sm font-medium text-foreground-alt">{t('bots.name')}</label>
			<input
				id="bot-name"
				type="text"
				class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				bind:value={botForm.name}
				placeholder="e.g., Support Assistant"
			/>
		</div>
		<div>
			<label for="bot-type" class="mb-1 block text-sm font-medium text-foreground-alt">{t('bots.type')}</label>
			<Select options={botTypeOptions} bind:value={botForm.type} />
		</div>
		<div>
			<label for="bot-desc" class="mb-1 block text-sm font-medium text-foreground-alt">{t('bots.description_2')}</label>
			<input
				id="bot-desc"
				type="text"
				class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				bind:value={botForm.description}
				placeholder="What this bot does"
			/>
		</div>
		<div>
			<label for="bot-status" class="mb-1 block text-sm font-medium text-foreground-alt">{t('bots.status')}</label>
			<Select options={botStatusOptions} bind:value={botForm.status} />
		</div>
	</div>
	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => showBotModal = false}>{t('bots.cancel')}</Button>
		<Button onclick={saveBot} disabled={saving || !botForm.name}>
			{saving ? 'Saving...' : editingBot ? 'Save Changes' : 'Create Bot'}
		</Button>
	</div>
</Modal>

<!-- View Modal -->
<Modal bind:open={showViewModal} title="Bot Details" size="sm">
	{#if viewBot}
		<div class="space-y-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('bots.name')}</p>
				<p class="mt-1 text-sm font-medium text-foreground">{viewBot.name}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('bots.type')}</p>
				<p class="mt-1 text-sm font-medium text-foreground">{viewBot.type}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('bots.status')}</p>
				<p class="mt-1 text-sm font-medium text-foreground">{viewBot.status}</p>
			</div>
			{#if viewBot.description}
				<div>
					<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('bots.description_2')}</p>
					<p class="mt-1 text-sm font-medium text-foreground">{viewBot.description}</p>
				</div>
			{/if}
		</div>
	{/if}
	<div class="mt-6 flex justify-end">
		<Button variant="outline" onclick={() => (showViewModal = false)}>{t('bots.cancel')}</Button>
	</div>
</Modal>

<!-- Action Cell -->
{#snippet BotActionsCell(row: Bot)}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openViewBot(row)}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if canManageBots}
			<Button variant="ghost" size="sm" onclick={() => openEditBot(row.id)}>
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => confirmDelete(row.id)} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
