<script lang="ts">
	// Call-request dialog — JapaConnect has no voice/video call API yet, so
	// instead of a dead button the dialog proposes a call by posting a real
	// message into the open DM thread (POST /dm/conversations/{id}/messages).
	import { t } from '#lib/i18n/index.js';
	import { api } from '#lib/api/client.js';
	import { Modal } from '#lib/components/ui/index.js';
	import { Phone, Video } from 'lucide-svelte';
	import type { DMConversation } from '#lib/types/index.js';

	interface Props {
		open: boolean;
		conversation: DMConversation;
		kind: 'voice' | 'video';
		/** Fired after the proposal message lands so the pane can append it. */
		onSent: (body: string) => void;
	}

	let { open = $bindable(), conversation, kind, onSent }: Props = $props();

	let note = $state('');
	let sending = $state(false);
	let sendError = $state('');

	const kindLabel = $derived(
		kind === 'video' ? t('hub.call_video', 'Request a video call') : t('hub.call_voice', 'Request a voice call')
	);

	async function sendProposal() {
		if (sending) return;
		sending = true;
		sendError = '';
		const proposal =
			kind === 'video'
				? t('hub.call_proposal_video', 'Would you be up for a video call?')
				: t('hub.call_proposal_voice', 'Would you be up for a voice call?');
		const body = note.trim() ? `${proposal} ${note.trim()}` : proposal;
		const res = await api.post(`/dm/conversations/${conversation.id}/messages`, { body });
		sending = false;
		if (res.error) {
			sendError = res.error.message ?? t('hub.call_send_error', 'Could not send the request.');
			return;
		}
		onSent(body);
		open = false;
		note = '';
	}
</script>

<Modal bind:open title={kindLabel} size="sm" zIndex={60}>
	<div class="space-y-3">
		<p class="flex items-start gap-2 text-sm text-[var(--color-muted)]">
			{#if kind === 'video'}
				<Video size={16} class="mt-0.5 shrink-0" aria-hidden="true" />
			{:else}
				<Phone size={16} class="mt-0.5 shrink-0" aria-hidden="true" />
			{/if}
			{t(
				'hub.call_explainer',
				'Calls are not live in Japa yet — this sends a call request as a message so you can arrange a time in the thread.'
			)}
		</p>
		<input
			type="text"
			bind:value={note}
			placeholder={t('hub.call_note_placeholder', 'Add a note (optional) — e.g. tomorrow 3pm')}
			class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
		/>
		{#if sendError}
			<p class="text-sm text-[var(--color-danger)]" role="alert">{sendError}</p>
		{/if}
		<div class="flex justify-end gap-2">
			<button
				type="button"
				onclick={() => (open = false)}
				class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
			>
				{t('common.cancel', 'Cancel')}
			</button>
			<button
				type="button"
				onclick={sendProposal}
				disabled={sending}
				class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
			>
				{t('hub.call_send', 'Send request')}
			</button>
		</div>
	</div>
</Modal>
