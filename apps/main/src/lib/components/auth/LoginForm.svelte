<script lang="ts">
	// Ref: japaconnect-engineering/sections/02-authentication-sessions.md
	// Presentational login form — calls the /api/auth/login proxy, sets the
	// cookie server-side, and invokes onSuccess. Used by both the /login route
	// (via form action) and the AuthDialog modal (via fetch).
	import { t } from '$lib/i18n/store.svelte';
	import { invalidateAll } from '$app/navigation';
	import { goto } from '$app/navigation';
	import { validateLogin } from '$lib/auth/validation';
	import { mapAuthError } from '$lib/auth/errors';
	import PasswordField from './PasswordField.svelte';
	import OAuthButtons from './OAuthButtons.svelte';
	import AuthFooter from './AuthFooter.svelte';
	import HoneypotField from './HoneypotField.svelte';

	let {
		redirectTarget = '/',
		onSuccess,
		showSwitchLink = true
	}: {
		redirectTarget?: string;
		onSuccess?: (redirect: string) => void | Promise<void>;
		showSwitchLink?: boolean;
	} = $props();

	let email = $state('');
	let password = $state('');
	let loading = $state(false);
	let errors = $state<Record<string, string>>({});
	let honeypot = $state('');

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errors = {};
		const v = validateLogin(email, password);
		if (!v.valid) {
			errors = v.errors;
			return;
		}

		loading = true;
		try {
			const res = await fetch('/api/auth/login', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({ email, password, redirect: redirectTarget, website: honeypot })
			});
			const data = await res.json();
			if (!res.ok) {
				const mapped = mapAuthError(data?.error?.code, res.status);
				errors = mapped.field ? { [mapped.field]: t(mapped.messageKey) } : {};
				return;
			}
			// 2FA-enabled account: redirect to the TOTP challenge page. The
			// challenge token + email + redirect target are passed via query.
			if (data.totp_required) {
				await goto(data.redirect);
				return;
			}
			await invalidateAll();
			if (onSuccess) {
				await onSuccess(data.redirect ?? redirectTarget);
			} else {
				await goto(data.redirect ?? redirectTarget);
			}
		} catch {
			errors = { form: t('auth.errors.generic') };
		} finally {
			loading = false;
		}
	}
</script>

<form onsubmit={submit} class="flex flex-col gap-5" novalidate>
	<HoneypotField bind:value={honeypot} />
	<div class="flex items-start justify-between gap-4">
		<div>
			<h1 class="text-2xl font-semibold text-[var(--color-text)]">{t('auth.login.title')}</h1>
			<p class="mt-1 text-sm text-[var(--color-muted)]">{t('auth.login.subtitle')}</p>
		</div>
		{#if showSwitchLink}
			<a
				href="/signup"
				class="shrink-0 text-sm text-[var(--color-primary)] transition hover:underline whitespace-nowrap"
			>
				{t('auth.login.switch_to_signup')}
			</a>
		{/if}
	</div>

	{#if errors.form}
		<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
			{t(errors.form)}
		</p>
	{/if}

	<div class="flex flex-col gap-1.5">
		<label for="login-email" class="text-sm font-medium text-[var(--color-text)]">{t('auth.fields.email')}</label>
		<input
			id="login-email"
			type="email"
			autocomplete="email"
			bind:value={email}
			placeholder={t('auth.fields.email')}
			aria-label={t('auth.fields.email')}
			aria-invalid={errors.email ? 'true' : undefined}
			aria-describedby={errors.email ? 'login-email-error' : undefined}
			class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2.5 text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
		/>
		{#if errors.email}<p id="login-email-error" class="text-sm text-red-500">{t(errors.email)}</p>{/if}
	</div>

	<PasswordField
		id="login-password"
		label={t('auth.fields.password')}
		bind:value={password}
		error={errors.password ?? ''}
	/>

	<div class="flex justify-end">
		<a href="/forgot-password" class="text-sm text-[var(--color-primary)] hover:underline">
			{t('auth.forgot_password.title')}
		</a>
	</div>

	<button
		type="submit"
		disabled={loading}
		class="w-full rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2.5 text-sm font-medium text-white transition hover:opacity-90 disabled:opacity-50"
	>
		{loading ? '...' : t('auth.login.submit')}
	</button>

	<div class="relative my-1">
		<div class="border-t border-[var(--color-border)]"></div>
		<span class="absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 bg-[var(--color-bg)] px-3 text-xs text-[var(--color-muted)]">
			{t('auth.oauth.or_continue_with')}
		</span>
	</div>

	<OAuthButtons redirect={redirectTarget} />

	<AuthFooter />
</form>
