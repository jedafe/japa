<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type Bot = {
		id: string;
		name: string;
		avatar_url: string;
		description: string;
		rate_limit: number;
		created_at: string;
	};

	let bot = $state<Bot | null>(null);
	let loading = $state(true);
	let errorMessage = $state('');
	let successMessage = $state('');

	// Edit form
	let editName = $state('');
	let editAvatarUrl = $state('');
	let editDescription = $state('');
	let rateLimitValue = $state(100);
	let saving = $state(false);

	const botId = $derived(page.params.id);

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<Bot>(`/admin/bots/${botId}`);
			if (result.ok) {
				bot = result.data;
				editName = bot.name;
				editAvatarUrl = bot.avatar_url ?? '';
				editDescription = bot.description ?? '';
				rateLimitValue = bot.rate_limit ?? 100;
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('bots.load_error');
		} finally {
			loading = false;
		}
	}

	async function saveBot() {
		if (!bot || !editName.trim()) return;
		saving = true;
		errorMessage = '';
		successMessage = '';
		try {
			const result = await api(`/admin/bots/${bot.id}`, {
				method: 'PATCH',
				body: {
					name: editName.trim(),
					avatar_url: editAvatarUrl.trim() || null,
					description: editDescription.trim() || null
				}
			});
			if (result.ok) {
				successMessage = t('bots.updated');
				bot.name = editName.trim();
				bot.avatar_url = editAvatarUrl.trim();
				bot.description = editDescription.trim();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('bots.update_error');
		} finally {
			saving = false;
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{bot?.name ?? 'Bot'} — JapaConnect Admin</title>
</svelte:head>

<div class="mb-4">
	<a href="/bots" class="text-sm hover:underline" style="color: var(--color-muted)">&larr; {t('bots.back')}</a>
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
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else if bot}
	<h1 class="mb-1 text-2xl font-bold tracking-tight">{bot.name}</h1>
	<p class="mb-6 text-sm" style="color: var(--color-muted)">
		Created: {bot.created_at}
	</p>

	<div class="rounded-lg border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<h2 class="mb-4 text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('bots.bot_details')}</h2>
		<div class="space-y-4">
			<div>
				<label for="edit-name" class="mb-1 block text-sm font-medium">{t('bots.name')}</label>
				<input
					id="edit-name"
					type="text"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					bind:value={editName}
				/>
			</div>
			<div>
				<label for="edit-avatar" class="mb-1 block text-sm font-medium">{t('bots.avatar_url')}</label>
				<input
					id="edit-avatar"
					type="url"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					placeholder="https://example.com/avatar.png"
					bind:value={editAvatarUrl}
				/>
			</div>
			<div>
				<label for="edit-desc" class="mb-1 block text-sm font-medium">{t('bots.description_label')}</label>
				<textarea
					id="edit-desc"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					rows="3"
					bind:value={editDescription}
				></textarea>
			</div>
			<div class="flex items-center gap-3 pt-2">
				<button
					class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
					style="background-color: var(--color-primary)"
					disabled={saving || !editName.trim()}
					onclick={saveBot}
				>
					{saving ? t('common.saving') : t('bots.save_changes')}
				</button>
				<button
					class="rounded-md px-4 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
					style="color: var(--color-danger)"
					onclick={async () => {
						if (!bot) return;
						if (!confirm(t('bots.delete_confirm'))) return;
						const result = await api(`/admin/bots/${bot.id}`, { method: 'DELETE' });
						if (result.ok) {
							await goto('/bots');
						} else {
							errorMessage = result.error;
						}
					}}
				>
					{t('bots.delete_bot')}
				</button>
			</div>
		</div>
	</div>
{/if}
