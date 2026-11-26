<script lang="ts">
	// DM request pane for the Communications Hub — the requester's preview
	// plus accept / decline / block actions (POST /dm/requests/{id}/{action}).
	// Accept returns the conversation_id and the page switches the stream
	// selection to the new DM thread.
	//
	// Ref: japaconnect-engineering/sections/48-direct-messages.md
	import { t } from '#lib/i18n/index.js';
	import { api } from '#lib/api/client.js';
	import { ArrowLeft, Loader2, PanelRight } from 'lucide-svelte';
	import VerifiedBadge from '#lib/components/ui/VerifiedBadge.svelte';
	import RelativeTime from '#lib/components/ui/RelativeTime.svelte';
	import { remoteImageUrl } from '#lib/utils/remoteImage.js';
	import { confirmDialog } from '@japaconnect/ui';
	import type { DMRequest } from '#lib/types/index.js';

	interface Props {
		request: DMRequest;
		onBack: () => void;
		onToggleDetails: () => void;
		onAccepted: (conversationId: string) => void;
		onResolved: (requestId: string) => void;
	}

	let { request, onBack, onToggleDetails, onAccepted, onResolved }: Props = $props();

	let acting = $state(false);
	let actionError = $state('');

	async function handleAction(action: 'accept' | 'decline' | 'block') {
		if (acting) return;
		// BLK-009: block is the only irreversible-flavoured request action —
		// confirm before firing (writes user_blocks, removes follows).
		if (
			action === 'block' &&
			!(await confirmDialog(
				t(
					'messages.requests_block_confirm',
					'Block this user? They will no longer be able to message you.'
				)
			))
		) {
			return;
		}
		actionError = '';
		acting = true;
		const res = await api.post<{ status: string; conversation_id?: string }>(
			`/dm/requests/${request.id}/${action}`
		);
		acting = false;
		if (res.error) {
			actionError =
				res.error.message ??
				t('messages.requests_action_error', 'The action could not be completed.');
			return;
		}
		const conversationId = res.data?.conversation_id ?? request.conversation_id;
		onResolved(request.id);
		if (action === 'accept' && conversationId) {
			onAccepted(conversationId);
		}
	}
</script>

<div class="flex min-w-0 flex-1 flex-col">
	<div class="flex items-center gap-3 border-b border-[var(--color-border)] px-4 py-3">
		<button
			type="button"
			onclick={onBack}
			class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] md:hidden"
			aria-label={t('messages.back', 'Back')}
		>
			<ArrowLeft class="rtl:-scale-x-100" size={18} />
		</button>
		{#if request.requester_avatar_url}
			<img
				src={remoteImageUrl(request.requester_avatar_url)}
				alt=""
				class="h-9 w-9 rounded-full object-cover"
			/>
		{:else}
			<span
				class="grid h-9 w-9 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]"
				>{(request.requester_display_name ?? '?').charAt(0).toUpperCase()}</span
			>
		{/if}
		<div class="min-w-0 flex-1">
			<p class="flex items-center gap-1 truncate text-sm font-medium text-[var(--color-text)]">
				<span class="truncate">{request.requester_display_name}</span>
				<VerifiedBadge
					verified={request.requester_verified}
					type={request.requester_verification_type}
					size={14}
				/>
			</p>
			<a
				href="/u/{request.requester_username}"
				class="truncate text-xs text-[var(--color-muted)] hover:text-[var(--color-primary-text)]"
				>@{request.requester_username}</a
			>
		</div>
		<button
			type="button"
			onclick={onToggleDetails}
			class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
			aria-label={t('hub.details_toggle', 'Conversation details')}
			title={t('hub.details_toggle', 'Conversation details')}
		>
			<PanelRight size={16} />
		</button>
	</div>

	<div class="min-h-0 flex-1 overflow-y-auto p-4">
		<p class="mb-2 text-xs text-[var(--color-muted)]">
			{t('hub.request_received', 'Message request')} · <RelativeTime iso={request.created_at} />
		</p>
		<article
			class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
		>
			<p class="whitespace-pre-wrap text-sm text-[var(--color-text)]">{request.preview_message}</p>
		</article>
		<p class="mt-3 text-xs text-[var(--color-muted)]">
			{t(
				'hub.request_hint',
				'Accept to start chatting, decline to dismiss, or block to stop further requests.'
			)}
		</p>
	</div>

	<div class="space-y-2 border-t border-[var(--color-border)] p-3">
		{#if actionError}
			<p class="text-sm text-[var(--color-danger)]" role="alert">{actionError}</p>
		{/if}
		<div class="flex gap-2">
			<button
				type="button"
				onclick={() => handleAction('accept')}
				disabled={acting}
				class="inline-flex flex-1 items-center justify-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
			>
				{#if acting}<Loader2 size={14} class="animate-spin" />{/if}
				{t('messages.requests_accept', 'Accept')}
			</button>
			<button
				type="button"
				onclick={() => handleAction('decline')}
				disabled={acting}
				class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
			>
				{t('messages.requests_decline', 'Decline')}
			</button>
			<button
				type="button"
				onclick={() => handleAction('block')}
				disabled={acting}
				class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm font-medium text-[var(--color-danger)] hover:bg-[var(--color-danger)]/10 disabled:opacity-50"
			>
				{t('messages.requests_block', 'Block')}
			</button>
		</div>
	</div>
</div>
