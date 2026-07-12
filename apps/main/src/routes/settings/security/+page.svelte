<script lang="ts">
	import { Shield, ShieldCheck, ShieldOff, QrCode, Loader2, Key } from 'lucide-svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { onMount } from 'svelte';

	let totpEnabled = $state(false);
	let loading = $state(false);
	let error = $state('');
	let successMessage = $state('');

	// Setup state
	let qrUri = $state('');
	let secret = $state('');
	let verificationCode = $state('');
	let setupPhase = $state(false);

	// Disable state
	let showDisableForm = $state(false);
	let disablePassword = $state('');
	let backupCodes = $state<string[]>([]);

	onMount(async () => {
		try {
			const user = await api.getMe();
			totpEnabled = !!(user as any).totp_enabled;
		} catch {
			// use defaults
		}
	});

	async function startSetup() {
		error = '';
		loading = true;
		try {
			const result = await api.setupTOTP();
			qrUri = result.otpauth_url;
			secret = result.secret;
			setupPhase = true;
		} catch (err) {
			error = err instanceof Error ? err.message : 'Network error. Please try again.';
		} finally {
			loading = false;
		}
	}

	async function verifyAndEnable(e: Event) {
		e.preventDefault();
		error = '';
		loading = true;

		try {
			await api.verifyTOTP(verificationCode);
			totpEnabled = true;
			setupPhase = false;
			backupCodes = [];
			successMessage = 'Two-factor authentication enabled successfully!';
		} catch (err) {
			error = err instanceof Error ? err.message : 'Network error. Please try again.';
		} finally {
			loading = false;
		}
	}

	async function handleDisable(e: Event) {
		e.preventDefault();
		error = '';
		loading = true;

		try {
			await api.disableTOTP(disablePassword);
			totpEnabled = false;
			showDisableForm = false;
			disablePassword = '';
			successMessage = 'Two-factor authentication disabled.';
		} catch (err) {
			error = err instanceof Error ? err.message : 'Network error. Please try again.';
		} finally {
			loading = false;
		}
	}

	function cancelSetup() {
		setupPhase = false;
		qrUri = '';
		secret = '';
		verificationCode = '';
		error = '';
	}
</script>

<svelte:head>
	<title>Security Settings | JapaConnect</title>
	<meta name="description" content="Manage two-factor authentication and security settings for your JapaConnect account." />
</svelte:head>

