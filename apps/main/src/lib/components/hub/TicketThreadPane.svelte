<script lang="ts">
	// Support-ticket thread pane for the Communications Hub — subject + ref +
	// status/priority pills, the original description as first post, nested
	// comments, real file attachments (upload via the 2-step presigned flow
	// then POST /me/tickets/{id}/attachments) and a reply composer.
	//
	// Ref: japaconnect-engineering/sections/38-ticket-system.md
	import { untrack } from 'svelte';
	import { t } from '#lib/i18n/index.js';
	import { api } from '#lib/api/client.js';
	import { initiateUpload, uploadToPresigned, confirmMedia, getMedia } from '#lib/api/create.js';
	import { ticketRef } from '#lib/tickets.js';
	import { ticketStatusLabel, formatBytes } from './hub.js';
	import {
		ArrowLeft,
		Send,
		Loader2,
		Paperclip,
		FileText,
		PanelRight,
		MessageSquare
	} from 'lucide-svelte';
	import EmptyState from '#lib/components/ui/EmptyState.svelte';
	import type {
		TicketDetailResponse,
		TicketComment,
		TicketAttachment,
		MediaAsset,
		TicketStatus
	} from '#lib/types/index.js';

	interface Props {
		detail: TicketDetailResponse;
		onBack: () => void;
		onToggleDetails: () => void;
	}

	let { detail, onBack, onToggleDetails }: Props = $props();
	const ticket = $derived(detail.ticket);

	let comments = $state<TicketComment[]>(untrack(() => detail.comments ?? []));
	let attachments = $state<TicketAttachment[]>(untrack(() => detail.attachments ?? []));
	let replyBody = $state('');
	let posting = $state(false);
	let errorKey = $state('');

	// Attachment media resolution — rows carry only media_id; fetch
	// GET /media/{id} for filename/url/size (bounded by attachment count).
	let mediaById = $state<Map<string, MediaAsset>>(new Map());

	// File attach state — one pending file per reply, uploaded on submit.
	let pendingFile = $state<File | null>(null);
	let fileInputEl = $state<HTMLInputElement | null>(null);
	let uploading = $state(false);

	// Re-sync when the selected ticket changes (component reuse across sel=).
	$effect(() => {
		ticket.id;
		comments = untrack(() => detail.comments ?? []);
		attachments = untrack(() => detail.attachments ?? []);
		replyBody = '';
		pendingFile = null;
		errorKey = '';
	});

	// Resolve media details for every attachment row once the list lands.
	$effect(() => {
		const missing = attachments
			.map((a) => a.media_id)
			.filter((id) => id && !mediaById.has(id));
		if (missing.length === 0) return;
		for (const id of missing) {
			void getMedia(id).then((res) => {
				if (res.data) {
					const next = new Map(mediaById);
					next.set(id, res.data);
					mediaById = next;
				}
			});
		}
	});

	// Attachment rows not bound to a comment render as a ticket-level list.
	const looseAttachments = $derived(attachments.filter((a) => !a.comment_id));

	function statusClass(status: TicketStatus): string {
		switch (status) {
			case 'open':
				return 'bg-[var(--color-info)]/10 text-[var(--color-info)]';
			case 'in_progress':
				return 'bg-[var(--color-warning)]/10 text-[var(--color-warning)]';
			case 'waiting_on_user':
				return 'bg-[var(--color-primary)]/10 text-[var(--color-primary-text)]';
			case 'resolved':
			case 'closed':
				return 'bg-[var(--color-success)]/10 text-[var(--color-success)]';
			default:
				return 'bg-[var(--color-surface-subtle)] text-[var(--color-muted)]';
		}
	}

	function pickFile(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		pendingFile = input.files?.[0] ?? null;
	}

	async function postReply(e: SubmitEvent) {
		e.preventDefault();
		errorKey = '';
		if (!replyBody.trim() && !pendingFile) {
			errorKey = 'tickets.errors.reply_required';
			return;
		}
		posting = true;
		try {
			let commentId: string | undefined;
			if (replyBody.trim()) {
				const res = await api.post<TicketComment>(`/me/tickets/${ticket.id}/comments`, {
					body: replyBody.trim()
				});
				if (res.data) {
					comments = [...comments, res.data];
					commentId = res.data.id;
					replyBody = '';
				} else {
					errorKey = 'tickets.errors.reply_failed';
					return;
				}
			}

			if (pendingFile) {
				uploading = true;
				const file = pendingFile;
				const init = await initiateUpload({
					filename: file.name,
					mime_type: file.type || 'application/octet-stream',
					file_size: file.size,
					context: 'document'
				});
				const initData = init.data;
				if (!initData) {
					errorKey = 'hub.errors.upload_failed';
					return;
				}
				const put = await uploadToPresigned(initData.presigned_url, file);
				if (!put.ok) {
					errorKey = 'hub.errors.upload_failed';
					return;
				}
				await confirmMedia(initData.media_id);
				const att = await api.post<TicketAttachment>(
					`/me/tickets/${ticket.id}/attachments`,
					{ media_id: initData.media_id, comment_id: commentId ?? null }
				);
				uploading = false;
				if (att.data) {
					attachments = [...attachments, att.data];
				} else {
					errorKey = 'hub.errors.attach_failed';
					return;
				}
				pendingFile = null;
				if (fileInputEl) fileInputEl.value = '';
			}
		} finally {
			posting = false;
		}
	}
