<script lang="ts">
	// Ref: japaconnect-engineering/sections/38-tickets.md (user portal)
	// Lists the user's submitted tickets with status/priority pills and links
	// to the single-ticket thread view. The "New ticket" CTA routes to the
	// full-page form at /dashboard/tickets/new.
	import { t } from '$lib/i18n';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { Ticket, Plus } from 'lucide-svelte';
	import type { TicketPriority, TicketStatus } from '$lib/types';

	let { data } = $props();
	const tickets = $derived(data.tickets);

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
</script>

<section class="space-y-4">
	<header class="flex items-center justify-between">
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('tickets.list_title')}</h1>
		<a
			href="/dashboard/tickets/new"
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-white transition hover:opacity-90"
		>
			<Plus class="h-4 w-4" />
			{t('tickets.new')}
		</a>
	</header>

	{#if tickets.length === 0}
		<EmptyState
			icon={Ticket}
			title={t('tickets.empty_title')}
			body={t('tickets.empty_body')}
		/>
	{:else}
		<ul class="space-y-2">
			{#each tickets as tk (tk.id)}
				<li>
					<a
						href="/dashboard/tickets/{tk.id}"
						class="block rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition hover:border-[var(--color-primary)]/40"
					>
						<div class="flex items-start justify-between gap-3">
							<div class="min-w-0 flex-1">
								<p class="truncate text-sm font-medium text-[var(--color-text)]">{tk.subject}</p>
								<p class="mt-0.5 text-xs text-[var(--color-muted)]">{tk.type_name}</p>
							</div>
							<span
								class="shrink-0 rounded-full px-2 py-0.5 text-xs font-medium {statusClass(tk.status)}"
							>
								{t(STATUS_KEY[tk.status])}
							</span>
						</div>
						<div class="mt-2 flex items-center gap-3 text-xs text-[var(--color-muted)]">
							<span>#{tk.id.slice(0, 8)}</span>
							<span>·</span>
							<span>{t(PRIORITY_KEY[tk.priority])}</span>
							<span>·</span>
							<time>{new Date(tk.created_at).toLocaleDateString()}</time>
						</div>
					</a>
				</li>
			{/each}
		</ul>
	{/if}
</section>
