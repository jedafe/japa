<script lang="ts">
	import { Mail, Lock, Loader2 } from 'lucide-svelte';
	import { enhance } from '$app/forms';
	import { trackEvent, AnalyticsEvents } from '$lib/analytics';
	import { t } from '$lib/stores/i18n.svelte';

	let { form } = $props();

	let email = $state('');
	let password = $state('');
	let loading = $state(false);
	let oauthLoading = $state('');
	let oauthError = $state('');

	function handleSubmit() {
		loading = true;
		trackEvent(AnalyticsEvents.LOGIN, { email });
		return async () => {
			loading = false;
		};
	}

	async function initiateOAuth(provider: string) {
		oauthLoading = provider;
		oauthError = '';
		try {
			const res = await fetch(`/api/v1/auth/oauth/${provider}/initiate`, { method: 'POST' });
			const body = await res.json();
			if (!res.ok || !body.data?.auth_url) {
				oauthError = body.error?.message || `${provider} sign-in unavailable. Please try again.`;
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
	<title>Login | JapaConnect</title>
	<meta name="description" content="Sign in to your JapaConnect account." />
</svelte:head>

<main class="mx-auto max-w-md px-6 py-16">
	<div class="rounded-xl border p-8" style="border-color: var(--color-border); background: var(--color-surface);">
		<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">{t('auth.sign_in')}</h1>
		<p class="mt-2 text-sm" style="color: var(--color-muted)">
			{t('auth.welcome_back')}
		</p>

		{#if form?.error}
			<div
				class="mt-4 rounded-lg border px-4 py-3 text-sm"
				style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
			>
				{form.error}
			</div>
		{/if}

		<form method="POST" use:enhance={handleSubmit} class="mt-6 space-y-4">
			<div>
				<label for="email" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
					{t('auth.email_label')}
				</label>
				<div class="relative">
					<Mail
						class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2"
						style="color: var(--color-muted)"
					/>
					<input
						id="email"
						type="email"
						required
						bind:value={email}
						placeholder="you@example.com"
						class="w-full rounded-lg border py-2.5 pl-10 pr-4 text-sm outline-none transition-colors focus:border-[var(--color-primary)]"
						style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
					/>
				</div>
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
					<input
						id="password"
						type="password"
						required
						bind:value={password}
						placeholder="••••••••"
						class="w-full rounded-lg border py-2.5 pl-10 pr-4 text-sm outline-none transition-colors focus:border-[var(--color-primary)]"
						style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
					/>
				</div>
				<div class="mt-1.5 text-right">
					<a
						href="/forgot-password"
						class="text-xs font-medium hover:underline"
						style="color: var(--color-primary)"
					>
						{t('auth.forgot_password')}
					</a>
				</div>
			</div>

			<button
				type="submit"
				disabled={loading}
				class="flex w-full items-center justify-center gap-2 rounded-lg px-4 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
				style="background-color: var(--color-primary)"
			>
				{#if loading}
					<Loader2 class="h-4 w-4 animate-spin" />
				{:else}
					{t('auth.sign_in')}
				{/if}
			</button>
		</form>

		<!-- OAuth Divider -->
		<div class="relative my-6">
			<div class="absolute inset-0 flex items-center">
				<span class="w-full border-t" style="border-color: var(--color-border)"></span>
			</div>
			<div class="relative flex justify-center text-xs uppercase">
				<span class="px-2" style="color: var(--color-muted); background: var(--color-surface)">
					{t('auth.or_continue_with')}
				</span>
			</div>
		</div>

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
			<button
				type="button"
				onclick={() => initiateOAuth('google')}
				disabled={!!oauthLoading}
				class="flex w-full items-center justify-center gap-3 rounded-lg border px-4 py-2.5 text-sm font-medium transition-opacity hover:opacity-80 disabled:opacity-50"
				style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
			>
				{#if oauthLoading === 'google'}
					<Loader2 class="h-4 w-4 animate-spin" />
				{:else}
					<svg class="h-4 w-4" viewBox="0 0 24 24">
						<path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 0 1-2.2 3.32v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.1z" fill="#4285F4"/>
						<path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" fill="#34A853"/>
						<path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" fill="#FBBC05"/>
						<path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" fill="#EA4335"/>
					</svg>
				{/if}
				{t('auth.continue_google')}
			</button>
			<button
				type="button"
				onclick={() => initiateOAuth('github')}
				disabled={!!oauthLoading}
				class="flex w-full items-center justify-center gap-3 rounded-lg px-4 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
				style="background-color: #24292f"
			>
				{#if oauthLoading === 'github'}
					<Loader2 class="h-4 w-4 animate-spin" />
				{:else}
					<svg class="h-4 w-4" viewBox="0 0 24 24" fill="currentColor">
						<path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0 0 24 12c0-6.63-5.37-12-12-12z"/>
					</svg>
				{/if}
				{t('auth.continue_github')}
			</button>
		</div>

		<p class="mt-6 text-center text-sm" style="color: var(--color-muted)">
			{t('auth.no_account')}
			<a href="/register" class="font-medium hover:underline" style="color: var(--color-primary)">
				{t('auth.create_one')}
			</a>
		</p>
	</div>
</main>
