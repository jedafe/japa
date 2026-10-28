<script lang="ts">
	import { t } from '$lib/i18n/store.svelte';
	import { validateForgotPassword } from '$lib/auth/validation';
	import { mapAuthError } from '$lib/auth/errors';
	import HoneypotField from '$lib/components/auth/HoneypotField.svelte';

	let email = $state('');
	let loading = $state(false);
	let errors = $state<Record<string, string>>({});
	let formError = $state<string | null>(null);
	let sent = $state(false);
	let honeypot = $state('');

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errors = {};
		formError = null;
		const v = validateForgotPassword(email);
		if (!v.valid) {
			errors = v.errors;
			return;
		}

		loading = true;
		try {
			const res = await fetch('/api/auth/forgot-password', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({ email, website: honeypot })
			});
			const data = await res.json().catch(() => null);
			if (!res.ok) {
				// Real infrastructure/rate-limit errors are surfaced here.
				const mapped = mapAuthError(data?.error?.code, res.status);
					formError = t(mapped.messageKey);
				return;
			}
			// Always show confirmation — don't reveal whether the email exists.
			sent = true;
		} catch {
			formError = t('auth.errors.generic');
		} finally {
			loading = false;
		}
	}
</script>

<form onsubmit={submit} class="flex flex-col gap-4" novalidate>
	<HoneypotField bind:value={honeypot} />
	<h1 class="text-2xl font-semibold text-[var(--color-text)]">{t('auth.forgot_password.title')}</h1>
	<p class="text-sm text-[var(--color-muted)]">{t('auth.forgot_password.subtitle')}</p>

	{#if formError}
		<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
			{formError}
		</p>
	{/if}

	{#if sent}
		<p class="rounded-[var(--radius)] bg-green-500/10 px-3 py-2 text-sm text-green-600" role="status">
			{t('auth.forgot_password.sent_confirmation')}
		</p>
		<a href="/login" class="text-sm text-[var(--color-primary)] hover:underline">
			{t('auth.forgot_password.back_to_login')}
		</a>
	{:else}
		<div class="flex flex-col gap-1.5">
			<label for="forgot-email" class="text-sm font-medium text-[var(--color-text)]">{t('auth.fields.email')}</label>
			<input
				id="forgot-email"
				type="email"
				autocomplete="email"
				bind:value={email}
				aria-invalid={errors.email ? 'true' : undefined}
				aria-describedby={errors.email ? 'forgot-email-error' : undefined}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2.5 text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
			/>
			{#if errors.email}<p id="forgot-email-error" class="text-sm text-red-500">{t(errors.email)}</p>{/if}
		</div>

		<button
			type="submit"
			disabled={loading}
			class="w-full rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2.5 text-sm font-medium text-white transition hover:opacity-90 disabled:opacity-50"
		>
			{loading ? '...' : t('auth.forgot_password.submit')}
		</button>

		<a href="/login" class="text-center text-sm text-[var(--color-primary)] hover:underline">
			{t('auth.forgot_password.back_to_login')}
		</a>
	{/if}
</form>
