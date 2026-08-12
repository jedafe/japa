<script lang="ts">
	// Ref: japaconnect-engineering/sections/33-professional-profiles.md (invitation acceptance)
	import { t } from '$lib/i18n';
	import { enhance } from '$app/forms';
	import { Loader2, CheckCircle2, AlertCircle, Mail } from 'lucide-svelte';
	import { goto } from '$app/navigation';
	import type { ActionData, PageData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();
	let submitting = $state(false);

	const errorKey = $derived(form?.message ?? '');
	const isSuccess = $derived(form?.success === true);

	const errorMessages: Record<string, string> = {
		'login_required': 'profiles.invitations.login_required',
		'invalid': 'profiles.invitations.error_invalid',
		'email_mismatch': 'profiles.invitations.error_email_mismatch',
		'failed': 'profiles.invitations.error_failed'
	};
</script>

<svelte:head>
	<title>{t('profiles.invitations.title')}</title>
</svelte:head>

<div class="mx-auto flex min-h-[60vh] w-full max-w-md flex-col items-center justify-center px-4">
	<div class="w-full rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6">
		<div class="mb-4 flex items-center justify-center">
			<div class="flex h-12 w-12 items-center justify-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
				<Mail class="h-6 w-6" />
			</div>
		</div>

		<h1 class="text-center text-lg font-semibold text-[var(--color-text)]">{t('profiles.invitations.heading')}</h1>
		<p class="mt-1 text-center text-sm text-[var(--color-text-subtle)]">{t('profiles.invitations.subtitle')}</p>

		{#if isSuccess}
			<div class="mt-6 flex flex-col items-center gap-3">
				<CheckCircle2 class="h-10 w-10 text-[var(--color-success)]" />
				<p class="text-center text-sm text-[var(--color-text)]">{t('profiles.invitations.success')}</p>
				<a
					href="/dashboard/profiles"
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition hover:opacity-90"
				>
					{t('profiles.dashboard.title')}
				</a>
			</div>
		{:else if errorKey}
			<div class="mt-6 flex flex-col items-center gap-3">
				<AlertCircle class="h-10 w-10 text-[var(--color-danger)]" />
				<p class="text-center text-sm text-[var(--color-danger)]">
					{t(errorMessages[errorKey] ?? 'profiles.invitations.error_failed')}
				</p>
			</div>
		{:else}
			<form
				method="POST"
				use:enhance={() => {
					submitting = true;
					return async ({ update }) => {
						await update();
						submitting = false;
					};
				}}
				class="mt-6"
			>
				<button
					type="submit"
					disabled={submitting}
					class="flex w-full items-center justify-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2.5 text-sm font-medium text-[var(--color-primary-contrast)] transition hover:opacity-90 disabled:opacity-50"
				>
					{#if submitting}
						<Loader2 class="h-4 w-4 animate-spin" />
						{t('profiles.invitations.accepting')}
					{:else}
						{t('profiles.invitations.accept')}
					{/if}
				</button>
			</form>
		{/if}
	</div>
</div>
