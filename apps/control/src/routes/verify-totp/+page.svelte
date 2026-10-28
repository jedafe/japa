<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// Ref: japaconnect-engineering/sections/02-authentication-sessions.md (TOTP login challenge)
	// Completes the second factor of a 2FA login for the control panel. The
	// login page stashes the challenge token + email in the challenge store
	// and redirects here when /auth/login returns totp_required=true. This
	// page collects a 6-digit TOTP code (or a one-time backup code) and posts
	// to /api/v1/auth/totp/verify-challenge or /api/v1/auth/totp/backup. On
	// success the access token is set and the user is forwarded to the
	// control root. Without this page, any control-panel user who enables 2FA
	// (mandatory per §22) is permanently locked out of the control panel.
	import { t } from '$lib/stores/i18n.svelte';
	import { setToken } from '$lib/api';
	import { clearChallenge, getChallengeEmail, getChallengeToken } from '$lib/api/challenge';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { PUBLIC_JC_API_URL } from '$env/static/public';
	import { buildApiUrl } from '@japaconnect/api-url';

	let code = $state('');
	let backupCode = $state('');
	let loading = $state(false);
	let error = $state('');
	let showBackup = $state(false);

	const challengeToken = getChallengeToken();
	const email = getChallengeEmail();

	onMount(() => {
		if (!getChallengeToken()) {
			void goto('/login', { replaceState: true });
		}
	});

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
			const res = await fetch(buildApiUrl('/auth/totp/verify-challenge', PUBLIC_JC_API_URL), {
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
			setToken(body.data.access_token);
			clearChallenge();
			// Hard reload so the root layout re-mounts and onMount re-runs
			// with the fresh access token (matches the login page pattern).
			window.location.href = '/';
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
			const res = await fetch(buildApiUrl('/auth/totp/backup', PUBLIC_JC_API_URL), {
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
			setToken(body.data.access_token);
			clearChallenge();
			window.location.href = '/';
		} catch {
			error = t('auth.errors.generic', 'Something went wrong. Please try again.');
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>{t('auth.totp.title', 'Two-Factor Authentication')} — {siteName()} Control</title>
</svelte:head>

<div class="px-4">
	<div class="border-border-card bg-background-alt w-full max-w-sm rounded-lg border p-6">
		<h1 class="text-foreground mb-1 text-center text-xl font-bold tracking-tight">
			{t('auth.totp.title', 'Two-Factor Authentication')}
		</h1>
		<p class="text-muted-foreground mb-6 text-center text-sm">
			{t('auth.totp.subtitle', 'Enter the 6-digit code from your authenticator app')}
		</p>

		{#if email}
			<p class="text-muted-foreground mb-4 text-center text-sm">
				{t('auth.totp.email_hint', 'Signed in as')} <span class="text-foreground font-medium">{email}</span>
			</p>
		{/if}

		{#if error}
			<div class="border-destructive bg-destructive/10 text-destructive mb-4 rounded-md border px-3 py-2 text-sm" role="alert">
				{error}
			</div>
		{/if}

		{#if !showBackup}
			<form onsubmit={submitCode} class="space-y-4" novalidate>
				<div>
					<label for="totp-code" class="text-foreground mb-1 block text-sm font-medium">
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
						class="border-border-input bg-background text-foreground w-full rounded-md border px-3 py-2 text-center text-lg tracking-[0.5em] transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
					/>
					<p class="text-muted-foreground mt-1 text-xs">{t('auth.totp.code_help', 'Enter the current 6-digit code from your authenticator app.')}</p>
				</div>
				<button
					type="submit"
					disabled={loading || code.length !== 6}
					class="bg-foreground text-background w-full rounded-md px-4 py-2 text-sm font-medium transition-colors hover:opacity-90 disabled:opacity-50"
				>
					{loading ? t('login.signing_in', 'Signing in...') : t('auth.totp.verify', 'Verify')}
				</button>
			</form>

			<button
				type="button"
				onclick={() => { showBackup = true; error = ''; }}
				class="text-foreground mt-4 w-full text-center text-sm hover:underline"
			>
				{t('auth.totp.use_backup', 'Use a backup code instead')}
			</button>
		{:else}
			<form onsubmit={submitBackup} class="space-y-4" novalidate>
				<div>
					<label for="totp-backup" class="text-foreground mb-1 block text-sm font-medium">
						{t('auth.totp.backup_label', 'Backup code')}
					</label>
					<input
						id="totp-backup"
						autocomplete="off"
						placeholder="xxxx-xxxx"
						value={backupCode}
						oninput={(e) => (backupCode = e.currentTarget.value)}
						class="border-border-input bg-background text-foreground w-full rounded-md border px-3 py-2 transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
					/>
					<p class="text-muted-foreground mt-1 text-xs">{t('auth.totp.backup_help', 'Enter one of the single-use backup codes you saved when enabling 2FA.')}</p>
				</div>
				<button
					type="submit"
					disabled={loading || !backupCode.trim()}
					class="bg-foreground text-background w-full rounded-md px-4 py-2 text-sm font-medium transition-colors hover:opacity-90 disabled:opacity-50"
				>
					{loading ? t('login.signing_in', 'Signing in...') : t('auth.totp.verify_backup', 'Verify backup code')}
				</button>
			</form>

			<button
				type="button"
				onclick={() => { showBackup = false; error = ''; }}
				class="text-foreground mt-4 w-full text-center text-sm hover:underline"
			>
				{t('auth.totp.back_to_code', 'Back to verification code')}
			</button>
		{/if}

		<a href="/login" class="text-muted-foreground mt-6 block text-center text-sm hover:underline">
			{t('auth.totp.back_to_login', 'Back to login')}
		</a>
	</div>
</div>
