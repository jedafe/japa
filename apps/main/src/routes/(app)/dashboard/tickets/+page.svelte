<script lang="ts">
	// Ref: japaconnect-engineering/sections/38-tickets.md (user portal)
	// Lists the user's submitted tickets with status/priority pills and links
	// to the single-ticket thread view. The "New ticket" CTA routes to the
	// full-page form at /dashboard/tickets/new.
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t } from '$lib/i18n';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import PaginationControls from '$lib/components/ui/PaginationControls.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import { Ticket, Plus } from 'lucide-svelte';
	import type { TicketPriority, TicketStatus } from '$lib/types';

	let { data } = $props();
	const tickets = $derived(data.tickets);
	const statusFilter = $derived(data.status);
	const pageNum = $derived(data.page);
	const pageSize = $derived(data.pageSize);
	const hasMore = $derived(data.hasMore);
	const showPagination = $derived(pageNum > 1 || hasMore);

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

	const statusOptions = $derived([
		{ value: '', label: t('tickets.status.all', 'All statuses') },
		{ value: 'open', label: t('tickets.status.open') },
		{ value: 'in_progress', label: t('tickets.status.in_progress') },
		{ value: 'waiting_on_user', label: t('tickets.status.waiting_on_user') },
		{ value: 'resolved', label: t('tickets.status.resolved') },
		{ value: 'closed', label: t('tickets.status.closed') }
	]);

	const pageSizeOptions = [
		{ value: '10', label: '10' },
		{ value: '25', label: '25' },
		{ value: '50', label: '50' }
	];

	function buildURL(overrides: Record<string, string | null>) {
		const url = new URL(page.url);
		for (const [key, value] of Object.entries(overrides)) {
			if (value === null || value === '') url.searchParams.delete(key);
			else url.searchParams.set(key, value);
		}
		return url.toString();
	}

	function changeStatus(v: string) {
		goto(buildURL({ status: v || null, page: null }));
	}
	function changePageSize(v: string) {
		goto(buildURL({ page_size: v, page: null }));
	}
	function changePage(next: number) {
		goto(buildURL({ page: String(next) }));
	}

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
	<header class="flex flex-wrap items-center justify-between gap-3">
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('tickets.list_title')}</h1>
		<div class="flex flex-wrap items-center gap-2">
			<AutocompleteSelect
				class="w-44"
				value={statusFilter}
				options={statusOptions}
				ariaLabel={t('tickets.filter_status', 'Filter by status')}
				onValueChange={changeStatus}
			/>
			<AutocompleteSelect
				class="w-20"
				value={String(pageSize)}
				options={pageSizeOptions}
				ariaLabel={t('tickets.page_size', 'Rows per page')}
				onValueChange={changePageSize}
			/>
			<a
				href="/dashboard/tickets/new"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-white transition hover:opacity-90"
			>
				<Plus class="h-4 w-4" />
				{t('tickets.new')}
			</a>
		</div>
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
		{#if showPagination}
			<div class="flex justify-center pt-2">
				<PaginationControls page={pageNum} {hasMore} onPageChange={changePage} />
			</div>
		{/if}
	{/if}
</section>
