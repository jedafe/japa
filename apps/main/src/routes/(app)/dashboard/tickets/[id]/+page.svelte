<script lang="ts">
	// Ref: japaconnect-engineering/sections/38-tickets.md (user portal)
	// Single-ticket thread view: header (subject, status, priority, dates) +
	// the original description as the first post + nested comments. The reply
	// box posts to /me/tickets/{id}/comments via the browser API client and
	// prepends the new comment optimistically.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { ticketRef } from '$lib/tickets';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { MessageSquare, Send } from 'lucide-svelte';
	import { untrack } from 'svelte';
	import type { TicketComment, TicketPriority, TicketStatus } from '$lib/types';

	let { data } = $props();
	const detail = $derived(data.detail);
	const ticket = $derived(detail.ticket);

	let comments = $state<TicketComment[]>(untrack(() => data.detail.comments ?? []));
	let replyBody = $state('');
	let posting = $state(false);
	let errorKey = $state('');

	// SvelteKit reuses this component when navigating ticket→ticket, so a
	// one-time seed would show the previous ticket's thread. Re-sync whenever
	// the load delivers a new detail payload; locally-posted replies only
	// touch `comments` and don't retrigger this.
	$effect(() => {
		comments = data.detail.comments ?? [];
		replyBody = '';
		errorKey = '';
	});

	const STATUS_KEY: Record<TicketStatus, string> = {
		open: 'tickets.status.open',
		in_progress: 'tickets.status.in_progress',
		waiting_on_user: 'tickets.status.waiting_on_user',
		resolved: 'tickets.status.resolved',
		closed: 'tickets.status.closed',
		spam: 'tickets.status.spam'
	};

	const PRIORITY_KEY: Record<TicketPriority, string> = {
		low: 'tickets.priority.low',
		normal: 'tickets.priority.normal',
		high: 'tickets.priority.high',
		urgent: 'tickets.priority.urgent'
	};

	function statusClass(status: TicketStatus): string {
		switch (status) {
			case 'open':
				return 'bg-[var(--color-info)]/10 text-[var(--color-info)]';
			case 'in_progress':
				return 'bg-[var(--color-warning)]/10 text-[var(--color-warning)]';
			case 'waiting_on_user':
				return 'bg-[var(--color-primary)]/10 text-[var(--color-primary)]';
			case 'resolved':
			case 'closed':
				return 'bg-[var(--color-success)]/10 text-[var(--color-success)]';
			default:
				return 'bg-[var(--color-surface-subtle)] text-[var(--color-muted)]';
		}
	}

	async function postReply(e: SubmitEvent) {
		e.preventDefault();
		errorKey = '';
		if (!replyBody.trim()) {
			errorKey = 'tickets.errors.reply_required';
			return;
		}
		posting = true;
		try {
			const res = await api.post<TicketComment>(
				`/me/tickets/${ticket.id}/comments`,
				{ body: replyBody.trim() }
			);
			if (res.data) {
				comments = [...comments, res.data];
				replyBody = '';
			} else {
				errorKey = 'tickets.errors.reply_failed';
			}
		} catch {
			errorKey = 'tickets.errors.reply_failed';
		} finally {
			posting = false;
		}
	}
</script>

<section class="space-y-4">
	<header>
		<a href="/dashboard/tickets" class="text-sm text-[var(--color-muted)] hover:underline">
			← {t('tickets.back_to_list')}
		</a>
		<div class="mt-2 flex flex-wrap items-start justify-between gap-3">
			<div class="min-w-0">
				<h1 class="text-lg font-medium text-[var(--color-text)]">{ticket.subject}</h1>
				<p class="mt-0.5 text-xs text-[var(--color-muted)]">
					{ticketRef(ticket)} · {ticket.type_name}
				</p>
			</div>
			<span class="shrink-0 rounded-full px-2.5 py-0.5 text-xs font-medium {statusClass(ticket.status)}">
				{t(STATUS_KEY[ticket.status])}
			</span>
		</div>
		<div class="mt-2 flex flex-wrap items-center gap-3 text-xs text-[var(--color-muted)]">
			<span>{t(PRIORITY_KEY[ticket.priority])}</span>
			<span>·</span>
			<span>{t('tickets.created')} {new Date(ticket.created_at).toLocaleString()}</span>
			{#if ticket.resolved_at}
				<span>·</span>
				<span>{t('tickets.resolved')} {new Date(ticket.resolved_at).toLocaleString()}</span>
			{/if}
		</div>
	</header>

	<!-- Original description as the first post -->
	<article class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<p class="mb-2 text-xs font-medium text-[var(--color-muted)]">{t('tickets.original_post')}</p>
		<div class="whitespace-pre-wrap text-sm text-[var(--color-text)]">{ticket.description}</div>
	</article>

	<!-- Comments thread -->
	<div class="space-y-3">
		<h2 class="flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
			<MessageSquare class="h-4 w-4" />
			{t('tickets.comments')} ({comments.length})
		</h2>

		{#if comments.length === 0}
			<EmptyState icon={MessageSquare} title={t('tickets.no_comments')} />
		{:else}
			<ul class="space-y-2">
				{#each comments as c (c.id)}
					<li class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
						<div class="mb-1 flex items-center justify-between text-xs text-[var(--color-muted)]">
							<span>{new Date(c.created_at).toLocaleString()}</span>
						</div>
						<p class="whitespace-pre-wrap text-sm text-[var(--color-text)]">{c.body}</p>
					</li>
				{/each}
			</ul>
		{/if}
	</div>

	<!-- Reply box -->
	{#if ticket.status !== 'closed' && ticket.status !== 'spam'}
		<form onsubmit={postReply} class="space-y-2 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3" novalidate>
			<label for="ticket-reply" class="text-sm font-medium text-[var(--color-text)]">
				{t('tickets.reply_label')}
			</label>
			<textarea
				id="ticket-reply"
				value={replyBody}
				oninput={(e) => (replyBody = e.currentTarget.value)}
				rows={4}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
			></textarea>
			{#if errorKey}
				<p class="text-sm text-[var(--color-danger)]" role="alert">{t(errorKey)}</p>
			{/if}
			<div class="flex justify-end">
				<button
					type="submit"
					disabled={posting || !replyBody.trim()}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-white transition hover:opacity-90 disabled:opacity-50"
				>
					<Send class="h-4 w-4" />
					{posting ? t('common.loading') : t('tickets.reply_submit')}
				</button>
			</div>
		</form>
	{:else}
		<p class="rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm text-[var(--color-muted)]">
			{t('tickets.closed_no_reply')}
		</p>
	{/if}
</section>
