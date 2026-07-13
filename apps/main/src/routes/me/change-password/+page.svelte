<script lang="ts">
	import { onMount } from 'svelte';
	import { request } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, Lock } from 'lucide-svelte';

	let currentPassword = $state('');
	let newPassword = $state('');
	let confirmPassword = $state('');
	let loading = $state(true);
	let errorMessage = $state('');
	let saving = $state(false);
	let successMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		successMessage = '';
		try {
			await request<void>('/me');
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function changePassword() {
		if (!currentPassword || !newPassword || newPassword !== confirmPassword) return;
		saving = true;
		errorMessage = '';
		successMessage = '';
		try {
			await request<void>('/me/change-password', { method: 'POST', body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }) });
			successMessage = t('me.changePassword.success');
			currentPassword = '';
			newPassword = '';
			confirmPassword = '';
		} catch {
			errorMessage = t('common.error');
		} finally {
			saving = false;
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('me.changePassword.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.changePassword.title')}</h1>

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

<div class="max-w-xl rounded-lg border p-6" style="border-color: var(--color-border); background: var(--color-surface)">
	<div class="space-y-4">
		<div>
			<label for="current" class="mb-1 block text-sm font-medium">{t('me.changePassword.current')}</label>
			<input id="current" type="password" bind:value={currentPassword} required class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)" />
		</div>
		<div>
			<label for="new" class="mb-1 block text-sm font-medium">{t('me.changePassword.new')}</label>
			<input id="new" type="password" bind:value={newPassword} required class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)" />
		</div>
		<div>
			<label for="confirm" class="mb-1 block text-sm font-medium">{t('me.changePassword.confirm')}</label>
			<input id="confirm" type="password" bind:value={confirmPassword} required class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)" />
		</div>
		<button class="btn btn-primary text-xs" onclick={changePassword} disabled={saving}>
			<Lock class="h-3 w-3 inline mr-1" />
			{saving ? t('common.saving') : t('me.changePassword.submit')}
		</button>
	</div>
</div>
