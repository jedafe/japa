<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	// "Message provider / Message Mentor" — opens a small modal that starts
	// (or reuses) a DM conversation with the listing author via
	// POST /dm/conversations, then deep-links into /messages?c={id}.
	// Guests get the login prompt (same pattern as BookingDialog).
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { Loader2, Send } from 'lucide-svelte';
	import { Modal } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import type { ServiceListing } from '$lib/types';

	interface Props {
		service: ServiceListing;
		authed: boolean;
		open?: boolean;
	}
	let { service, authed, open = $bindable(false) }: Props = $props();

	const loginHref = $derived(
		`/login?redirect=${encodeURIComponent(page.url.pathname + page.url.search)}`
	);
	const recipientName = $derived(
		service.mentor?.display_name ||
			service.mentor?.username ||
			service.org_name ||
			service.author_name ||
			service.title
	);

	let body = $state('');
	let sending = $state(false);
	let sendError = $state('');
	let sent = $state(false);

	// Reset the inquiry success state when the dialog closes so a reopen
	// shows the composer, not a stale confirmation.
	$effect(() => {
		if (!open) {
			sent = false;
			sendError = '';
		}
	});

	// Channel by listing kind: mentorship contact is a personal 1:1 DM
	// (structured context card; the same thread the mentorship accept
	// materializes) and org-owned listings open an org conversation that
	// every member can answer (000942). Personal non-mentorship service
	// contact is a commercial lead — POST /services/:id/bookings
	// intent='inquiry' lands it in the author's bookings inbox.
	const isMentorship = $derived(service.listing_kind === 'mentorship');
	const dmChannel = $derived(isMentorship || !!service.org_id);

	function friendlyError(code: string | undefined, fallback: string): string {
		switch (code) {
			case 'self_conversation':
			case 'self_booking':
				return t('services.message_err_self', "You can't message your own listing.");
			case 'dm_privacy_blocked':
			case 'dm_not_mutual_follower':
			case 'dm_blocked':
				return t(
					'services.message_err_unavailable',
					"This provider isn't accepting direct messages right now."
				);
			case 'invalid_context':
				return t(
					'services.message_err_context',
					"This listing can't accept messages right now."
				);
			case 'duplicate_inquiry':
				return t(
					'services.message_err_dup_inquiry',
					'You already have an open inquiry with this provider.'
				);
			default:
				return fallback;
		}
	}

	async function send(e: SubmitEvent) {
		e.preventDefault();
		if (!body.trim() || sending) return;
		sendError = '';
		sending = true;
		if (dmChannel) {
			const res = await api.post<{ conversation_id: string }>('/dm/conversations', {
				recipient_id: service.author_id,
				message: body.trim(),
				context: { type: 'service_listing', id: service.id }
			});
			sending = false;
			if (res.error || !res.data) {
				sendError = friendlyError(
					res.error?.code,
					res.error?.message ??
						t('messages.new_conv_error', 'Could not start the conversation.')
				);
				return;
			}
			open = false;
			body = '';
			await goto(`/messages?c=${res.data.conversation_id}`);
			return;
		}
		const res = await api.post<{ booking: { id: string } }>(
			`/services/${service.id}/bookings`,
			{ intent: 'inquiry', note: body.trim() }
		);
		sending = false;
		if (res.error || !res.data) {
			sendError = friendlyError(
				res.error?.code,
				res.error?.message ?? t('services.message_send_error', 'Could not send your message.')
			);
			return;
		}
		sent = true;
	}
</script>

<Modal bind:open title={t('services.message_title', 'Message {name}', { name: recipientName })}>
	{#if !authed}
		<p class="text-sm text-[var(--color-muted)]">
			<a href={loginHref} class="text-[var(--color-primary)] hover:underline">
				{t('services.message_login', 'Log in to send a message')}
			</a>
		</p>
	{:else if sent}
		<p
			class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-4 py-3 text-sm text-[var(--color-text)]"
		>
			{t(
				'services.message_inquiry_sent',
				'Your inquiry was sent — the provider will respond from their bookings inbox.'
			)}
		</p>
	{:else}
		<form onsubmit={send} class="flex flex-col gap-3">
			{#if dmChannel}
				<!-- Context chip — the recipient's inbox renders the same
				     listing as a structured card on the first message (000941). -->
				<p
					class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-xs text-[var(--color-text-subtle)]"
				>
					{t('services.message_context', 'Re: {title}', { title: service.title })}
				</p>
			{/if}
			<textarea
				bind:value={body}
				rows={4}
				maxlength={2000}
				placeholder={t('services.message_placeholder', 'Write your message…')}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
			></textarea>
			{#if sendError}
				<p role="alert" class="text-sm text-[var(--color-danger)]">{sendError}</p>
			{/if}
			<button
				type="submit"
				disabled={sending || !body.trim()}
				class="inline-flex items-center justify-center gap-1.5 self-end rounded-[var(--radius)] bg-[var(--color-primary)] px-5 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
			>
				{#if sending}
					<Loader2 size={15} class="motion-safe:animate-spin" aria-hidden="true" />
				{:else}
					<Send class="rtl:-scale-x-100" size={15} aria-hidden="true" />
				{/if}
				{t('services.message_send', 'Send message')}
			</button>
		</form>
	{/if}
</Modal>
