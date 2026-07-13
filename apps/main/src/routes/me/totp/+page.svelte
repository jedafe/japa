<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { request } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, Shield } from 'lucide-svelte';

	type TotpStatus = {
		enabled: boolean;
		qr_code?: string;
	};

	let { data } = $props();
	let status = $derived(data.data ?? null);
	let errorMessage = $state('');
	let enabling = $state(false);
	let confirming = $state(false);
	let secret = $state('');
	let code = $state('');

	async function enableTotp() {
		enabling = true;
		errorMessage = '';
		try {
			const result = await request<{ secret: string; otpauth_url: string }>('/me/totp/enable', { method: 'POST' });
			secret = result.secret;
			status = { enabled: false, qr_code: result.otpauth_url };
		} catch {
			errorMessage = t('common.error');
		} finally {
			enabling = false;
		}
	}

	async function confirmTotp() {
		if (!code.trim()) return;
		confirming = true;
		errorMessage = '';
		try {
			await request<{ status: string }>('/me/totp/confirm', { method: 'POST', body: JSON.stringify({ code: code.trim() }) });
			await invalidateAll();
		} catch {
			errorMessage = t('common.error');
		} finally {
			confirming = false;
		}
	}
</script>

<svelte:head>
	<title>{t('me.totp.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.totp.title')}</h1>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

{#if !data.data}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else if status}
	<div class="max-w-xl rounded-lg border p-6" style="border-color: var(--color-border); background: var(--color-surface)">
		<div class="flex items-center gap-3 mb-4">
			<Shield class="h-5 w-5" style="color: var(--color-primary)" />
			<p class="font-medium">{status.enabled ? t('me.totp.enabled') : t('me.totp.disabled')}</p>
		</div>
		{#if !status.enabled}
			{#if status.qr_code}
				<img src={status.qr_code} alt="QR Code" class="mb-4 h-40 w-40" />
				<p class="mb-2 text-xs" style="color: var(--color-muted)">{t('me.totp.secret')}: {secret}</p>
				<div class="flex gap-2">
					<input type="text" bind:value={code} placeholder={t('me.totp.code_placeholder')} class="flex-1 rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)" />
					<button class="btn btn-primary text-xs" onclick={confirmTotp} disabled={confirming}>{confirming ? t('common.loading') : t('me.totp.confirm')}</button>
				</div>
			{:else}
				<button class="btn btn-primary text-xs" onclick={enableTotp} disabled={enabling}>{enabling ? t('common.loading') : t('me.totp.enable')}</button>
			{/if}
		{/if}
	</div>
{/if}
