<script lang="ts">
	// Ref: japaconnect-engineering/sections/02-authentication-sessions.md (TOTP login challenge)
	// Completes the second factor of a 2FA login for the admin panel. The login
	// page stashes the challenge token + email in the challenge store and
	// redirects here when /auth/login returns totp_required=true. This page
	// collects a 6-digit TOTP code (or a one-time backup code) and posts to
	// /api/v1/auth/totp/verify-challenge or /api/v1/auth/totp/backup. On
	// success the access token is set and the user is forwarded to the admin
	// root. Without this page, any admin who enables 2FA (mandatory per §22)
	// is permanently locked out of the admin panel.
	import { t } from '$lib/stores/i18n.svelte';
	import { goto, invalidateAll } from '$app/navigation';
	import { setAccessToken } from '$lib/api/token';
	import { clearChallenge, getChallengeEmail, getChallengeToken } from '$lib/api/challenge';
	import { restoreSession } from '$lib/stores/session.svelte';
	import { getPermissions } from '$lib/stores/permissions.svelte';

	let code = $state('');
	let backupCode = $state('');
	let loading = $state(false);
	let error = $state('');
	let showBackup = $state(false);

	const challengeToken = getChallengeToken();
	const email = getChallengeEmail();

	function sanitizeCode(value: string): string {
		return value.replace(/\D/g, '').slice(0, 6);
	}

	function mapTotpError(code: string | undefined): string {
		switch (code) {
			case 'invalid_totp':
				return t('auth.totp.invalid_code', 'Invalid verification code');
			case 'invalid_challenge':
				return t('auth.totp.invalid_challenge', 'Invalid or expired challenge token');
			case 'invalid_backup':
				return t('auth.totp.invalid_backup', 'Invalid backup code');
			case 'no_backup_codes':
				return t('auth.totp.no_backup_codes', 'No backup codes found');
			case 'totp_error':
				return t('auth.totp.not_setup', '2FA not set up');
			case 'missing_fields':
			case 'missing_code':
				return t('auth.totp.code_required', 'Verification code is required');
			default:
				return t('auth.errors.generic', 'Something went wrong. Please try again.');
		}
	}

	async function submitCode(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		if (!challengeToken) {
			error = t('auth.totp.invalid_challenge', 'Invalid or expired challenge token');
			return;
		}
		if (code.length !== 6) {
			error = t('auth.totp.code_required', 'Verification code is required');
			return;
		}
		loading = true;
		try {
			const res = await fetch('/api/v1/auth/totp/verify-challenge', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				credentials: 'include',
				body: JSON.stringify({ challenge_token: challengeToken, code })
			});
			const body = await res.json().catch(() => ({}));
			if (!res.ok || !body.data?.access_token) {
				error = mapTotpError(body?.error?.code);
				return;
			}
			setAccessToken(body.data.access_token);
			clearChallenge();
			await restoreSession();
			void getPermissions();
			await goto('/', { invalidateAll: true });
		} catch {
			error = t('auth.errors.generic', 'Something went wrong. Please try again.');
		} finally {
			loading = false;
		}
	}

	async function submitBackup(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		if (!challengeToken) {
			error = t('auth.totp.invalid_challenge', 'Invalid or expired challenge token');
			return;
		}
		if (!backupCode.trim()) {
			error = t('auth.totp.backup_required', 'Backup code is required');
			return;
		}
		loading = true;
		try {
			const res = await fetch('/api/v1/auth/totp/backup', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				credentials: 'include',
				body: JSON.stringify({ challenge_token: challengeToken, backup_code: backupCode.trim() })
			});
			const body = await res.json().catch(() => ({}));
			if (!res.ok || !body.data?.access_token) {
				error = mapTotpError(body?.error?.code);
				return;
			}
			setAccessToken(body.data.access_token);
			clearChallenge();
			await restoreSession();
			void getPermissions();
			await goto('/', { invalidateAll: true });
		} catch {
			error = t('auth.errors.generic', 'Something went wrong. Please try again.');
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>{t('auth.totp.title', 'Two-Factor Authentication')} — JapaConnect Admin</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center px-4">
	<div class="w-full max-w-sm rounded-lg border p-6" style="border-color: var(--color-border); background: var(--color-surface)">
		<h1 class="mb-1 text-center text-xl font-bold tracking-tight" style="color: var(--color-foreground)">
			{t('auth.totp.title', 'Two-Factor Authentication')}
		</h1>
		<p class="mb-6 text-center text-sm" style="color: var(--color-muted)">
			{t('auth.totp.subtitle', 'Enter the 6-digit code from your authenticator app')}
		</p>

		{#if email}
			<p class="mb-4 text-center text-sm" style="color: var(--color-muted)">
				{t('auth.totp.email_hint', 'Signed in as')} <span class="font-medium" style="color: var(--color-foreground)">{email}</span>
			</p>
		{/if}

		{#if error}
			<div class="mb-4 rounded-md border px-3 py-2 text-sm" role="alert" style="border-color: var(--color-danger); background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
				{error}
			</div>
		{/if}

		{#if !showBackup}
			<form onsubmit={submitCode} class="space-y-4" novalidate>
				<div>
					<label for="totp-code" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
						{t('auth.totp.code_label', 'Verification code')}
					</label>
					<input
						id="totp-code"
						inputmode="numeric"
						autocomplete="one-time-code"
						autocapitalize="none"
						placeholder="000000"
						value={code}
						oninput={(e) => (code = sanitizeCode(e.currentTarget.value))}
						class="w-full rounded-md border px-3 py-2 text-center text-lg tracking-[0.5em]"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					/>
					<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('auth.totp.code_help', 'Enter the current 6-digit code from your authenticator app.')}</p>
				</div>
				<button
					type="submit"
					disabled={loading || code.length !== 6}
					class="w-full rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
					style="background: var(--color-primary)"
				>
					{loading ? t('login.signing_in', 'Signing in...') : t('auth.totp.verify', 'Verify')}
				</button>
			</form>

			<button
				type="button"
				onclick={() => { showBackup = true; error = ''; }}
				class="mt-4 w-full text-center text-sm hover:underline"
				style="color: var(--color-primary)"
			>
				{t('auth.totp.use_backup', 'Use a backup code instead')}
			</button>
		{:else}
			<form onsubmit={submitBackup} class="space-y-4" novalidate>
				<div>
					<label for="totp-backup" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
						{t('auth.totp.backup_label', 'Backup code')}
					</label>
					<input
						id="totp-backup"
						autocomplete="off"
						placeholder="xxxx-xxxx"
						value={backupCode}
						oninput={(e) => (backupCode = e.currentTarget.value)}
						class="w-full rounded-md border px-3 py-2"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					/>
					<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('auth.totp.backup_help', 'Enter one of the single-use backup codes you saved when enabling 2FA.')}</p>
				</div>
				<button
					type="submit"
					disabled={loading || !backupCode.trim()}
					class="w-full rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
					style="background: var(--color-primary)"
				>
					{loading ? t('login.signing_in', 'Signing in...') : t('auth.totp.verify_backup', 'Verify backup code')}
				</button>
			</form>

			<button
				type="button"
				onclick={() => { showBackup = false; error = ''; }}
				class="mt-4 w-full text-center text-sm hover:underline"
				style="color: var(--color-primary)"
			>
				{t('auth.totp.back_to_code', 'Back to verification code')}
			</button>
		{/if}

		<a href="/login" class="mt-6 block text-center text-sm hover:underline" style="color: var(--color-muted)">
			{t('auth.totp.back_to_login', 'Back to login')}
		</a>
	</div>
</div>
