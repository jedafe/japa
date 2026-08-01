<script lang="ts">
	import { botsAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { Select } from '$lib/components/ui/select';
	import { Plus, RefreshCw, Pencil, Trash2, Eye } from 'lucide-svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';

	type Bot = {
		id: string;
		name: string;
		user_agent_pattern: string;
		bot_type: string;
		policy: string;
		is_system: boolean;
		crawl_delay: number;
	};

	const BOT_TYPES = [
		{ value: 'search_engine', label: 'Search Engine' },
		{ value: 'ai_llm', label: 'AI / LLM' },
		{ value: 'social', label: 'Social' },
		{ value: 'monitoring', label: 'Monitoring' },
		{ value: 'scraper', label: 'Scraper' },
		{ value: 'unknown', label: 'Unknown' }
	];

	const POLICIES = [
		{ value: 'allow', label: 'Allow' },
		{ value: 'block', label: 'Block' },
		{ value: 'limit', label: 'Limit' }
	];

	let bots = $state<Bot[]>([]);
	let loading = $state(true);
	let error = $state('');
	let searchQuery = $state('');

	let selectedIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

	// Modal state
	let showBotModal = $state(false);
	let editingBot = $state<Bot | null>(null);
	let botForm = $state({
		name: '',
		user_agent_pattern: '',
		bot_type: 'search_engine',
		policy: 'allow',
		crawl_delay: 0
	});
	let saving = $state(false);

	// Delete confirmation
	let showDeleteModal = $state(false);
	let deletingBot = $state<Bot | null>(null);

	// View modal
	let showViewModal = $state(false);
	let viewBot = $state<Bot | null>(null);

	// Backend gates bot mutations on bots.admin
	const canManageBots = $derived(hasPermission('bots.admin'));

	const botTypeLabels: Record<string, string> = {
		search_engine: 'Search Engine',
		ai_llm: 'AI / LLM',
		social: 'Social',
		monitoring: 'Monitoring',
		scraper: 'Scraper',
		unknown: 'Unknown'
	};

	const policyClasses: Record<string, string> = {
		allow: 'bg-success/15 text-success border-success',
		block: 'bg-danger/15 text-danger border-danger',
		limit: 'bg-warning/15 text-warning border-warning'
	};

	const botTypeClasses: Record<string, string> = {
		search_engine: 'bg-primary-100 text-primary-800 border-primary-200',
		ai_llm: 'bg-info-100 text-info-800 border-info-200',
		social: 'bg-success/15 text-success border-success',
		monitoring: 'bg-warning/15 text-warning border-warning',
		scraper: 'bg-danger/15 text-danger border-danger',
		unknown: 'bg-surface text-foreground border'
	};

	const botColumns = [
		{
			key: 'name',
			label: t('app.bots.column_bot', 'Bot'),
			sortable: true,
			render: (_: any, row: any) => `
				<div class="flex items-center gap-3">
					<div>
						<p class="font-medium text-foreground">${row.name}${row.is_system ? ' <span class="text-xs text-muted">(system)</span>' : ''}</p>
						<p class="text-sm text-muted">${row.user_agent_pattern}</p>
					</div>
				</div>
			`
		},
		{
			key: 'bot_type',
			label: t('app.bots.column_type', 'Type'),
			sortable: true,
			render: (value: string) => `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${botTypeClasses[value] || botTypeClasses.unknown}">${botTypeLabels[value] || value}</span>`
		},
		{
			key: 'policy',
			label: t('app.bots.column_policy', 'Policy'),
			sortable: true,
			render: (value: string) => `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${policyClasses[value] || 'bg-surface text-foreground border'}">${value}</span>`
		},
		{
			key: 'crawl_delay',
			label: t('app.bots.column_crawl_delay', 'Crawl Delay'),
			sortable: true,
			render: (value: number) => `<span class="text-sm text-muted">${value > 0 ? value + 's' : '—'}</span>`
		},
		{
			key: 'actions',
			label: '',
			class: 'w-24',
			componentSnippet: BotActionsCell
		}
	];

	async function load() {
		loading = true;
		error = '';
		try {
			const result = await botsAdminApiExtended.list();
			if (result.ok) {
				let items = (Array.isArray(result.data) ? result.data : []) as Bot[];
				if (searchQuery) {
					const q = searchQuery.toLowerCase();
					items = items.filter(
						(b) =>
							b.name.toLowerCase().includes(q) ||
							b.user_agent_pattern.toLowerCase().includes(q) ||
							b.bot_type.toLowerCase().includes(q)
					);
				}
				bots = items;
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
		botForm = { name: '', user_agent_pattern: '', bot_type: 'search_engine', policy: 'allow', crawl_delay: 0 };
		showBotModal = true;
	}

	function openEditBot(botId: string) {
		const bot = bots.find((b) => b.id === botId);
		if (!bot) return;
		editingBot = bot;
		botForm = {
			name: bot.name,
			user_agent_pattern: bot.user_agent_pattern,
			bot_type: bot.bot_type,
			policy: bot.policy,
			crawl_delay: bot.crawl_delay
		};
		showBotModal = true;
	}

	async function saveBot() {
		saving = true;
		try {
			if (editingBot) {
				const result = await botsAdminApiExtended.update(editingBot.id, botForm);
				if (result.ok) {
					showBotModal = false;
					await load();
				}
			} else {
				const result = await botsAdminApiExtended.create(botForm);
				if (result.ok) {
					showBotModal = false;
					await load();
				}
			}
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
				showDeleteModal = false;
				deletingBot = null;
				await load();
			}
		} catch {
			error = t('common.error');
		}
	}

	async function bulkDelete() {
		if (selectedIds.size === 0) return;
		bulkDeleting = true;
		try {
			for (const id of selectedIds) {
				await botsAdminApiExtended.delete(id);
			}
			selectedIds = new Set();
			await load();
		} catch {
			error = t('common.error');
		}
		bulkDeleting = false;
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('app.bots.page_title', 'Bots')} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('app.bots.title', 'Bot & Crawler Management')}
	description={t('app.bots.description', 'Manage known crawlers, bots, and their access policies')}
	actions={[
		{ label: t('app.bots.refresh', 'Refresh'), icon: RefreshCw, variant: 'outline' as const, onclick: load },
		...(canManageBots ? [{ label: t('app.bots.add_bot', 'Add Bot'), icon: Plus, variant: 'default' as const, onclick: openCreateBot }] : [])
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
					emptyMessage={t('app.bots.no_bots_configured', 'No bots configured')}
					searchable
					onsearch={(q) => { searchQuery = q; }}
					selectable
					bind:selectedIds
				>
					{#snippet bulkActions()}
						<Button variant="destructive" size="sm" onclick={bulkDelete} disabled={bulkDeleting || selectedIds.size === 0}>
							<Trash2 class="h-3.5 w-3.5 mr-1" />
							{bulkDeleting ? t('app.bots.deleting', 'Deleting...') : t('app.bots.delete_selected', 'Delete Selected')} ({selectedIds.size})
						</Button>
					{/snippet}
				</DataTable>
			</CardContent>
		</Card>
	{/if}
</div>

<!-- Bot Edit Modal -->
<Modal bind:open={showBotModal} title={editingBot ? t('app.bots.edit_bot', 'Edit Bot') : t('app.bots.create_bot', 'Create Bot')} size="lg">
	<div class="space-y-4">
		<div>
			<label for="bot-name" class="mb-1 block text-sm font-medium text-foreground-alt">{t('bots.name', 'Name')}</label>
			<input
				id="bot-name"
				type="text"
				class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				bind:value={botForm.name}
				placeholder={t('app.bots.name_placeholder', 'e.g., Googlebot')}
			/>
		</div>
		<div>
			<label for="bot-ua" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.bots.user_agent_pattern', 'User-Agent Pattern')}</label>
			<input
				id="bot-ua"
				type="text"
				class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				bind:value={botForm.user_agent_pattern}
				placeholder={t('app.bots.ua_placeholder', 'e.g., Googlebot')}
			/>
		</div>
		<div>
			<label for="bot-type" class="mb-1 block text-sm font-medium text-foreground-alt">{t('bots.type', 'Type')}</label>
			<Select options={BOT_TYPES} bind:value={botForm.bot_type} />
		</div>
		<div>
			<label for="bot-policy" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.bots.policy', 'Policy')}</label>
			<Select options={POLICIES} bind:value={botForm.policy} />
		</div>
		<div>
			<label for="bot-delay" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.bots.crawl_delay', 'Crawl Delay (seconds)')}</label>
			<input
				id="bot-delay"
				type="number"
				min="0"
				class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				bind:value={botForm.crawl_delay}
			/>
		</div>
	</div>
	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => (showBotModal = false)}>{t('bots.cancel', 'Cancel')}</Button>
		<Button onclick={saveBot} disabled={saving || !botForm.name || !botForm.user_agent_pattern}>
			{saving ? t('app.bots.saving', 'Saving...') : editingBot ? t('app.bots.save_changes', 'Save Changes') : t('app.bots.create_bot', 'Create Bot')}
		</Button>
	</div>
</Modal>

<!-- View Modal -->
<Modal bind:open={showViewModal} title={t('app.bots.bot_details', 'Bot Details')} size="sm">
	{#if viewBot}
		<div class="space-y-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('bots.name', 'Name')}</p>
				<p class="mt-1 text-sm font-medium text-foreground">{viewBot.name}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.bots.user_agent_pattern', 'User-Agent Pattern')}</p>
				<p class="mt-1 text-sm font-medium text-foreground">{viewBot.user_agent_pattern}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('bots.type', 'Type')}</p>
				<p class="mt-1 text-sm font-medium text-foreground">{botTypeLabels[viewBot.bot_type] || viewBot.bot_type}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.bots.policy', 'Policy')}</p>
				<p class="mt-1 text-sm font-medium text-foreground">{viewBot.policy}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.bots.crawl_delay', 'Crawl Delay')}</p>
				<p class="mt-1 text-sm font-medium text-foreground">{viewBot.crawl_delay > 0 ? viewBot.crawl_delay + 's' : '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.bots.is_system', 'System')}</p>
				<p class="mt-1 text-sm font-medium text-foreground">{viewBot.is_system ? 'Yes' : 'No'}</p>
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end">
		<Button variant="outline" onclick={() => (showViewModal = false)}>{t('bots.cancel', 'Cancel')}</Button>
	</div>
</Modal>

<!-- Delete Confirmation Modal -->
<Modal bind:open={showDeleteModal} title={t('bots.delete_bot', 'Delete Bot')} size="sm">
	<p class="text-sm text-muted">
		{t('app.bots.delete_confirm', 'Delete this bot permanently?')}
	</p>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteModal = false)}>{t('bots.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={deleteBot}>{t('common.delete', 'Delete')}</Button>
	</div>
</Modal>

<!-- Action Cell -->
{#snippet BotActionsCell({ value, row }: { value: any; row: Bot })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openViewBot(row)}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if canManageBots}
			<Button variant="ghost" size="sm" onclick={() => openEditBot(row.id)}>
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			{#if !row.is_system}
				<Button variant="ghost" size="sm" onclick={() => confirmDelete(row.id)} style="color: var(--color-danger)">
					<Trash2 class="h-3.5 w-3.5" />
				</Button>
			{/if}
		{/if}
	</div>
{/snippet}
