<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { MailX, CheckCircle2, AlertCircle } from 'lucide-svelte';
	import { enhance } from '$app/forms';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let unsubResult = $state<{ ok?: boolean; error?: string } | null>(null);
	let submitting = $state(false);

	// Auto-submit if token is present in the URL (one-click unsubscribe).
	let autoSubmitted = $state(false);
</script>

<svelte:head>
	<title>{t('newsletter.unsubscribe_title', 'Unsubscribe — {site}')}</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto flex min-h-[60vh] max-w-md flex-col items-center justify-center px-4">
	{#if unsubResult?.ok}
		<div class="text-center">
			<CheckCircle2 size={48} class="mx-auto text-green-500" />
			<h1 class="mt-4 text-lg font-semibold text-[var(--color-text)]">
				{t('newsletter.unsubscribed', 'You have been unsubscribed')}
			</h1>
			<p class="mt-2 text-sm text-[var(--color-muted)]">
				{t('newsletter.unsubscribed_body', 'You will no longer receive emails from us. You can resubscribe at any time.')}
			</p>
			<a href="/" class="mt-4 inline-block text-sm text-[var(--color-primary)] hover:underline">
				{t('common.back_home', 'Back to home')}
			</a>
		</div>
	{:else if data.token && !autoSubmitted}
		<!-- One-click unsubscribe with token -->
		<div class="w-full text-center">
			<MailX size={48} class="mx-auto text-[var(--color-muted)]" />
			<h1 class="mt-4 text-lg font-semibold text-[var(--color-text)]">
				{t('newsletter.confirm_unsubscribe', 'Confirm unsubscribe')}
			</h1>
			<p class="mt-2 text-sm text-[var(--color-muted)]">
				{t('newsletter.confirm_unsubscribe_body', 'Click below to stop receiving our newsletter.')}
			</p>
			<form
				method="POST"
				action="?/unsubscribe"
				use:enhance={() => {
					autoSubmitted = true;
					submitting = true;
					return async ({ result: actionResult }) => {
						submitting = false;
						if (actionResult.type === 'success') {
							unsubResult = actionResult.data as { ok?: boolean; error?: string };
						}
					};
				}}
				class="mt-4"
			>
				<input type="hidden" name="token" value={data.token} />
				<button
					type="submit"
					disabled={submitting}
					class="w-full rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2.5 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50"
				>
					{submitting ? t('common.submitting', 'Submitting…') : t('newsletter.unsubscribe_btn', 'Unsubscribe me')}
				</button>
			</form>
		</div>
	{:else}
		<!-- Manual unsubscribe by email -->
		<div class="w-full text-center">
			<MailX size={48} class="mx-auto text-[var(--color-muted)]" />
			<h1 class="mt-4 text-lg font-semibold text-[var(--color-text)]">
				{t('newsletter.unsubscribe', 'Unsubscribe')}
			</h1>
			<p class="mt-2 text-sm text-[var(--color-muted)]">
				{t('newsletter.unsubscribe_body', 'Enter your email to unsubscribe from our newsletter.')}
			</p>
			{#if unsubResult?.error}
				<div class="mt-3 flex items-center justify-center gap-2 text-sm text-red-500">
					<AlertCircle size={16} /> {unsubResult?.error}
				</div>
			{/if}
			<form
				method="POST"
				action="?/unsubscribe"
				use:enhance={() => {
					submitting = true;
					return async ({ result: actionResult }) => {
						submitting = false;
						if (actionResult.type === 'success') {
							unsubResult = actionResult.data as { ok?: boolean; error?: string };
						}
					};
				}}
				class="mt-4 space-y-3"
			>
				<input
					id="unsubscribe-email"
					type="email"
					name="email"
					value={data.email}
					placeholder={t('newsletter.email_placeholder', 'Your email')}
					aria-label={t('auth.fields.email', 'Email')}
					aria-describedby="unsubscribe-email-hint"
					required
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2.5 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
				/>
				<button
					type="submit"
					disabled={submitting}
					class="w-full rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2.5 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50"
				>
					{submitting ? t('common.submitting', 'Submitting…') : t('newsletter.unsubscribe_btn', 'Unsubscribe me')}
				</button>
			</form>
		</div>
	{/if}
</div>
