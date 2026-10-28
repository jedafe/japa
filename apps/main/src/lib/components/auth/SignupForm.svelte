<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	// Ref: japaconnect-engineering/sections/02-authentication-sessions.md
	// Presentational signup form — calls the /api/auth/signup proxy, sets the
	// cookie server-side, and redirects to /onboarding for new users.
	import { t } from '$lib/i18n/store.svelte';
	import { invalidateAll } from '$app/navigation';
	import { goto } from '$app/navigation';
	import { validateSignup } from '$lib/auth/validation';
	import { mapAuthError } from '$lib/auth/errors';
	import PasswordField from './PasswordField.svelte';
	import OAuthButtons from './OAuthButtons.svelte';
	import AuthFooter from './AuthFooter.svelte';
	import HoneypotField from './HoneypotField.svelte';
	import { completePendingNextAction } from '$lib/api/nextActions';

	let {
		redirectTarget = '/onboarding',
		onSuccess,
		showSwitchLink = true
	}: {
		redirectTarget?: string;
		onSuccess?: (redirect: string) => void | Promise<void>;
		showSwitchLink?: boolean;
	} = $props();

	let email = $state('');
	let username = $state('');
	let password = $state('');
	let termsAccepted = $state(false);
	let loading = $state(false);
	let errors = $state<Record<string, string>>({});
	let honeypot = $state('');

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errors = {};
		const v = validateSignup(email, username, password);
		if (!v.valid) {
			errors = v.errors;
			return;
		}
		// Terms agreement is a client-side gate (no backend contract).
		if (!termsAccepted) {
			errors = { terms: t('auth.signup.terms_required') };
			return;
		}

		loading = true;
		try {
			const res = await fetch('/api/auth/signup', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({ email, username, password, redirect: redirectTarget, website: honeypot })
			});
			const data = await res.json();
			if (!res.ok) {
				const mapped = mapAuthError(data?.error?.code, res.status);
				errors = mapped.field ? { [mapped.field]: t(mapped.messageKey) } : {};
				return;
			}
			await invalidateAll();
			// §122 — a pending suggestion that drove the signup completes for
			// real here (rules without completion_signal otherwise never do).
			completePendingNextAction();
			if (onSuccess) {
				await onSuccess(data.redirect ?? '/onboarding');
			} else {
				await goto(data.redirect ?? '/onboarding');
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
	<div class="flex flex-col items-center gap-2 text-center sm:flex-row sm:items-start sm:justify-between sm:gap-4 sm:text-start">
		<div>
			<h1 class="text-2xl font-semibold text-[var(--color-text)]">{t('auth.signup.title')}</h1>
			<p class="mt-1 text-sm text-[var(--color-muted)]">{t('auth.signup.subtitle')}</p>
		</div>
		{#if showSwitchLink}
			<a
				href="/login"
				class="text-sm text-[var(--color-primary)] transition hover:underline"
			>
				{t('auth.signup.switch_to_login')}
			</a>
		{/if}
	</div>

	{#if errors.form}
		<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
			{t(errors.form)}
		</p>
	{/if}

	<div class="flex flex-col gap-1.5">
		<label for="signup-email" class="sr-only">{t('auth.fields.email')}</label>
		<input
			id="signup-email"
			type="email"
			autocomplete="email"
			bind:value={email}
			placeholder={t('auth.fields.email_placeholder', 'you@example.com')}
			aria-label={t('auth.fields.email')}
			aria-invalid={errors.email ? 'true' : undefined}
			aria-describedby={errors.email ? 'signup-email-hint signup-email-error' : 'signup-email-hint'}
			class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2.5 text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
		/>
		{#if errors.email}<p id="signup-email-error" class="text-sm text-red-500">{t(errors.email)}</p>{/if}
	</div>

	<div class="flex flex-col gap-1.5">
		<label for="signup-username" class="sr-only">{t('auth.fields.username')}</label>
		<input
			id="signup-username"
			type="text"
			autocomplete="username"
			bind:value={username}
			placeholder={t('auth.fields.username_placeholder', 'e.g. ada_lovelace')}
			aria-label={t('auth.fields.username')}
			aria-invalid={errors.username ? 'true' : undefined}
			aria-describedby={errors.username ? 'signup-username-hint signup-username-error' : 'signup-username-hint'}
			class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2.5 text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
		/>
		<HintIcon id="signup-username-hint" text={t('auth.fields.username_hint', '3–30 characters — letters, numbers and underscoresswordField
		id="signup-password"
		label={t('auth.fields.password')}
		bind:value={password}
		showStrength
		error={errors.password ?? ''}
		autocomplete="new-password"
		hint={t('auth.fields.new_password_hint', 'At least 8 characters with upper & lower case letters and a number.')}
		hideLabel
	/>

	<div class="flex flex-col gap-1.5">
		<label for="signup-terms" class="flex cursor-pointer items-start gap-2.5 text-sm text-[var(--color-muted)]">
			<input
				id="signup-terms"
				type="checkbox"
				bind:checked={termsAccepted}
				aria-invalid={errors.terms ? 'true' : undefined}
				aria-describedby={errors.terms ? 'signup-terms-error' : undefined}
				class="mt-0.5 h-4 w-4 shrink-0 cursor-pointer rounded border-[var(--color-border)] accent-[var(--color-primary)]"
			/>
			<span>{t('auth.signup.terms_label')}</span>
		</label>
		{#if errors.terms}
			<p id="signup-terms-error" class="text-sm text-red-500">{t(errors.terms)}</p>
		{/if}
	</div>

	<button
		type="submit"
		disabled={loading}
		class="w-full rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2.5 text-sm font-medium text-white transition hover:opacity-90 disabled:opacity-50"
	>
		{loading ? '...' : t('auth.signup.submit')}
	</button>

	<div class="relative my-1">
		<div class="border-t border-[var(--color-border)]"></div>
		<span class="absolute start-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 bg-[var(--color-bg)] px-3 text-xs text-[var(--color-muted)]">
			{t('auth.oauth.or_continue_with')}
		</span>
	</div>

	<OAuthButtons redirect={redirectTarget} />

	<AuthFooter />
</form>
