<script lang="ts">
	import { t } from '$lib/i18n';
	import { Bell, Mail, CheckCircle2, AlertCircle } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { enhance } from '$app/forms';
	import type { NewsletterSubscriber } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const subscriber = $state<NewsletterSubscriber | null>(data.subscriber);

	let digestEnabled = $state(subscriber?.digest_enabled ?? false);
	let campaignEnabled = $state(subscriber?.campaign_enabled ?? false);
	let result = $state<{ ok?: boolean; error?: string; subscriber?: NewsletterSubscriber } | null>(null);
	let saving = $state(false);
</script>

<svelte:head>
	<title>{t('notifications.title', 'Notification Preferences — JapaConnect')}</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-2xl">
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Bell size={20} /> {t('notifications.heading', 'Notification Preferences')}
		</h1>
	</div>

	<div class="p-4">
		{#if !subscriber}
			<EmptyState
				icon={Mail}
				title={t('notifications.not_subscribed', 'You are not subscribed to the newsletter')}
				body={t('notifications.not_subscribed_body', 'Subscribe from the sidebar to manage email preferences.')}
			/>
		{:else}
			{#if result?.ok}
				<div class="mb-3 flex items-center gap-2 text-sm text-green-600">
					<CheckCircle2 size={16} /> {t('notifications.saved', 'Preferences saved')}
				</div>
			{:else if result?.error}
				<div class="mb-3 flex items-center gap-2 text-sm text-red-500">
					<AlertCircle size={16} /> {result.error}
				</div>
			{/if}

			<form
				method="POST"
				action="?/update"
				use:enhance={() => {
					saving = true;
					return async ({ result: r }) => {
						saving = false;
						if (r.type === 'success') {
							result = r.data as { ok?: boolean; error?: string; subscriber?: NewsletterSubscriber };
							if (result?.subscriber) {
								digestEnabled = result.subscriber.digest_enabled;
								campaignEnabled = result.subscriber.campaign_enabled;
							}
						}
					};
				}}
				class="space-y-4"
			>
				<label class="flex items-start gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<input
						type="checkbox"
						name="digest_enabled"
						checked={digestEnabled}
						onchange={(e) => (digestEnabled = (e.target as HTMLInputElement).checked)}
						class="mt-0.5 accent-[var(--color-primary)]"
					/>
					<span class="min-w-0 flex-1">
						<span class="block text-sm font-medium text-[var(--color-text)]">
							{t('notifications.digest', 'Weekly digest')}
						</span>
						<span class="mt-0.5 block text-xs text-[var(--color-muted)]">
							{t('notifications.digest_desc', 'A summary of the best content from the week, delivered to your inbox.')}
						</span>
					</span>
				</label>

				<label class="flex items-start gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<input
						type="checkbox"
						name="campaign_enabled"
						checked={campaignEnabled}
						onchange={(e) => (campaignEnabled = (e.target as HTMLInputElement).checked)}
						class="mt-0.5 accent-[var(--color-primary)]"
					/>
					<span class="min-w-0 flex-1">
						<span class="block text-sm font-medium text-[var(--color-text)]">
							{t('notifications.campaign', 'Promotional emails')}
						</span>
						<span class="mt-0.5 block text-xs text-[var(--color-muted)]">
							{t('notifications.campaign_desc', 'Special offers and announcements from our partners.')}
						</span>
					</span>
				</label>

				<button
					type="submit"
					disabled={saving}
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50"
				>
					{saving ? t('common.saving', 'Saving…') : t('common.save', 'Save')}
				</button>
			</form>

			<div class="mt-6 border-t border-[var(--color-border)] pt-4">
				<a href="/unsubscribe?email={encodeURIComponent(subscriber.email)}" class="text-xs text-[var(--color-muted)] hover:underline">
					{t('notifications.unsubscribe_all', 'Unsubscribe from all emails')}
				</a>
			</div>
		{/if}
	</div>
</div>
