<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { deleteDevice, deletePushToken, getVapidKey, getPushTokens, subscribePush } from '$lib/api/hub';
	import { optimisticAction } from '$lib/components/hub/optimisticAction';
	import { pwaInstall } from '@japaconnect/ui/pwa-install';
	import { arrayBufferToBase64, urlBase64ToUint8Array } from '$lib/pwa/push-sync';
	import type { DeviceInfo } from '$lib/types';

	interface PushToken {
		id: string;
		endpoint: string;
	}

	interface DevicesData {
		devices?: DeviceInfo[];
		pushTokens?: PushToken[];
	}

	let { data }: { data: unknown } = $props();
	const payload = $derived(data as DevicesData | null);
	let devices = $state<DeviceInfo[]>(untrack(() => payload?.devices ?? []));
	let pushTokens = $state<PushToken[]>(untrack(() => payload?.pushTokens ?? []));

	// Resync when the loader data is replaced (e.g. invalidateAll) — the
	// untracked init alone leaves both lists permanently stale (BUG-P014).
	// Reads only `payload` so optimistic removals don't retrigger it.
	$effect(() => {
		devices = payload?.devices ?? [];
		pushTokens = payload?.pushTokens ?? [];
	});
	let pushLoading = $state(false);
	let pushError = $state<string | null>(null);
	let installLoading = $state(false);

	// §124 — install row mirrors the banner; gated by pwa.install_prompt_enabled
	// and hidden entirely once running standalone.
	const installPromptEnabled = $derived(
		page.data.siteSettings?.['pwa.install_prompt_enabled'] !== false
	);

	function deviceName(device: DeviceInfo): string {
		const platform = device.platform || t('settings.devices.unknown', 'Unknown device');
		const app = device.app_version ? ` (${device.app_version})` : '';
		return `${platform}${app}`;
	}

	async function doRemove(deviceID: string) {
		const idx = devices.findIndex((d) => d.device_id === deviceID);
		if (idx < 0) return;
		const removed = devices[idx];
		await optimisticAction({
			apply: () => (devices = devices.filter((d) => d.device_id !== deviceID)),
			rollback: () => (devices = [...devices.slice(0, idx), removed, ...devices.slice(idx + 1)]),
			call: () => deleteDevice(deviceID)
		});
	}

	async function doRemovePushToken(id: string) {
		const idx = pushTokens.findIndex((t) => t.id === id);
		if (idx < 0) return;
		const removed = pushTokens[idx];
		await optimisticAction({
			apply: () => (pushTokens = pushTokens.filter((t) => t.id !== id)),
			rollback: () => (pushTokens = [...pushTokens.slice(0, idx), removed, ...pushTokens.slice(idx + 1)]),
			call: () => deletePushToken(id)
		});
		// Drop the browser-side subscription too when the removed token is
		// this browser's — otherwise the push-sync reconciliation sees an
		// unregistered endpoint and re-registers it on the next visit,
		// resurrecting a subscription the user explicitly removed.
		try {
			const reg = await navigator.serviceWorker.ready;
			const sub = await reg.pushManager.getSubscription();
			if (sub && sub.endpoint === removed.endpoint) await sub.unsubscribe();
		} catch {
			// Best effort — the server row is already gone.
		}
	}

	async function enablePush() {
		if (pushLoading) return;
		if (typeof window === 'undefined' || !('serviceWorker' in navigator) || !('PushManager' in window)) {
			pushError = t('settings.devices.push_unsupported', 'Browser push is not supported in this environment.');
			return;
		}
		pushLoading = true;
		pushError = null;
		try {
			const vapid = await getVapidKey();
			if (vapid.error || !vapid.data?.publicKey) throw new Error(vapid.error?.message ?? 'No VAPID key');
			const reg = await navigator.serviceWorker.ready;
			const existing = await reg.pushManager.getSubscription();
			if (existing) {
				await existing.unsubscribe();
				// Drop the stale endpoint server-side too — the browser
				// subscription is dead, but the push_subscriptions row would
				// otherwise linger until a failed delivery prunes it.
				const stale = pushTokens.find((pt) => pt.endpoint === existing.endpoint);
				if (stale) await deletePushToken(stale.id).catch(() => {});
			}
			const sub = await reg.pushManager.subscribe({
				userVisibleOnly: true,
				applicationServerKey: urlBase64ToUint8Array(vapid.data.publicKey) as BufferSource
			});
			const res = await subscribePush({
				endpoint: sub.endpoint,
				p256dh: arrayBufferToBase64(sub.getKey('p256dh')),
				auth: arrayBufferToBase64(sub.getKey('auth'))
			});
			if (res.error) throw new Error(res.error.message);
			const fresh = await getPushTokens();
			if (!fresh.error && Array.isArray(fresh.data)) pushTokens = fresh.data as PushToken[];
		} catch (e) {
			pushError = e instanceof Error ? e.message : t('settings.devices.push_error', 'Could not enable push notifications.');
		} finally {
			pushLoading = false;
		}
	}

	async function installApp() {
		if (installLoading) return;
		installLoading = true;
		try {
			await pwaInstall.prompt();
		} catch {
			// The native dialog can reject (e.g. rapid re-trigger) — the button
			// just resets.
		} finally {
			installLoading = false;
		}
	}
