<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	import { t } from '$lib/i18n/store.svelte';
	import { page } from '$app/state';
	import { validateResetPassword } from '$lib/auth/validation';
	import { mapAuthError } from '$lib/auth/errors';
	import PasswordField from '$lib/components/auth/PasswordField.svelte';
	import HoneypotField from '$lib/components/auth/HoneypotField.svelte';

	const token = $derived(page.data.token);

	let password = $state('');
	let confirm = $state('');
	let loading = $state(false);
	let errors = $state<Record<string, string>>({});
	let success = $state(false);
	let honeypot = $state('');

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errors = {};
		const v = validateResetPassword(password, confirm);
		if (!v.valid) {
			errors = v.errors;
			return;
		}

		loading = true;
		try {
			const res = await fetch('/api/auth/reset-password', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({ token, new_password: password, website: honeypot })
			});
			if (!res.ok) {
				const data = await res.json().catch(() => null);
				const mapped = mapAuthError(data?.error?.code ?? '', res.status);
				errors = mapped.field ? { [mapped.field]: t(mapped.messageKey) } : {};
				return;
			}
			success = true;
		} catch {
			errors = { form: t('auth.errors.generic') };
		} finally {
			loading = false;
		}
	}
</script>

<form onsubmit={submit} class="flex flex-col gap-4" novalidate>
	<HoneypotField bind:value={honeypot} />
	<h1 class="text-2xl font-semibold text-[var(--color-text)]">{t('auth.reset_password.title')}</h1>
	<p class="text-sm text-[var(--color-muted)]">{t('auth.reset_password.subtitle')}</p>

	{#if success}
		<p class="rounded-[var(--radius)] bg-green-500/10 px-3 py-2 text-sm text-green-600" role="status">
			{t('auth.reset_password.success')}
		</p>
		<a href="/login" class="text-sm text-[var(--color-primary)] hover:underline">
			{t('auth.reset_password.back_to_login')}
		</a>
	{:else}
		{#if errors.form}
			<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
				{errors.form}
			</p>
		{/if}

		<PasswordField
			id="reset-password"
			label={t('auth.fields.password')}
			bind:value={password}
			showStrength
			error={errors.password ?? ''}
			autocomplete="new-password"
			hint={t('auth.fields.new_password_hint', 'At least 8 characters with upper & lower case letters and a number.')}
		/>

		<div class="flex flex-col gap-1.5">
			<label for="reset-confirm" class="text-sm font-medium text-[var(--color-text)]">{t('auth.fields.confirm_password')}</label>
			<input
				id="reset-confirm"
				type="password"
				autocomplete="new-password"
				bind:value={confirm}
				placeholder={t('auth.fields.confirm_password_placeholder', 'Repeat your new password')}
				aria-invalid={errors.confirm ? 'true' : undefined}
				aria-describedby="reset-confirm-hint"
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2.5 text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
			/>
			<HintIcon id="reset-confirm-hint" text={t('auth.fields.confirm_password_hint', 'Re-enter your new password exactly.')} />
			{#if errors.confirm}<p class="text-sm text-red-500">{errors.confirm}</p>{/if}
		</div>

		<button
			type="submit"
			disabled={loading}
			class="w-full rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2.5 text-sm font-medium text-white transition hover:opacity-90 disabled:opacity-50"
		>
			{loading ? '...' : t('auth.reset_password.submit')}
		</button>
	{/if}
</form>
