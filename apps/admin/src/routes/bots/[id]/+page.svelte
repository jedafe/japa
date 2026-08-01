<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { Select } from '$lib/components/ui/select';

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

	let bot = $state<Bot | null>(null);
	let loading = $state(true);
	let errorMessage = $state('');
	let successMessage = $state('');

	// Edit form
	let editName = $state('');
	let editUserAgentPattern = $state('');
	let editBotType = $state('search_engine');
	let editPolicy = $state('allow');
	let editCrawlDelay = $state(0);
	let saving = $state(false);
	let showDeleteModal = $state(false);

	const botId = $derived(page.params.id);

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<Bot>(`/admin/bots/${botId}`);
			if (result.ok) {
				bot = result.data;
				editName = bot.name;
				editUserAgentPattern = bot.user_agent_pattern;
				editBotType = bot.bot_type;
				editPolicy = bot.policy;
				editCrawlDelay = bot.crawl_delay;
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('bots.load_error', 'Failed to load bot');
		} finally {
			loading = false;
		}
	}

	async function saveBot() {
		if (!bot || !editName.trim() || !editUserAgentPattern.trim()) return;
		saving = true;
		errorMessage = '';
		successMessage = '';
		try {
			const result = await api(`/admin/bots/${bot.id}`, {
				method: 'PUT',
				body: {
					name: editName.trim(),
					user_agent_pattern: editUserAgentPattern.trim(),
					bot_type: editBotType,
					policy: editPolicy,
					crawl_delay: editCrawlDelay
				}
			});
			if (result.ok) {
				successMessage = t('bots.updated', 'Bot updated successfully');
				await load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('bots.update_error', 'Failed to update bot');
		} finally {
			saving = false;
		}
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{bot?.name ?? 'Bot'} — JapaConnect Admin</title>
</svelte:head>

<div class="mb-4">
	<a href="/bots" class="text-sm hover:underline" style="color: var(--color-muted)">&larr; {t('bots.back', 'Back to Bots')}</a>
</div>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}
{#if successMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-success); background-color: color-mix(in srgb, var(--color-success) 8%, transparent); color: var(--color-success)">
		{successMessage}
	</div>
{/if}

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading', 'Loading...')}</p>
{:else if bot}
	<h1 class="mb-1 text-2xl font-bold tracking-tight">{bot.name}</h1>
	<p class="mb-6 text-sm" style="color: var(--color-muted)">
		{t('app.bots.user_agent_pattern', 'User-Agent')}: {bot.user_agent_pattern}
		{#if bot.is_system}
			· <span class="text-xs">{t('app.bots.is_system', 'System bot')}</span>
		{/if}
	</p>

	<div class="rounded-lg border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<h2 class="mb-4 text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('bots.bot_details', 'Bot Details')}</h2>
		<div class="space-y-4">
			<div>
				<label for="edit-name" class="mb-1 block text-sm font-medium">{t('bots.name', 'Name')}</label>
				<input
					id="edit-name"
					type="text"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					bind:value={editName}
				/>
			</div>
			<div>
				<label for="edit-ua" class="mb-1 block text-sm font-medium">{t('app.bots.user_agent_pattern', 'User-Agent Pattern')}</label>
				<input
					id="edit-ua"
					type="text"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					bind:value={editUserAgentPattern}
				/>
			</div>
			<div>
				<label for="edit-type" class="mb-1 block text-sm font-medium">{t('bots.type', 'Type')}</label>
				<Select options={BOT_TYPES} bind:value={editBotType} />
			</div>
			<div>
				<label for="edit-policy" class="mb-1 block text-sm font-medium">{t('app.bots.policy', 'Policy')}</label>
				<Select options={POLICIES} bind:value={editPolicy} />
			</div>
			<div>
				<label for="edit-delay" class="mb-1 block text-sm font-medium">{t('app.bots.crawl_delay', 'Crawl Delay (seconds)')}</label>
				<input
					id="edit-delay"
					type="number"
					min="0"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					bind:value={editCrawlDelay}
				/>
			</div>
			<div class="flex items-center gap-3 pt-2">
				<button
					class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
					style="background-color: var(--color-primary)"
					disabled={saving || !editName.trim() || !editUserAgentPattern.trim()}
					onclick={saveBot}
				>
					{saving ? t('common.saving', 'Saving...') : t('bots.save_changes', 'Save Changes')}
				</button>
				{#if !bot.is_system}
					<button
						class="rounded-md px-4 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-surface/10"
						style="color: var(--color-danger)"
						onclick={() => { if (bot) showDeleteModal = true; }}
					>
						{t('bots.delete_bot', 'Delete Bot')}
					</button>
				{/if}
			</div>
		</div>
	</div>

	<Modal bind:open={showDeleteModal} title={t('bots.delete_confirm', 'Delete this bot permanently?')} size="sm">
		<p class="text-sm" style="color: var(--color-muted)">
			{t('app.bots.delete_warning', 'This action cannot be undone. The bot will be permanently removed.')}
		</p>
		<div class="mt-4 flex justify-end gap-2">
			<Button variant="ghost" onclick={() => (showDeleteModal = false)}>{t('common.cancel', 'Cancel')}</Button>
			<Button
				variant="destructive"
				onclick={async () => {
					showDeleteModal = false;
					if (!bot) return;
					const result = await api(`/admin/bots/${bot.id}`, { method: 'DELETE' });
					if (result.ok) {
						await goto('/bots');
					} else {
						errorMessage = result.error;
					}
				}}
			>
				{t('common.delete', 'Delete')}
			</Button>
		</div>
	</Modal>
{/if}
