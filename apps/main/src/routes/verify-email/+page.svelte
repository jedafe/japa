<script lang="ts">
	import { page } from '$app/state';
	import { Mail, Check, X, Loader2, RefreshCw } from 'lucide-svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { onMount } from 'svelte';

	let status = $state<'loading' | 'success' | 'error'>('loading');
	let errorMessage = $state('');
	let resendEmail = $state('');
	let resendLoading = $state(false);
	let resendSent = $state(false);
	let resendError = $state('');

	function getToken(): string {
		return page.url.searchParams.get('token') ?? '';
	}

	onMount(async () => {
		const token = getToken();
		if (!token) {
			status = 'error';
			errorMessage = 'Invalid or missing verification link.';
			return;
		}

		try {
			await api.verifyEmail(token);
			status = 'success';
		} catch (err) {
			status = 'error';
			errorMessage = err instanceof Error ? err.message : 'Network error. Please try again.';
		}
	});

	async function handleResend(e: Event) {
		e.preventDefault();
		resendError = '';
		resendLoading = true;

		try {
			await api.resendVerification(resendEmail);
			resendSent = true;
		} catch (err) {
			resendError = err instanceof Error ? err.message : 'Network error. Please try again.';
		} finally {
			resendLoading = false;
		}
	}
</script>

<svelte:head>
	<title>Verify Email | JapaConnect</title>
	<meta name="description" content="Verify your email address for JapaConnect." />
</svelte:head>

<main class="mx-auto max-w-md px-6 py-16">
	<div class="rounded-xl border p-8" style="border-color: var(--color-border); background: var(--color-surface);">
		{#if status === 'loading'}
			<div class="text-center">
				<Loader2 class="mx-auto h-10 w-10 animate-spin" style="color: var(--color-primary)" />
				<h1 class="mt-4 text-2xl font-bold" style="color: var(--color-foreground)">{t('auth.verifying_email')}</h1>
				<p class="mt-2 text-sm" style="color: var(--color-muted)">
					{t('auth.verifying_msg')}
				</p>
			</div>

		{:else if status === 'success'}
			<div class="text-center">
				<div
					class="mx-auto flex h-12 w-12 items-center justify-center rounded-full"
					style="background-color: color-mix(in srgb, var(--color-success, #22c55e) 15%, transparent)"
				>
					<Check class="h-6 w-6" style="color: var(--color-success, #22c55e)" />
				</div>
				<h1 class="mt-4 text-2xl font-bold" style="color: var(--color-foreground)">{t('auth.email_verified')}</h1>
				<p class="mt-2 text-sm" style="color: var(--color-muted)">
					{t('auth.email_verified_msg')}
				</p>
				<a
					href="/login"
					class="mt-6 inline-flex items-center justify-center rounded-lg px-4 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90"
					style="background-color: var(--color-primary)"
				>
					{t('auth.sign_in')}
				</a>
			</div>

		{:else}
			<div class="text-center">
				<div
					class="mx-auto flex h-12 w-12 items-center justify-center rounded-full"
					style="background-color: color-mix(in srgb, var(--color-danger) 10%, transparent)"
				>
					<X class="h-6 w-6" style="color: var(--color-danger)" />
				</div>
				<h1 class="mt-4 text-2xl font-bold" style="color: var(--color-foreground)">{t('auth.verification_failed')}</h1>
				<p class="mt-2 text-sm" style="color: var(--color-muted)">
					{errorMessage || 'Verification failed or link expired.'}
				</p>
			</div>

			{#if !resendSent}
				<form onsubmit={handleResend} class="mt-6 space-y-4">
					<div>
						<label for="resend_email" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
							{t('auth.enter_email_resend')}
						</label>
						<div class="relative">
							<Mail
								class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2"
								style="color: var(--color-muted)"
							/>
							<input
								id="resend_email"
								type="email"
								required
								bind:value={resendEmail}
								placeholder="you@example.com"
								class="w-full rounded-lg border py-2.5 pl-10 pr-4 text-sm outline-none transition-colors focus:border-[var(--color-primary)]"
								style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
							/>
						</div>
					</div>

					{#if resendError}
						<div
							class="rounded-lg border px-4 py-3 text-sm"
							style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
						>
							{resendError}
						</div>
					{/if}

					<button
						type="submit"
						disabled={resendLoading}
						class="flex w-full items-center justify-center gap-2 rounded-lg px-4 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
						style="background-color: var(--color-primary)"
					>
						{#if resendLoading}
							<Loader2 class="h-4 w-4 animate-spin" />
						{:else}
							<RefreshCw class="h-4 w-4" />
							{t('auth.resend_verification')}
						{/if}
					</button>
				</form>
			{:else}
				<div
					class="mt-6 rounded-lg border px-4 py-3 text-center text-sm"
					style="border-color: var(--color-primary); background-color: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-primary)"
				>
					{t('auth.verification_sent')}
				</div>
			{/if}

			<p class="mt-6 text-center text-sm" style="color: var(--color-muted)">
				<a href="/login" class="font-medium hover:underline" style="color: var(--color-primary)">
					{t('auth.back_to_sign_in')}
				</a>
			</p>
		{/if}
	</div>
</main>
