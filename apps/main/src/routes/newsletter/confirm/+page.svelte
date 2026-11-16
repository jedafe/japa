<script lang="ts">
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import { t } from '#lib/i18n/index.js';
	import { MailCheck, CheckCircle2, AlertCircle } from 'lucide-svelte';
	import { enhance } from '$app/forms';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let result = $state<{ ok?: boolean; error?: string } | null>(null);
	let submitting = $state(false);
</script>

<SeoHead
	title={t('newsletter.confirm_title', 'Confirm subscription — {site}')}
	robots="noindex"
	description={t('newsletter.confirm_description', 'Confirm your newsletter subscription.')}
/>

<div class="mx-auto flex min-h-[60vh] max-w-md flex-col items-center justify-center px-4">
	{#if result?.ok}
		<div class="text-center">
			<CheckCircle2 size={48} class="mx-auto text-green-500" />
			<h1 class="mt-4 text-lg font-semibold text-[var(--color-text)]">
				{t('newsletter.confirmed', 'Subscription confirmed')}
			</h1>
			<p class="mt-2 text-sm text-[var(--color-muted)]">
				{t('newsletter.confirmed_body', 'You are now subscribed to our newsletter.')}
			</p>
			<a href="/" class="mt-4 inline-block text-sm text-[var(--color-primary-text)] hover:underline">
				{t('common.back_home', 'Back to home')}
			</a>
		</div>
	{:else if data.token}
		<div class="w-full text-center">
			<MailCheck size={48} class="mx-auto text-[var(--color-muted)]" />
			<h1 class="mt-4 text-lg font-semibold text-[var(--color-text)]">
				{t('newsletter.confirm_subscription', 'Confirm your subscription')}
			</h1>
			<p class="mt-2 text-sm text-[var(--color-muted)]">
				{t('newsletter.confirm_subscription_body', 'Click below to confirm and start receiving our newsletter.')}
			</p>
			{#if result?.error}
				<div class="mt-3 flex items-center justify-center gap-2 text-sm text-red-500">
					<AlertCircle size={16} />
					{t('newsletter.confirm_failed', 'This confirmation link is invalid or has already been used.')}
				</div>
			{/if}
			<form
				method="POST"
				action="?/confirm"
				use:enhance={() => {
					submitting = true;
					return async ({ result: actionResult }) => {
						submitting = false;
						if (actionResult.type === 'success') {
							result = actionResult.data as { ok?: boolean; error?: string };
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
					{submitting ? t('common.submitting', 'Submitting…') : t('newsletter.confirm_btn', 'Confirm subscription')}
				</button>
			</form>
		</div>
	{:else}
		<div class="w-full text-center">
			<AlertCircle size={48} class="mx-auto text-[var(--color-muted)]" />
			<h1 class="mt-4 text-lg font-semibold text-[var(--color-text)]">
				{t('newsletter.confirm_missing', 'Missing confirmation link')}
			</h1>
			<p class="mt-2 text-sm text-[var(--color-muted)]">
				{t('newsletter.confirm_missing_body', 'Use the confirmation link from your subscription email.')}
			</p>
			<a href="/" class="mt-4 inline-block text-sm text-[var(--color-primary-text)] hover:underline">
				{t('common.back_home', 'Back to home')}
			</a>
		</div>
	{/if}
</div>