</script>

<div class="flex min-w-0 flex-1 flex-col">
	<!-- Thread header — ticket ref + type + status. -->
	<div class="flex items-center gap-3 border-b border-[var(--color-border)] px-4 py-3">
		<button
			type="button"
			onclick={onBack}
			class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] md:hidden"
			aria-label={t('messages.back', 'Back')}
		>
			<ArrowLeft class="rtl:-scale-x-100" size={18} />
		</button>
		<div class="min-w-0 flex-1">
			<p class="truncate text-sm font-medium text-[var(--color-text)]">{ticket.subject}</p>
			<p class="truncate text-xs text-[var(--color-muted)]">
				{ticketRef(ticket)} · {ticket.type_name}
			</p>
		</div>
		<span class="shrink-0 rounded-full px-2.5 py-0.5 text-xs font-medium {statusClass(ticket.status)}">
			{ticketStatusLabel(ticket.status, t)}
		</span>
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

	<!-- Thread body -->
	<div class="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
		<article
			class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
		>
			<p class="mb-2 text-xs font-medium text-[var(--color-muted)]">
				{t('tickets.original_post', 'Original request')}
			</p>
			<div class="whitespace-pre-wrap text-sm text-[var(--color-text)]">{ticket.description}</div>
		</article>

		{#if comments.length === 0}
			<EmptyState icon={MessageSquare} title={t('tickets.no_comments', 'No replies yet')} />
		{:else}
			<ul class="space-y-2">
				{#each comments as c (c.id)}
					{@const commentAttachments = attachments.filter((a) => a.comment_id === c.id)}
					<li
						class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
					>
						<div class="mb-1 flex items-center justify-between text-xs text-[var(--color-muted)]">
							<span>{new Date(c.created_at).toLocaleString()}</span>
						</div>
						<p class="whitespace-pre-wrap text-sm text-[var(--color-text)]">{c.body}</p>
						{#if commentAttachments.length > 0}
							<ul class="mt-2 space-y-1">
								{#each commentAttachments as a (a.id)}
									{@const media = mediaById.get(a.media_id)}
									<li>
										{#if media?.url}
											<a
												href={media.url}
												target="_blank"
												rel="noopener noreferrer"
												class="inline-flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-2.5 py-1.5 text-xs text-[var(--color-primary-text)] hover:border-[var(--color-primary)]"
											>
												<FileText size={13} aria-hidden="true" />
												<span class="truncate font-medium">{media.filename ?? 'Attachment'}</span>
												{#if media.file_size}
													<span class="text-[var(--color-muted)]">{formatBytes(media.file_size)}</span>
												{/if}
											</a>
										{:else}
											<span
												class="inline-flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-2.5 py-1.5 text-xs text-[var(--color-muted)]"
											>
												<Loader2 size={13} class="animate-spin" aria-hidden="true" />
												{t('common.loading', 'Loading…')}
											</span>
										{/if}
									</li>
								{/each}
							</ul>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}

		<!-- Ticket-level attachments (not linked to a comment). -->
		{#if looseAttachments.length > 0}
			<div>
				<p class="mb-1.5 text-xs font-medium text-[var(--color-muted)]">
					{t('hub.attachments', 'Attachments')}
				</p>
				<ul class="space-y-1">
					{#each looseAttachments as a (a.id)}
						{@const media = mediaById.get(a.media_id)}
						<li>
							{#if media?.url}
								<a
									href={media.url}
									target="_blank"
									rel="noopener noreferrer"
									class="inline-flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-2.5 py-1.5 text-xs text-[var(--color-primary-text)] hover:border-[var(--color-primary)]"
								>
									<FileText size={13} aria-hidden="true" />
									<span class="truncate font-medium">{media.filename ?? 'Attachment'}</span>
									{#if media.file_size}
										<span class="text-[var(--color-muted)]">{formatBytes(media.file_size)}</span>
									{/if}
								</a>
							{/if}
						</li>
					{/each}
				</ul>
			</div>
		{/if}
	</div>

	<!-- Reply composer + attachment pick -->
	{#if ticket.status !== 'closed' && ticket.status !== 'spam'}
		<form
			class="space-y-2 border-t border-[var(--color-border)] p-3"
			onsubmit={postReply}
			novalidate
		>
			{#if pendingFile}
				<div
					class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-1.5 text-xs text-[var(--color-text)]"
				>
					<Paperclip size={13} aria-hidden="true" />
					<span class="truncate">{pendingFile.name}</span>
					<span class="text-[var(--color-muted)]">{formatBytes(pendingFile.size)}</span>
					<button
						type="button"
						onclick={() => {
							pendingFile = null;
							if (fileInputEl) fileInputEl.value = '';
						}}
						class="ms-auto text-[var(--color-muted)] hover:text-[var(--color-danger)]"
						aria-label={t('hub.remove_attachment', 'Remove attachment')}
					>
						×
					</button>
				</div>
			{/if}
			<div class="flex items-center gap-2">
				<input
					bind:this={fileInputEl}
					type="file"
					class="hidden"
					onchange={pickFile}
					aria-label={t('hub.attach_file', 'Attach a file')}
				/>
				<button
					type="button"
					onclick={() => fileInputEl?.click()}
					class="grid h-9 w-9 shrink-0 place-items-center rounded-full border border-[var(--color-border)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
					aria-label={t('hub.attach_file', 'Attach a file')}
					title={t('hub.attach_file', 'Attach a file')}
				>
					<Paperclip size={16} />
				</button>
				<input
					type="text"
					value={replyBody}
					oninput={(e) => (replyBody = e.currentTarget.value)}
					placeholder={t('tickets.reply_label', 'Write a reply…')}
					class="w-full rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
				/>
				<button
					type="submit"
					disabled={posting || uploading || (!replyBody.trim() && !pendingFile)}
					class="grid h-9 w-9 shrink-0 place-items-center rounded-full bg-[var(--color-primary)] text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
					aria-label={t('tickets.reply_submit', 'Send reply')}
				>
					{#if posting || uploading}
						<Loader2 size={16} class="animate-spin" />
					{:else}
						<Send class="rtl:-scale-x-100" size={16} />
					{/if}
				</button>
			</div>
			{#if errorKey}
				<p class="text-sm text-[var(--color-danger)]" role="alert">{t(errorKey)}</p>
			{/if}
		</form>
	{:else}
		<p
			class="border-t border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-4 py-3 text-sm text-[var(--color-muted)]"
		>
			{t('tickets.closed_no_reply', 'This ticket is closed — replies are off.')}
		</p>
	{/if}
</div>
