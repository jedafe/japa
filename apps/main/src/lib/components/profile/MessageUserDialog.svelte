<script lang="ts">
	// "Message" on the profile — starts (or reuses) a DM conversation with
	// the member via POST /dm/conversations, then deep-links into
	// /messages?c={id}. Guests get the login prompt (MessageDialog pattern).
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { Loader2, Send } from 'lucide-svelte';
	import { Modal } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';

	interface Props {
		recipientId: string;
		recipientName: string;
		authed: boolean;
		open?: boolean;
	}
	let { recipientId, recipientName, authed, open = $bindable(false) }: Props = $props();

	const loginHref = $derived(
		`/login?redirect=${encodeURIComponent(page.url.pathname + page.url.search)}`
	);

	let body = $state('');
	let sending = $state(false);
	let sendError = $state('');

	async function send(e: SubmitEvent) {
		e.preventDefault();
		if (!body.trim() || sending) return;
		sendError = '';
		sending = true;
		const res = await api.post<{ conversation_id: string }>('/dm/conversations', {
			recipient_id: recipientId,
			message: body.trim()
		});
		sending = false;
		if (res.error || !res.data) {
			sendError =
				res.error?.message ?? t('messages.new_conv_error', 'Could not start the conversation.');
			return;
		}
		open = false;
		body = '';
		await goto(`/messages?c=${res.data.conversation_id}`);
	}
</script>

<Modal bind:open title={t('services.message_title', 'Message {name}', { name: recipientName })}>
	{#if !authed}
		<p class="text-sm text-[var(--color-muted)]">
			<a href={loginHref} class="text-[var(--color-primary)] hover:underline">
				{t('services.message_login', 'Log in to send a message')}
			</a>
		</p>
	{:else}
		<form onsubmit={send} class="flex flex-col gap-3">
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
