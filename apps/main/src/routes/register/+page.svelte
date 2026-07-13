<script lang="ts">
	import { goto } from '$app/navigation';
	import { Mail, Lock, User, Loader2 } from 'lucide-svelte';
	import { api } from '$lib/api';
	import { trackEvent, AnalyticsEvents } from '$lib/analytics';
	import { t } from '$lib/stores/i18n.svelte';
	import { Button, Input } from '$lib/components/ui';

	let username = $state('');
	let email = $state('');
	let password = $state('');
	let error = $state('');
	let loading = $state(false);
	let oauthLoading = $state('');
	let oauthError = $state('');

	// Validation state
	let usernameError = $state('');
	let emailError = $state('');
	let passwordError = $state('');

	function validateUsername(value: string): boolean {
		if (!value.trim()) {
			usernameError = 'Username is required';
			return false;
		}
		if (value.length < 3) {
			usernameError = 'Username must be at least 3 characters';
			return false;
		}
		if (value.length > 30) {
			usernameError = 'Username must be 30 characters or less';
			return false;
		}
		const usernameRegex = /^[a-zA-Z0-9_]+$/;
		if (!usernameRegex.test(value)) {
			usernameError = 'Username can only contain letters, numbers, and underscores';
			return false;
		}
		usernameError = '';
		return true;
	}

	function validateEmail(value: string): boolean {
		if (!value.trim()) {
			emailError = 'Email is required';
			return false;
		}
		const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
		if (!emailRegex.test(value)) {
			emailError = 'Please enter a valid email address';
			return false;
		}
		emailError = '';
		return true;
	}

	function validatePassword(value: string): boolean {
		if (!value) {
			passwordError = 'Password is required';
			return false;
		}
		if (value.length < 8) {
			passwordError = 'Password must be at least 8 characters';
			return false;
		}
		// Check for at least one uppercase, one lowercase, and one number
		const hasUppercase = /[A-Z]/.test(value);
		const hasLowercase = /[a-z]/.test(value);
		const hasNumber = /[0-9]/.test(value);
		if (!hasUppercase || !hasLowercase || !hasNumber) {
			passwordError = 'Password must contain at least one uppercase letter, one lowercase letter, and one number';
			return false;
		}
		passwordError = '';
		return true;
	}

	async function handleSubmit(e: Event) {
		e.preventDefault();
		error = '';

		// Validate before submitting
		const isUsernameValid = validateUsername(username);
		const isEmailValid = validateEmail(email);
		const isPasswordValid = validatePassword(password);

		if (!isUsernameValid || !isEmailValid || !isPasswordValid) {
			return;
		}

		loading = true;

		try {
			await api.register({ username, email, password });
			trackEvent(AnalyticsEvents.REGISTER, { username });
			goto('/login');
		} catch (err) {
			error = err instanceof Error ? err.message : 'Network error. Please try again.';
		} finally {
			loading = false;
		}
	}

	async function initiateOAuth(provider: string) {
		oauthLoading = provider;
		oauthError = '';
		try {
			const res = await fetch(`/api/v1/auth/oauth/${provider}/initiate`, { method: 'POST' });
			const body = await res.json();
			if (!res.ok || !body.data?.auth_url) {
				oauthError = body.error?.message || `${provider} sign-up unavailable. Please try again.`;
				return;
			}
			window.location.href = body.data.auth_url;
		} catch {
			oauthError = 'Could not connect to authentication service. Please try again.';
		} finally {
			oauthLoading = '';
		}
	}
</script>

<svelte:head>
	<title>Register | JapaConnect</title>
	<meta name="description" content="Create your JapaConnect account." />
</svelte:head>

