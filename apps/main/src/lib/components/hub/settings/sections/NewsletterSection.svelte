<script lang="ts">
	/**
	 * Newsletter preferences — Section 62 / ADR-080.
	 *
	 * Renders the subscriber's digest_enabled and campaign_enabled toggles
	 * fetched from GET /newsletter/preferences (Rule 16: no hardcoded data).
	 * Saves via PATCH /newsletter/preferences.
	 *
	 * When the user is not subscribed (preferences is null), renders a
	 * subscribe CTA that calls POST /newsletter/subscribe.
	 *
	 * Ref: japaconnect-engineering/sections/62-newsletter-digest.md
	 *      japaconnect-engineering/adr/ADR-080-newsletter-digest.md
	 */
	import { Modal } from '$lib/components/ui';
	import { untrack } from 'svelte';
	import { beforeNavigate, invalidateAll } from '$app/navigation';
	import { DirtyForm } from '$lib/components/hub/useDirtyForm.svelte';
	import { patchNewsletterPreferences, unsubscribeNewsletter } from '$lib/api/hub';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import { Mail, Rss, Megaphone } from 'lucide-svelte';

	type NewsletterPrefs = {
		email?: string;
		status?: string;
		digest_enabled?: boolean;
		campaign_enabled?: boolean;
	};

	let { data }: { data: unknown } = $props();
	const sectionData = $derived((data as { preferences: NewsletterPrefs | null } | null) ?? null);
	const prefs = $derived(sectionData?.preferences ?? null);

	const form = new DirtyForm<NewsletterPrefs>(
		untrack(() => ({
			digest_enabled: prefs?.digest_enabled ?? false,
			campaign_enabled: prefs?.campaign_enabled ?? false
		})),
		async (v) => {
			return patchNewsletterPreferences({
				digest_enabled: v.digest_enabled,
				campaign_enabled: v.campaign_enabled
			});
		}
	);

	beforeNavigate((nav) => {
		if (form.dirty && !confirm(t('settings.unsaved_warning'))) nav.cancel();
	});

	// Subscribe CTA state (shown when prefs is null).
	let subscribeEmail = $state('');
	let subscribing = $state(false);
	let subscribeMessage = $state<{ kind: 'success' | 'error'; text: string } | null>(null);

	// Unsubscribe state.
	let unsubscribeOpen = $state(false);
	let unsubscribing = $state(false);
	let unsubscribeMessage = $state<string | null>(null);

	async function handleSubscribe(e: SubmitEvent) {
		e.preventDefault();
		if (subscribing) return;
		const email = subscribeEmail.trim();
		if (!email) return;
		subscribing = true;
		subscribeMessage = null;
		const res = await api.post<NewsletterPrefs>('/newsletter/subscribe', { email, source: 'signup' });
		subscribing = false;
		if (res.error) {
			subscribeMessage = { kind: 'error', text: res.error.message ?? t('settings.newsletter.error', 'Something went wrong. Please try again.') };
			return;
		}
		subscribeMessage = { kind: 'success', text: t('settings.newsletter.subscribed', 'You are now subscribed.') };
		// Reload the page to re-run the server loader and show the toggles.
		setTimeout(() => window.location.reload(), 1200);
	}

	async function handleUnsubscribe() {
		if (unsubscribing || !prefs?.email) return;
		unsubscribing = true;
		unsubscribeMessage = null;
		const res = await unsubscribeNewsletter(prefs.email);
		unsubscribing = false;
		if (res.error) {
			unsubscribeMessage = res.error.message ?? t('settings.newsletter.error', 'Something went wrong. Please try again.');
			return;
		}
		unsubscribeOpen = false;
		unsubscribeMessage = null;
		// Re-run the loader so the section flips back to the subscribe CTA.
		await invalidateAll();
	}
</script>

