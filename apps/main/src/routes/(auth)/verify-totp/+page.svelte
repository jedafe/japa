<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	// Ref: japaconnect-engineering/sections/02-authentication-sessions.md (TOTP login challenge)
	// Collects a 6-digit TOTP code (or a one-time backup code) and posts to the
	// /api/auth/totp/verify or /api/auth/totp/backup proxy. On success the
	// jc_token cookie is set and the user is forwarded to the redirect target.
	import { t } from '$lib/i18n/store.svelte';
	import { invalidateAll } from '$app/navigation';
	import { goto } from '$app/navigation';
	import { navigatePostLogin, safeRelativePath } from '$lib/auth/redirect';
	import { ShieldCheck } from 'lucide-svelte';

	let { data } = $props();

	let code = $state('');
	let backupCode = $state('');
	let loading = $state(false);
	let errorKey = $state('');
	let showBackup = $state(false);
	// Exponential backoff on backup-code failures to throttle guessing
	// (Bug Hunter §5.2: 2FA recovery path not rate-limited → brute force).
	// Each invalid_backup failure doubles the cooldown up to a 30s cap.
	let backupAttempts = $state(0);
	let backupCooldown = $state(0);

	function sanitizeCode(value: string): string {
		return value.replace(/\D/g, '').slice(0, 6);
	}

	async function submitCode(e: SubmitEvent) {
		e.preventDefault();
		errorKey = '';
		if (code.length !== 6) {
			errorKey = 'auth.totp.code_required';
			return;
		}
		loading = true;
		try {
			const res = await fetch('/api/auth/totp/verify', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({
					challenge_token: data.challengeToken,
					code,
					redirect: data.redirectTarget
				})
			});
			const payload = await res.json();
			if (!res.ok) {
				errorKey = mapTotpError(payload?.error?.code);
				return;
			}
			await invalidateAll();
			await navigatePostLogin(payload.redirect ?? safeRelativePath(data.redirectTarget));
		} catch {
			errorKey = 'auth.errors.generic';
		} finally {
			loading = false;
		}
	}

	async function submitBackup(e: SubmitEvent) {
		e.preventDefault();
		errorKey = '';
		if (!backupCode.trim()) {
			errorKey = 'auth.totp.backup_required';
			return;
		}
		// Client-side cooldown: if previous attempts failed, wait before
		// allowing another submission. This complements the server-side
		// rate limit (jc:ratelimit:totp_backup:{user_id}) and gives
		// immediate UX feedback rather than waiting for a 429.
		if (backupCooldown > 0) {
			errorKey = 'auth.totp.backup_cooldown';
			return;
		}
		loading = true;
		try {
			const res = await fetch('/api/auth/totp/backup', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({
					challenge_token: data.challengeToken,
					backup_code: backupCode.trim(),
					redirect: data.redirectTarget
				})
			});
			const payload = await res.json();
			if (!res.ok) {
				errorKey = mapTotpError(payload?.error?.code);
				// On invalid_backup or rate_limit_exceeded, apply exponential
				// backoff so the user can't immediately retry.
				if (payload?.error?.code === 'invalid_backup' || payload?.error?.code === 'rate_limit_exceeded') {
					backupAttempts += 1;
					backupCooldown = Math.min(2 ** backupAttempts, 30);
					startBackupCooldownTimer();
				}
				return;
			}
			await invalidateAll();
			await navigatePostLogin(payload.redirect ?? safeRelativePath(data.redirectTarget));
		} catch {
			errorKey = 'auth.errors.generic';
		} finally {
			loading = false;
		}
	}

	let backupCooldownTimer: ReturnType<typeof setInterval> | null = null;
	function startBackupCooldownTimer() {
		if (backupCooldownTimer) clearInterval(backupCooldownTimer);
		backupCooldownTimer = setInterval(() => {
			backupCooldown -= 1;
			if (backupCooldown <= 0) {
				backupCooldown = 0;
				if (backupCooldownTimer) {
					clearInterval(backupCooldownTimer);
					backupCooldownTimer = null;
				}
			}
		}, 1000);
	}

	function mapTotpError(code: string | undefined): string {
		switch (code) {
			case 'invalid_totp':
				return 'auth.totp.invalid_code';
			case 'invalid_challenge':
				return 'auth.totp.invalid_challenge';
			case 'invalid_backup':
				return 'auth.totp.invalid_backup';
			case 'no_backup_codes':
				return 'auth.totp.no_backup_codes';
			case 'totp_error':
				return 'auth.totp.not_setup';
			case 'missing_fields':
			case 'missing_code':
				return 'auth.totp.code_required';
			default:
				return 'auth.errors.generic';
		}
	}