<main class="mx-auto max-w-md px-6 py-16">
	<div class="rounded-xl border p-8" style="border-color: var(--color-border); background: var(--color-surface);">
		<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">{t('auth.create_account')}</h1>
		<p class="mt-2 text-sm" style="color: var(--color-muted)">
			{t('auth.join_community')}
		</p>

		{#if error}
			<div
				class="mt-4 rounded-lg border px-4 py-3 text-sm"
				style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
			>
				{error}
			</div>
		{/if}

		{#if oauthError}
			<div
				class="mt-4 rounded-lg border px-4 py-3 text-sm"
				style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
			>
				{oauthError}
			</div>
		{/if}

		<!-- OAuth Buttons -->
		<div class="flex flex-col gap-3">
			<Button
				variant="secondary"
				type="button"
				onclick={() => initiateOAuth('google')}
				disabled={!!oauthLoading}
				loading={oauthLoading === 'google'}
				class="w-full"
			>
				{#if oauthLoading !== 'google'}
					<svg class="h-4 w-4" viewBox="0 0 24 24">
						<path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 0 1-2.2 3.32v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.1z" fill="#4285F4"/>
						<path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" fill="#34A853"/>
						<path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" fill="#FBBC05"/>
						<path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" fill="#EA4335"/>
					</svg>
				{/if}
				{t('auth.sign_up_google')}
			</Button>
			<Button
				type="button"
				onclick={() => initiateOAuth('github')}
				disabled={!!oauthLoading}
				loading={oauthLoading === 'github'}
				class="w-full bg-[#24292f] text-white hover:opacity-90"
			>
				{#if oauthLoading !== 'github'}
					<svg class="h-4 w-4" viewBox="0 0 24 24" fill="currentColor">
						<path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0 0 24 12c0-6.63-5.37-12-12-12z"/>
					</svg>
				{/if}
				{t('auth.sign_up_github')}
			</Button>
		</div>

		<!-- OAuth Divider -->
		<div class="relative my-6">
			<div class="absolute inset-0 flex items-center">
				<span class="w-full border-t" style="border-color: var(--color-border)"></span>
			</div>
			<div class="relative flex justify-center text-xs uppercase">
				<span class="px-2" style="color: var(--color-muted); background: var(--color-surface)">
					{t('auth.or_sign_up_with')}
				</span>
			</div>
		</div>

		<form onsubmit={handleSubmit} class="mt-6 space-y-4">
			<div>
				<label for="username" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
					{t('auth.username_label')}
				</label>
				<div class="relative">
					<User
						class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2"
						style="color: var(--color-muted)"
					/>
					<Input
						id="username"
						type="text"
						required
						minlength="3"
						maxlength="30"
						pattern="[a-zA-Z0-9_]+"
						bind:value={username}
						placeholder="johndoe"
						class="pl-10"
						aria-invalid={!!usernameError}
						aria-describedby={usernameError ? 'username-error' : undefined}
					/>
				</div>
				{#if usernameError}
					<p id="username-error" class="mt-1 text-xs" style="color: var(--color-danger)">{usernameError}</p>
				{/if}
			</div>

			<div>
				<label for="email" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
					{t('auth.email_label')}
				</label>
				<div class="relative">
					<Mail
						class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2"
						style="color: var(--color-muted)"
					/>
					<Input
						id="email"
						type="email"
						required
						bind:value={email}
						placeholder="you@example.com"
						class="pl-10"
						aria-invalid={!!emailError}
						aria-describedby={emailError ? 'email-error' : undefined}
					/>
				</div>
				{#if emailError}
					<p id="email-error" class="mt-1 text-xs" style="color: var(--color-danger)">{emailError}</p>
				{/if}
			</div>

			<div>
				<label for="password" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
					{t('auth.password_label')}
				</label>
				<div class="relative">
					<Lock
						class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2"
						style="color: var(--color-muted)"
					/>
					<Input
						id="password"
						type="password"
						required
						minlength="8"
						bind:value={password}
						placeholder="••••••••"
						class="pl-10"
						aria-invalid={!!passwordError}
						aria-describedby={passwordError ? 'password-error' : undefined}
					/>
				</div>
				{#if passwordError}
					<p id="password-error" class="mt-1 text-xs" style="color: var(--color-danger)">{passwordError}</p>
				{/if}
				<p class="mt-1 text-xs" style="color: var(--color-muted)">
					Must be at least 8 characters with uppercase, lowercase, and number
				</p>
			</div>

			<Button
				type="submit"
				disabled={loading}
				loading={loading}
				class="w-full"
			>
				{#if !loading}
					{t('auth.create_account')}
				{/if}
			</Button>
		</form>

		<p class="mt-6 text-center text-sm" style="color: var(--color-muted)">
			{t('auth.has_account')}
			<a href="/login" class="font-medium hover:underline" style="color: var(--color-primary)">
				{t('auth.sign_in')}
			</a>
		</p>
	</div>
</main>