<section class="max-w-2xl">
	<header class="mb-4">
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('settings.nav.newsletter', 'Newsletter')}</h1>
		<p class="text-sm text-[var(--color-muted)]">{t('settings.newsletter.desc', 'Manage your newsletter and digest email preferences.')}</p>
	</header>

	{#if prefs}
		<!-- Subscriber preferences: digest + campaign toggles -->
		<ul class="space-y-2">
			<li class="flex flex-col gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-3 sm:flex-row sm:items-center sm:gap-4">
				<div class="flex flex-1 items-start gap-3">
					<Rss size={18} class="mt-0.5 shrink-0 text-[var(--color-muted)]" />
					<div class="min-w-0">
						<p class="text-sm font-medium text-[var(--color-text)]">{t('settings.newsletter.digest', 'Weekly digest')}</p>
						<p class="text-xs text-[var(--color-muted)]">{t('settings.newsletter.digest_desc', 'A weekly summary of the best content from your followed spaces and tags.')}</p>
					</div>
				</div>
				<label class="flex cursor-pointer items-center gap-2 text-sm text-[var(--color-text)]">
					<input
						type="checkbox"
						class="h-4 w-4 rounded border-[var(--color-border)] accent-[var(--color-primary)]"
						checked={form.values.digest_enabled}
						onchange={(e) => form.set({ ...form.values, digest_enabled: e.currentTarget.checked })}
						aria-label={t('settings.newsletter.digest', 'Weekly digest')}
					/>
				</label>
			</li>
			<li class="flex flex-col gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-3 sm:flex-row sm:items-center sm:gap-4">
				<div class="flex flex-1 items-start gap-3">
					<Megaphone size={18} class="mt-0.5 shrink-0 text-[var(--color-muted)]" />
					<div class="min-w-0">
						<p class="text-sm font-medium text-[var(--color-text)]">{t('settings.newsletter.campaigns', 'Promotional campaigns')}</p>
						<p class="text-xs text-[var(--color-muted)]">{t('settings.newsletter.campaigns_desc', 'Occasional product updates, offers, and announcements.')}</p>
					</div>
				</div>
				<label class="flex cursor-pointer items-center gap-2 text-sm text-[var(--color-text)]">
					<input
						type="checkbox"
						class="h-4 w-4 rounded border-[var(--color-border)] accent-[var(--color-primary)]"
						checked={form.values.campaign_enabled}
						onchange={(e) => form.set({ ...form.values, campaign_enabled: e.currentTarget.checked })}
						aria-label={t('settings.newsletter.campaigns', 'Promotional campaigns')}
					/>
				</label>
			</li>
		</ul>

		{#if prefs.email}
			<div class="mt-3 flex items-center justify-between gap-3">
				<p class="text-xs text-[var(--color-muted)]">
					{t('settings.newsletter.subscribed_as', 'Subscribed as')} <strong>{prefs.email}</strong>
				</p>
				<button
					type="button"
					onclick={() => (unsubscribeOpen = true)}
					class="text-xs font-medium text-[var(--color-danger,#dc2626)] hover:underline"
				>
					{t('settings.newsletter.unsubscribe', 'Unsubscribe')}
				</button>
			</div>
		{/if}

		<Modal bind:open={unsubscribeOpen} title={t('settings.newsletter.unsubscribe_confirm_title', 'Unsubscribe from newsletter?')} description={t('settings.newsletter.unsubscribe_confirm_desc', 'You can resubscribe at any time from this page.')} size="sm">
			{#snippet children(close)}
				{#if unsubscribeMessage}
					<p class="mb-3 text-xs text-[var(--color-danger,#dc2626)]">{unsubscribeMessage}</p>
				{/if}
				<div class="flex items-center justify-end gap-2">
					<button
						type="button"
						disabled={unsubscribing}
						onclick={close}
						class="rounded-[var(--radius)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
					>
						{t('settings.action.cancel', 'Cancel')}
					</button>
					<button
						type="button"
						disabled={unsubscribing}
						onclick={handleUnsubscribe}
						class="rounded-[var(--radius)] bg-[var(--color-danger,#dc2626)] px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50"
					>
						{unsubscribing ? t('common.submitting', 'Submitting…') : t('settings.newsletter.unsubscribe', 'Unsubscribe')}
					</button>
				</div>
			{/snippet}
		</Modal>

		{#if form.dirty}
			<div
				class="sticky bottom-0 mt-6 flex items-center justify-end gap-2 border-t border-[var(--color-border)] bg-[var(--color-surface)] py-3"
			>
				{#if form.error}
					<span class="me-auto text-xs text-[var(--color-danger,#dc2626)]">{form.error.message}</span>
				{/if}
				<button
					type="button"
					class="rounded-[var(--radius)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
					onclick={() => form.reset()}>{t('settings.action.discard')}</button
				>
				<button
					type="button"
					disabled={form.saving}
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50"
					onclick={() => form.save()}>{t('settings.action.save')}</button
				>
			</div>
		{/if}
	{:else}
		<!-- Not subscribed: show subscribe CTA -->
		<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 text-center">
			<Mail size={32} class="mx-auto text-[var(--color-muted)]" />
			<h2 class="mt-3 text-sm font-medium text-[var(--color-text)]">{t('settings.newsletter.not_subscribed', 'You are not subscribed to the newsletter')}</h2>
			<p class="mt-1 text-xs text-[var(--color-muted)]">{t('settings.newsletter.not_subscribed_desc', 'Subscribe to receive the weekly digest and occasional updates.')}</p>
			<form onsubmit={handleSubscribe} class="mx-auto mt-4 flex max-w-sm gap-2">
				<input
					type="email"
					bind:value={subscribeEmail}
					placeholder={t('landing.newsletter_placeholder', 'your@email.com')}
					required
					disabled={subscribing}
					class="flex-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none disabled:opacity-50"
				/>
				<button
					type="submit"
					disabled={subscribing}
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
				>
					{subscribing ? t('common.submitting', 'Submitting…') : t('landing.newsletter_subscribe', 'Subscribe')}
				</button>
			</form>
			{#if subscribeMessage}
				<p class="mt-3 text-sm {subscribeMessage.kind === 'error' ? 'text-[var(--color-danger)]' : 'text-[var(--color-success)]'}">{subscribeMessage.text}</p>
			{/if}
		</div>
	{/if}
</section>