</script>

<div class="flex flex-col gap-4">
	<div class="flex items-center gap-3">
		<div class="flex h-10 w-10 items-center justify-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
			<ShieldCheck class="h-5 w-5" />
		</div>
		<div>
			<h1 class="text-2xl font-semibold text-[var(--color-text)]">{t('auth.totp.title')}</h1>
			<p class="text-sm text-[var(--color-muted)]">{t('auth.totp.subtitle')}</p>
		</div>
	</div>

	{#if data.email}
		<p class="text-sm text-[var(--color-muted)]">
			{t('auth.totp.email_hint')} <span class="font-medium text-[var(--color-text)]">{data.email}</span>
		</p>
	{/if}

	{#if errorKey}
		<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
			{t(errorKey)}
		</p>
	{/if}

	{#if !showBackup}
		<form onsubmit={submitCode} class="flex flex-col gap-4" novalidate>
			<div class="flex flex-col gap-1.5">
				<label for="totp-code" class="text-sm font-medium text-[var(--color-text)]">
					{t('auth.totp.code_label')}
				</label>
				<input
					id="totp-code"
					inputmode="numeric"
					autocomplete="one-time-code"
					autocapitalize="none"
					placeholder="000000"
					value={code}
					oninput={(e) => (code = sanitizeCode(e.currentTarget.value))}
					aria-invalid={errorKey === 'auth.totp.invalid_code' || errorKey === 'auth.totp.code_required' ? 'true' : undefined}
					aria-describedby="totp-code-hint"
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2.5 text-center text-lg tracking-[0.5em] text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
				/>
			</div>

			<button
				type="submit"
				disabled={loading}
				class="w-full rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2.5 text-sm font-medium text-white transition hover:opacity-90 disabled:opacity-50"
			>
				{loading ? '...' : t('auth.totp.verify')}
			</button>
		</form>

		<button
			type="button"
			onclick={() => { showBackup = true; errorKey = ''; }}
			class="text-center text-sm text-[var(--color-primary)] hover:underline"
		>
			{t('auth.totp.use_backup')}
		</button>
	{:else}
		<form onsubmit={submitBackup} class="flex flex-col gap-4" novalidate>
			<div class="flex flex-col gap-1.5">
				<label for="totp-backup" class="text-sm font-medium text-[var(--color-text)]">
					{t('auth.totp.backup_label')}
				</label>
				<input
					id="totp-backup"
					autocomplete="off"
					placeholder="xxxx-xxxx-xxxx"
					value={backupCode}
					oninput={(e) => (backupCode = e.currentTarget.value)}
					aria-invalid={errorKey === 'auth.totp.invalid_backup' || errorKey === 'auth.totp.backup_required' ? 'true' : undefined}
					aria-describedby="totp-backup-hint"
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2.5 text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
				/>
				<HintIcon id="totp-backup-hint" text={t('auth.totp.backup_help')kupCooldown > 0}
				class="w-full rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2.5 text-sm font-medium text-white transition hover:opacity-90 disabled:opacity-50"
			>
				{#if backupCooldown > 0}
					{t('auth.totp.retry_in', 'Retry in')} {backupCooldown}s
				{:else}
					{loading ? '...' : t('auth.totp.verify_backup')}
				{/if}
			</button>
		</form>

		<button
			type="button"
			onclick={() => { showBackup = false; errorKey = ''; }}
			class="text-center text-sm text-[var(--color-primary)] hover:underline"
		>
			{t('auth.totp.back_to_code')}
		</button>
	{/if}

	<a href="/login" class="text-center text-sm text-[var(--color-muted)] hover:underline">
		{t('auth.forgot_password.back_to_login')}
	</a>
</div>
