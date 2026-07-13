<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { request } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, Shield, ShieldOff } from 'lucide-svelte';

	type Subscription = {
		status: string;
		plan: string;
		renews_at?: string;
		canceled_at?: string;
	};

	let { data } = $props();
	let subscription = $derived(data.data ?? null);
	let errorMessage = $state('');
	let canceling = $state(false);
	let reactivating = $state(false);

	async function cancelSubscription() {
		canceling = true;
		errorMessage = '';
		try {
			await request<void>('/me/subscription/cancel', { method: 'POST' });
			await invalidateAll();
		} catch {
			errorMessage = t('common.error');
		} finally {
			canceling = false;
		}
	}

	async function reactivateSubscription() {
		reactivating = true;
		errorMessage = '';
		try {
			await request<void>('/me/subscription/reactivate', { method: 'POST' });
			await invalidateAll();
		} catch {
			errorMessage = t('common.error');
		} finally {
			reactivating = false;
		}
	}
</script>

<svelte:head>
	<title>{t('me.subscription.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.subscription.title')}</h1>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

{#if !data.data}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else if subscription}
	<div class="max-w-xl rounded-lg border p-6" style="border-color: var(--color-border); background: var(--color-surface)">
		<div class="flex items-center justify-between mb-4">
			<div class="flex items-center gap-3">
				<Shield class="h-5 w-5" style="color: var(--color-primary)" />
				<div>
					<p class="font-medium">{subscription.plan}</p>
					<p class="text-xs" style="color: var(--color-muted)">{t('me.subscription.status')}: {subscription.status}</p>
					{#if subscription.renews_at}
						<p class="text-xs" style="color: var(--color-muted)">{t('me.subscription.renews')}: {new Date(subscription.renews_at).toLocaleDateString()}</p>
					{/if}
				</div>
			</div>
		</div>
		<div class="flex gap-2">
			{#if subscription.status === 'active'}
				<button class="btn btn-secondary text-xs" onclick={cancelSubscription} disabled={canceling}>
					<ShieldOff class="h-3 w-3 inline mr-1" />
					{canceling ? t('common.loading') : t('me.subscription.cancel')}
				</button>
			{:else if subscription.status === 'canceled'}
				<button class="btn btn-primary text-xs" onclick={reactivateSubscription} disabled={reactivating}>
					<Shield class="h-3 w-3 inline mr-1" />
					{reactivating ? t('common.loading') : t('me.subscription.reactivate')}
				</button>
			{/if}
		</div>
	</div>
{/if}