</script>

<section class="max-w-2xl space-y-6">
	<header>
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('settings.devices.title')}</h1>
		<p class="text-sm text-[var(--color-muted)]">{t('settings.devices.desc')}</p>
	</header>

	{#if installPromptEnabled && !pwaInstall.installed && (pwaInstall.canPrompt || pwaInstall.needsManualIos)}
		<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{t('settings.devices.install_app', 'Install app')}</h2>
			<div class="flex items-center justify-between gap-3">
				<p class="text-sm text-[var(--color-muted)]">
					{#if pwaInstall.needsManualIos}
						{t('settings.devices.install_ios', 'Use the browser Share menu, then "Add to Home Screen".')}
					{:else}
						{t('settings.devices.install_desc', 'Install this app for faster access and a native feel.')}
					{/if}
				</p>
				{#if pwaInstall.canPrompt}
					<button
						type="button"
						disabled={installLoading}
						class="shrink-0 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-white disabled:opacity-50"
						onclick={installApp}
					>
						{installLoading ? t('settings.devices.install_working', 'Installing…') : t('settings.devices.install_button', 'Install')}
					</button>
				{/if}
			</div>
		</div>
	{/if}

	<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{t('settings.devices.browser_push', 'Browser push')}</h2>

		<div class="mb-4 flex items-center justify-between gap-3">
			<p class="text-sm text-[var(--color-muted)]">{t('settings.devices.push_desc', 'Get notifications in this browser when the page is closed.')}</p>
			<button
				type="button"
				disabled={pushLoading}
				class="shrink-0 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-white disabled:opacity-50"
				onclick={enablePush}
			>
				{pushLoading ? t('settings.devices.push_enabling', 'Enabling…') : t('settings.devices.push_enable', 'Enable')}
			</button>
		</div>
		{#if pushError}<p class="mb-3 text-xs text-[var(--color-danger,#dc2626)]">{pushError}</p>{/if}

		{#if pushTokens.length === 0}
			<p class="py-2 text-center text-sm text-[var(--color-muted)]">{t('settings.devices.push_empty', 'No browser push subscriptions.')}</p>
		{:else}
			<ul class="space-y-2">
				{#each pushTokens as token (token.id)}
					<li class="flex items-center justify-between rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-2">
						<p class="max-w-[80%] truncate text-sm text-[var(--color-text)]">{token.endpoint}</p>
						<button
							type="button"
							class="text-xs text-[var(--color-muted)] hover:text-[var(--color-danger,#dc2626)]"
							onclick={() => doRemovePushToken(token.id)}
						>
							{t('settings.devices.remove')}
						</button>
					</li>
				{/each}
			</ul>
		{/if}
	</div>

	<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{t('settings.devices.title')}</h2>

		{#if devices.length === 0}
			<p class="py-4 text-center text-sm text-[var(--color-muted)]">{t('settings.devices.empty')}</p>
		{:else}
			<ul class="space-y-2">
				{#each devices as device (device.device_id)}
					<li
						class="flex items-center justify-between rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-2"
					>
						<div>
							<p class="text-sm text-[var(--color-text)]">{deviceName(device)}</p>
							<p class="text-xs text-[var(--color-muted)]">
								{device.is_active ? t('settings.devices.active', 'Active') : t('settings.devices.inactive', 'Inactive')}
								{#if device.last_used_at}
									&middot; {new Date(device.last_used_at).toLocaleString()}
								{/if}
							</p>
						</div>
						<button
							type="button"
							class="text-xs text-[var(--color-muted)] hover:text-[var(--color-danger,#dc2626)]"
							onclick={() => doRemove(device.device_id)}
						>
							{t('settings.devices.remove')}
						</button>
					</li>
				{/each}
			</ul>
		{/if}
	</div>
</section>
