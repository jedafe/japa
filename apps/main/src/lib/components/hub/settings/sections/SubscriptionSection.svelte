<script lang="ts">
	import { Dialog } from 'bits-ui';
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { cancelSubscription, reactivateSubscription } from '$lib/api/hub';
	import { optimisticAction } from '$lib/components/hub/optimisticAction';
	import { invalidateAll } from '$app/navigation';

	interface Subscription {
		plan?: string;
		status?: string;
		renews_at?: string;
	}

	let { data }: { data: unknown } = $props();
	let sub = $state<Subscription>(untrack(() => (data as Subscription | null) ?? {}));
	let cancelOpen = $state(false);
	let loading = $state(false);
	let error = $state<string | null>(null);

	const isCancelled = $derived(sub.status === 'cancelled' || sub.status === 'canceled');

	async function doCancel() {
		cancelOpen = false;
		error = null;
		loading = true;
		const res = await optimisticAction({
			apply: () => (sub = { ...sub, status: 'cancelled' }),
			rollback: () => (sub = (data as Subscription | null) ?? {}),
			call: () => cancelSubscription()
		});
		loading = false;
		if (!res.ok) error = res.error?.message ?? null;
		await invalidateAll();
	}

	async function doReactivate() {
		error = null;
		loading = true;
		const res = await optimisticAction({
			apply: () => (sub = { ...sub, status: 'active' }),
			rollback: () => (sub = (data as Subscription | null) ?? {}),
			call: () => reactivateSubscription()
		});
		loading = false;
		if (!res.ok) error = res.error?.message ?? null;
		await invalidateAll();
	}
</script>

<section class="max-w-2xl">
	<header class="mb-4">
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('settings.nav.subscription')}</h1>
		<p class="text-sm text-[var(--color-muted)]">{t('settings.subscription.desc')}</p>
	</header>

	<div class="mb-6 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<dl class="space-y-2">
			<div class="flex justify-between">
				<dt class="text-sm text-[var(--color-muted)]">{t('settings.subscription.plan')}</dt>
				<dd class="text-sm font-medium text-[var(--color-text)]">{sub.plan ?? '—'}</dd>
			</div>
			<div class="flex justify-between">
				<dt class="text-sm text-[var(--color-muted)]">{t('settings.subscription.status')}</dt>
				<dd class="text-sm font-medium text-[var(--color-text)]">{sub.status ?? '—'}</dd>
			</div>
			<div class="flex justify-between">
				<dt class="text-sm text-[var(--color-muted)]">{t('settings.subscription.renews')}</dt>
				<dd class="text-sm text-[var(--color-text)]">{sub.renews_at ?? '—'}</dd>
			</div>
		</dl>
	</div>

	{#if error}<p class="mb-4 text-xs text-[var(--color-danger,#dc2626)]">{error}</p>{/if}

	{#if isCancelled}
		<button
			type="button"
			disabled={loading}
			class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
			onclick={doReactivate}
		>
			{loading ? '...' : t('settings.subscription.reactivate')}
		</button>
	{:else}
		<Dialog.Root bind:open={cancelOpen}>
			<Dialog.Trigger
				class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
			>
				{t('settings.subscription.cancel')}
			</Dialog.Trigger>
			<Dialog.Portal>
				<Dialog.Content class="z-50 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 shadow-lg">
					<Dialog.Title class="mb-2 text-lg font-medium text-[var(--color-text)]">
						{t('settings.subscription.cancel_confirm_title')}
					</Dialog.Title>
					<Dialog.Description class="mb-4 text-sm text-[var(--color-muted)]">
						{t('settings.subscription.cancel_confirm_body')}
					</Dialog.Description>
					<div class="flex justify-end gap-2">
						<Dialog.Close
							class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)]"
						>
							{t('common.cancel')}
						</Dialog.Close>
						<button
							type="button"
							disabled={loading}
							class="rounded-[var(--radius)] bg-[var(--color-danger,#dc2626)] px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50"
							onclick={doCancel}
						>
							{loading ? '...' : t('settings.subscription.cancel_confirm')}
						</button>
					</div>
				</Dialog.Content>
			</Dialog.Portal>
		</Dialog.Root>
	{/if}
</section>
