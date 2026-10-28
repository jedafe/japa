<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	/**
	 * TOTP 2FA enrollment flow — Section 02 (Authentication & Sessions).
	 *
	 * Three states: disabled → enrolling → enabled.
	 *
	 * - disabled:  Shows "Enable 2FA" button → calls POST /me/totp/enable
	 * - enrolling: Shows QR code (from otpauth_url) + secret for manual entry,
	 *              code input → calls POST /me/totp/confirm
	 * - enabled:   Shows "Disable 2FA" button → prompts for TOTP code →
	 *              calls DELETE /me/totp with the code
	 *
	 * Backup codes are shown once after successful enrollment.
	 *
	 * Ref: japaconnect-engineering/sections/02-authentication-sessions.md (2FA TOTP)
	 */
	import { Modal } from '$lib/components/ui';
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { totpEnable, totpConfirm, totpDisable } from '$lib/api/hub';
	import QRCode from 'qrcode';
	import { ShieldCheck, ShieldAlert, Smartphone, Copy, Check } from 'lucide-svelte';

	type Step = 'disabled' | 'enrolling' | 'enabled';

	let { enabled = false }: { enabled?: boolean } = $props();

	let step = $state<Step>(untrack(() => (enabled ? 'enabled' : 'disabled')));
	let secret = $state('');
	let otpauthUrl = $state('');
	let qrDataUrl = $state('');
	let code = $state('');
	let backupCodes = $state<string[]>([]);
	let error = $state<string | null>(null);
	let loading = $state(false);
	let disableOpen = $state(false);
	let disableCode = $state('');
	let backupOpen = $state(false);
	let copiedSecret = $state(false);

	// Generate QR code data URL from the otpauth URL.
	async function generateQR(url: string) {
		try {
			qrDataUrl = await QRCode.toDataURL(url, {
				width: 200,
				margin: 1,
				color: { dark: '#000000', light: '#ffffff' }
			});
		} catch {
			qrDataUrl = '';
		}
	}

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
		await generateQR(otpauthUrl);
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
		backupOpen = backupCodes.length > 0;
		step = 'enabled';
		code = '';
	}

	async function doDisable() {
		error = null;
		if (!disableCode.trim()) {
			error = t('settings.security.totp_disable_code_required');
			return;
		}
		loading = true;
		const res = await totpDisable(disableCode.trim());
		loading = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		disableOpen = false;
		disableCode = '';
		step = 'disabled';
		secret = '';
		otpauthUrl = '';
		qrDataUrl = '';
		backupCodes = [];
	}

	async function copySecret() {
		try {
			await navigator.clipboard.writeText(secret);
			copiedSecret = true;
			setTimeout(() => (copiedSecret = false), 2000);
		} catch {
			// ignore
		}
	}

	function openDisableDialog() {
		disableCode = '';
		error = null;
		disableOpen = true;
	}
</script>

