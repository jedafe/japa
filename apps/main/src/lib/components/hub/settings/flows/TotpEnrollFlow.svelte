<script lang="ts">
	import { Dialog } from 'bits-ui';
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { totpEnable, totpConfirm, totpDisable } from '$lib/api/hub';

	type Step = 'disabled' | 'enrolling' | 'enabled';

	let { enabled = false }: { enabled?: boolean } = $props();

	let step = $state<Step>(untrack(() => (enabled ? 'enabled' : 'disabled')));
	let secret = $state('');
	let otpauthUrl = $state('');
	let code = $state('');
	let backupCodes = $state<string[]>([]);
	let error = $state<string | null>(null);
	let loading = $state(false);
	let disableOpen = $state(false);
	let backupOpen = $state(true);

	async function startEnable() {
		error = null;
		loading = true;
		const res = await totpEnable();
		loading = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		secret = res.data?.secret ?? '';
		otpauthUrl = res.data?.otpauth_url ?? '';
		step = 'enrolling';
	}

	async function confirmCode() {
		error = null;
		loading = true;
		const res = await totpConfirm(code);
		loading = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		backupCodes = res.data?.backup_codes ?? [];
		backupOpen = true;
		step = 'enabled';
		code = '';
	}

	async function doDisable() {
		disableOpen = false;
		error = null;
		loading = true;
		const res = await totpDisable();
		loading = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		step = 'disabled';
		secret = '';
		otpauthUrl = '';
		backupCodes = [];
	}
</script>

<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
	<h2 class="mb-2 text-sm font-medium text-[var(--color-text)]">{t('settings.security.totp_title')}</h2>
	{#if error}<p class="mb-2 text-xs text-[var(--color-danger,#dc2626)]">{error}</p>{/if}

	{#if step === 'disabled'}
		<p class="mb-3 text-sm text-[var(--color-muted)]">{t('settings.security.totp_disabled_desc')}</p>
		<button
			type="button"
			disabled={loading}
			class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
			onclick={startEnable}
		>
			{t('settings.security.totp_enable')}
		</button>
	{:else if step === 'enrolling'}
		<p class="mb-2 text-sm text-[var(--color-muted)]">{t('settings.security.totp_enroll_desc')}</p>
		<div class="mb-3 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] p-3">
			<p class="mb-1 text-xs text-[var(--color-muted)]">{t('settings.security.totp_secret')}</p>
			<code class="block break-all text-sm text-[var(--color-text)]">{secret}</code>
		</div>
		<label class="mb-3 block">
			<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.security.totp_code')}</span>
			<input
				type="text"
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
				value={code}
				oninput={(e) => (code = e.currentTarget.value)}
			/>
		</label>
		<button
			type="button"
			disabled={loading || !code}
			class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
			onclick={confirmCode}
		>
			{t('settings.security.totp_confirm')}
		</button>
	{:else if step === 'enabled'}
		<p class="mb-3 text-sm text-green-600">{t('settings.security.totp_enabled')}</p>
		<Dialog.Root bind:open={disableOpen}>
			<Dialog.Trigger
				class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
			>
				{t('settings.security.totp_disable')}
			</Dialog.Trigger>
			<Dialog.Portal>
				<Dialog.Content class="z-50 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 shadow-lg">
					<Dialog.Title class="mb-2 text-lg font-medium text-[var(--color-text)]">
						{t('settings.security.totp_disable_confirm')}
					</Dialog.Title>
					<div class="flex justify-end gap-2">
						<Dialog.Close class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)]">
							{t('common.cancel')}
						</Dialog.Close>
						<button
							type="button"
							class="rounded-[var(--radius)] bg-[var(--color-danger,#dc2626)] px-4 py-1.5 text-sm font-medium text-white"
							onclick={doDisable}
						>
							{t('settings.security.totp_disable')}
						</button>
					</div>
				</Dialog.Content>
			</Dialog.Portal>
		</Dialog.Root>

		{#if backupCodes.length > 0}
			<Dialog.Root bind:open={backupOpen}>
				<Dialog.Portal>
					<Dialog.Content class="z-50 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 shadow-lg">
						<Dialog.Title class="mb-2 text-lg font-medium text-[var(--color-text)]">
							{t('settings.security.totp_backup_codes')}
						</Dialog.Title>
						<Dialog.Description class="mb-4 text-sm text-[var(--color-muted)]">
							{t('settings.security.totp_backup_desc')}
						</Dialog.Description>
						<div class="mb-4 grid grid-cols-2 gap-2 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] p-3">
							{#each backupCodes as code_item (code_item)}
								<code class="text-sm text-[var(--color-text)]">{code_item}</code>
							{/each}
						</div>
						<div class="flex justify-end">
							<Dialog.Close class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-1.5 text-sm font-medium text-white">
								{t('settings.security.totp_backup_ack')}
							</Dialog.Close>
						</div>
					</Dialog.Content>
				</Dialog.Portal>
			</Dialog.Root>
		{/if}
	{/if}
</div>