<main class="mx-auto max-w-3xl px-4 py-8">
	<div class="mb-6 flex items-center gap-3">
		<a href="/settings" class="text-sm font-medium hover:underline" style="color: var(--color-primary)">
			&larr; {t('security.back_settings')}
		</a>
	</div>

	<h1 class="mb-6 text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">{t('security.title')}</h1>

	{#if successMessage}
		<div
			class="mb-6 rounded-lg border px-4 py-3 text-sm"
			style="border-color: var(--color-primary); background-color: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-primary)"
		>
			{successMessage}
		</div>
	{/if}

	{#if error}
		<div
			class="mb-6 rounded-lg border px-4 py-3 text-sm"
			style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
		>
			{error}
		</div>
	{/if}

	<div class="space-y-6">
		<!-- 2FA Status Card -->
		<div class="rounded-xl border p-6" style="border-color: var(--color-border); background: var(--color-surface);">
			<div class="flex items-start justify-between">
				<div class="flex items-center gap-3">
					{#if totpEnabled}
						<ShieldCheck class="h-8 w-8" style="color: var(--color-success, #22c55e)" />
					{:else}
						<Shield class="h-8 w-8" style="color: var(--color-muted)" />
					{/if}
					<div>
						<h2 class="text-lg font-semibold" style="color: var(--color-foreground)">
							{t('security.two_factor')}
						</h2>
						<p class="mt-1 text-sm" style="color: var(--color-muted)">
							{totpEnabled
								? t('security.protected')
								: t('security.add_security')}
						</p>
					</div>
				</div>
				<span
					class="rounded-full px-2.5 py-0.5 text-xs font-medium"
					style="background-color: {totpEnabled ? 'color-mix(in srgb, var(--color-success, #22c55e) 15%, transparent)' : 'var(--color-border)'}; color: {totpEnabled ? 'var(--color-success, #22c55e)' : 'var(--color-muted)'}"
				>
					{totpEnabled ? t('security.enabled') : t('security.disabled')}
				</span>
			</div>

			{#if !totpEnabled && !setupPhase}
				<button
					onclick={startSetup}
					disabled={loading}
					class="mt-4 flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
					style="background-color: var(--color-primary)"
				>
					{#if loading}
						<Loader2 class="h-4 w-4 animate-spin" />
					{:else}
						<ShieldCheck class="h-4 w-4" />
					{/if}
					{t('security.enable_2fa')}
				</button>
			{/if}

			{#if totpEnabled && !showDisableForm}
				<button
					onclick={() => (showDisableForm = true)}
					class="mt-4 flex items-center gap-2 rounded-lg border px-4 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
					style="border-color: var(--color-danger); color: var(--color-danger)"
				>
					<ShieldOff class="h-4 w-4" />
					{t('security.disable_2fa')}
				</button>
			{/if}
		</div>

		<!-- Setup Phase: QR + Verification -->
		{#if setupPhase}
			<div class="rounded-xl border p-6" style="border-color: var(--color-border); background: var(--color-surface);">
				<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('security.setup_authenticator')}</h3>
				<p class="mt-1 text-sm" style="color: var(--color-muted)">
					{t('security.setup_instructions')}
				</p>

				{#if qrUri}
					<div class="mt-4 flex flex-col items-center gap-4">
						<div
							class="flex h-48 w-48 items-center justify-center rounded-lg border"
							style="border-color: var(--color-border); background-color: white"
						>
							<!-- QR code rendered as a placeholder — in production, use a QR library or render the URI in an <img> with a QR generator -->
							<QrCode class="h-24 w-24" style="color: var(--color-foreground)" />
						</div>
						<p class="text-xs" style="color: var(--color-muted)">
							{t('security.cant_scan')}
						</p>
						<code
							class="rounded border px-3 py-1.5 text-sm font-mono select-all"
							style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
						>
							{secret}
						</code>
					</div>
				{/if}

				<form onsubmit={verifyAndEnable} class="mt-6 space-y-4">
					<div>
						<label for="verification_code" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
							{t('security.verification_code')}
						</label>
						<div class="relative">
							<Key
								class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2"
								style="color: var(--color-muted)"
							/>
							<input
								id="verification_code"
								type="text"
								required
								maxlength="6"
								pattern="[0-9]{6}"
								inputmode="numeric"
								autocomplete="one-time-code"
								bind:value={verificationCode}
								placeholder="000000"
								class="w-full rounded-lg border py-2.5 pl-10 pr-4 text-sm outline-none transition-colors focus:border-[var(--color-primary)]"
								style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
							/>
						</div>
					</div>

					<div class="flex gap-3">
						<button
							type="submit"
							disabled={loading}
							class="flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
							style="background-color: var(--color-primary)"
						>
							{#if loading}
								<Loader2 class="h-4 w-4 animate-spin" />
							{/if}
							{t('security.verify_enable')}
						</button>
						<button
							type="button"
							onclick={cancelSetup}
							class="rounded-lg border px-4 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
							style="border-color: var(--color-border); color: var(--color-foreground)"
						>
							{t('common.cancel')}
						</button>
					</div>
				</form>
			</div>
		{/if}

		<!-- Disable 2FA Form -->
		{#if totpEnabled && showDisableForm}
			<div class="rounded-xl border p-6" style="border-color: var(--color-border); background: var(--color-surface);">
				<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('security.disable_2fa')}</h3>
				<p class="mt-1 text-sm" style="color: var(--color-muted)">
					{t('security.disable_instructions')}
				</p>

				<form onsubmit={handleDisable} class="mt-4 space-y-4">
					<div>
						<label for="disable_password" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
							{t('auth.password_label')}
						</label>
						<input
							id="disable_password"
							type="password"
							required
							bind:value={disablePassword}
							placeholder="••••••••"
							class="w-full rounded-lg border py-2.5 px-4 text-sm outline-none transition-colors focus:border-[var(--color-primary)]"
							style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
						/>
					</div>

					<div class="flex gap-3">
						<button
							type="submit"
							disabled={loading}
							class="flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-opacity hover:opacity-90 disabled:opacity-50"
							style="background-color: var(--color-danger); color: white"
						>
							{#if loading}
								<Loader2 class="h-4 w-4 animate-spin" />
							{/if}
							{t('security.disable_2fa')}
						</button>
						<button
							type="button"
							onclick={() => { showDisableForm = false; disablePassword = ''; error = ''; }}
							class="rounded-lg border px-4 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
							style="border-color: var(--color-border); color: var(--color-foreground)"
						>
							{t('common.cancel')}
						</button>
					</div>
				</form>
			</div>
		{/if}

		<!-- Backup Codes -->
		{#if backupCodes.length > 0}
			<div class="rounded-xl border p-6" style="border-color: var(--color-border); background: var(--color-surface);">
				<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('security.backup_codes')}</h3>
				<p class="mt-1 text-sm" style="color: var(--color-muted)">
					{t('security.backup_codes_msg')}
				</p>
				<div class="mt-4 grid grid-cols-2 gap-2">
					{#each backupCodes as code}
						<code
							class="rounded border px-3 py-2 text-center text-sm font-mono"
							style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
						>
							{code}
						</code>
					{/each}
				</div>
			</div>
		{/if}
	</div>
</main>