<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
	<div class="mb-3 flex items-center gap-2">
		{#if step === 'enabled'}
			<ShieldCheck size={18} class="text-green-600" />
		{:else}
			<ShieldAlert size={18} class="text-[var(--color-muted)]" />
		{/if}
		<h2 class="text-sm font-medium text-[var(--color-text)]">{t('settings.security.totp_title')}</h2>
	</div>

	{#if error}<p class="mb-3 text-xs text-[var(--color-danger)]">{error}</p>{/if}

	{#if step === 'disabled'}
		<p class="mb-3 text-sm text-[var(--color-muted)]">{t('settings.security.totp_disabled_desc')}</p>
		<button
			type="button"
			disabled={loading}
			class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
			onclick={startEnable}
		>
			<Smartphone size={16} />
			{loading ? t('settings.security.totp_loading') : t('settings.security.totp_enable')}
		</button>

	{:else if step === 'enrolling'}
		<p class="mb-4 text-sm text-[var(--color-muted)]">{t('settings.security.totp_enroll_desc')}</p>

		<!-- QR code -->
		{#if qrDataUrl}
			<div class="mb-4 flex justify-center">
				<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-white p-3">
					<img src={qrDataUrl} alt={t('settings.security.totp_qr_alt')} class="h-[200px] w-[200px]" />
				</div>
			</div>
		{/if}

		<!-- Secret for manual entry -->
		<div class="mb-4 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] p-3">
			<div class="mb-1 flex items-center justify-between">
				<span class="text-xs text-[var(--color-muted)]">{t('settings.security.totp_secret')}</span>
				<button
					type="button"
					class="inline-flex items-center gap-1 text-xs text-[var(--color-primary)] hover:underline"
					onclick={copySecret}
				>
					{#if copiedSecret}
						<Check size={12} /> {t('settings.security.totp_copied')}
					{:else}
						<Copy size={12} /> {t('settings.security.totp_copy')}
					{/if}
				</button>
			</div>
			<code class="block break-all font-mono text-sm text-[var(--color-text)]">{secret}</code>
		</div>

		<!-- Code input -->
		<label class="mb-3 block">
			<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.security.totp_code')}</span>
			<input
				type="text"
				inputmode="numeric"
				maxlength="6"
				placeholder="000000"
				aria-describedby="totp-enroll-code-hint"
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
				value={code}
				oninput={(e) => (code = e.currentTarget.value.replace(/\D/g, ''))}
			/>
			<HintIcon id="totp-enroll-code-hint" text={t('auth.totp.code_help')} />
		</label>
		<div class="flex gap-2">
			<button
				type="button"
				disabled={loading || code.length < 6}
				class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
				onclick={confirmCode}
			>
				{loading ? t('settings.security.totp_loading') : t('settings.security.totp_confirm')}
			</button>
			<button
				type="button"
				class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				onclick={() => { step = 'disabled'; secret = ''; otpauthUrl = ''; qrDataUrl = ''; code = ''; error = null; }}
			>
				{t('common.cancel')}
			</button>
		</div>

	{:else if step === 'enabled'}
		<p class="mb-3 inline-flex items-center gap-1.5 text-sm text-green-600">
			<ShieldCheck size={14} />
			{t('settings.security.totp_enabled')}
		</p>

		<button
			type="button"
			class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
			onclick={openDisableDialog}
		>
			{t('settings.security.totp_disable')}
		</button>

		<!-- Disable confirmation dialog — requires TOTP code -->
		<Modal bind:open={disableOpen} title={t('settings.security.totp_disable_confirm')} description={t('settings.security.totp_disable_desc')} size="md">
			{#snippet children(close)}
					<label class="mb-4 block">
						<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.security.totp_code')}</span>
						<input
							type="text"
							inputmode="numeric"
							maxlength="6"
							placeholder="000000"
							aria-describedby="totp-disable-code-hint"
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
							value={disableCode}
							oninput={(e) => (disableCode = e.currentTarget.value.replace(/\D/g, ''))}
						/>
						<HintIcon id="totp-disable-code-hint" text={t('auth.totp.code_help')} />
					</label>
					{#if error}<p class="mb-3 text-xs text-[var(--color-danger)]">{error}</p>{/if}
					<div class="flex justify-end gap-2">
						<button type="button" onclick={close} class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)]">
							{t('common.cancel')}
						</button>
						<button
							type="button"
							disabled={loading || disableCode.length < 6}
							class="rounded-[var(--radius)] bg-[var(--color-danger)] px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50"
							onclick={doDisable}
						>
							{loading ? t('settings.security.totp_loading') : t('settings.security.totp_disable')}
						</button>
					</div>
			{/snippet}
		</Modal>

		<!-- Backup codes dialog (shown once after enrollment) -->
		{#if backupCodes.length > 0}
			<Modal bind:open={backupOpen} title={t('settings.security.totp_backup_codes')} description={t('settings.security.totp_backup_desc')} size="md">
				{#snippet children(close)}
						<div class="mb-4 grid grid-cols-2 gap-2 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] p-3">
							{#each backupCodes as code_item (code_item)}
								<code class="font-mono text-sm text-[var(--color-text)]">{code_item}</code>
							{/each}
						</div>
						<div class="flex justify-end">
							<button type="button" onclick={close} class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-1.5 text-sm font-medium text-white">
								{t('settings.security.totp_backup_ack')}
							</button>
						</div>
				{/snippet}
			</Modal>
		{/if}
	{/if}
</div>
