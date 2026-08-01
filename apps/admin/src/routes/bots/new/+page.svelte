<script lang="ts">
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Select } from '$lib/components/ui/select';

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

	let name = $state('');
	let userAgentPattern = $state('');
	let botType = $state('search_engine');
	let policy = $state('allow');
	let crawlDelay = $state(0);
	let creating = $state(false);
	let errorMessage = $state('');

	async function createBot() {
		if (!name.trim() || !userAgentPattern.trim()) return;
		creating = true;
		errorMessage = '';
		try {
			const result = await api('/admin/bots', {
				method: 'POST',
				body: {
					name: name.trim(),
					user_agent_pattern: userAgentPattern.trim(),
					bot_type: botType,
					policy: policy,
					crawl_delay: crawlDelay
				}
			});
			if (result.ok) {
				const bot = result.data as { id: string };
				await goto(`/bots/${bot.id}`);
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('bots.create_error', 'Failed to create bot');
		} finally {
			creating = false;
		}
	}
</script>

<svelte:head>
	<title>{t('app.bots.create_bot', 'Create Bot')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('app.bots.create_bot', 'Create Bot')}</h1>
<p class="mb-6 text-sm" style="color: var(--color-muted)">{t('app.bots.description', 'Manage known crawlers, bots, and their access policies')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mx-auto max-w-lg rounded-lg border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
	<div class="space-y-4">
		<div>
			<label for="bot-name" class="mb-1 block text-sm font-medium">{t('bots.name', 'Name')} <span style="color: var(--color-danger)">*</span></label>
			<input
				id="bot-name"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-background)"
				placeholder={t('app.bots.name_placeholder', 'e.g., Googlebot')}
				bind:value={name}
			/>
		</div>

		<div>
			<label for="bot-ua" class="mb-1 block text-sm font-medium">{t('app.bots.user_agent_pattern', 'User-Agent Pattern')} <span style="color: var(--color-danger)">*</span></label>
			<input
				id="bot-ua"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-background)"
				placeholder={t('app.bots.ua_placeholder', 'e.g., Googlebot')}
				bind:value={userAgentPattern}
			/>
		</div>

		<div>
			<label for="bot-type" class="mb-1 block text-sm font-medium">{t('bots.type', 'Type')}</label>
			<Select options={BOT_TYPES} bind:value={botType} />
		</div>

		<div>
			<label for="bot-policy" class="mb-1 block text-sm font-medium">{t('app.bots.policy', 'Policy')}</label>
			<Select options={POLICIES} bind:value={policy} />
		</div>

		<div>
			<label for="bot-delay" class="mb-1 block text-sm font-medium">{t('app.bots.crawl_delay', 'Crawl Delay (seconds)')}</label>
			<input
				id="bot-delay"
				type="number"
				min="0"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-background)"
				bind:value={crawlDelay}
			/>
		</div>

		<div class="flex items-center gap-3 pt-2">
			<button
				class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
				style="background-color: var(--color-primary)"
				disabled={creating || !name.trim() || !userAgentPattern.trim()}
				onclick={createBot}
			>
				{creating ? t('bots.creating', 'Creating...') : t('app.bots.create_bot', 'Create Bot')}
			</button>
			<a href="/bots" class="rounded-md px-4 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-surface/10" style="color: var(--color-muted)">
				{t('common.cancel', 'Cancel')}
			</a>
		</div>
	</div>
</div>
