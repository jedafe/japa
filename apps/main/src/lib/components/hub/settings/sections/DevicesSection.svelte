<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { deleteDevice } from '$lib/api/hub';
	import { optimisticAction } from '$lib/components/hub/optimisticAction';
	import type { DeviceInfo } from '$lib/types';

	interface DevicesData {
		devices?: DeviceInfo[];
	}

	let { data }: { data: unknown } = $props();
	const payload = $derived(data as DevicesData | null);
	let devices = $state<DeviceInfo[]>(untrack(() => payload?.devices ?? []));

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
</script>

<section class="max-w-2xl space-y-6">
	<header>
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('settings.devices.title')}</h1>
		<p class="text-sm text-[var(--color-muted)]">{t('settings.devices.desc')}</p>
	</header>

	<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{t('settings.devices.title')}</h2>

		{#if devices.length === 0}
			<p class="py-4 text-center text-sm text-[var(--color-muted)]">{t('settings.devices.empty')}</p>
		{:else}
			<ul class="space-y-2">
				{#each devices as device (device.id)}
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
