<script lang="ts">
	// Ref: japaconnect-engineering/sections/111-service-packages-bookings.md
	//
	// Left pane of the /messages/bookings + /messages/mentorship inboxes —
	// role sub-filter (sent|received → API booker|provider), status pills,
	// booking rows (listing title, status chip, counterpart, time). Row
	// selection writes ?b=<id> which the server load resolves to the detail
	// pane.
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { Inbox } from 'lucide-svelte';
	import { EmptyState, PaginationControls, RelativeTime } from '$lib/components/ui';
	import type { ListMeta, ServiceBooking } from '$lib/types';
	import {
		bookingCounterpart,
		bookingStatus,
		bookingStatusColor,
		bookingStatusLabel
	} from './bookingDisplay';

	interface Props {
		bookings: ServiceBooking[];
		/** Currently selected booking id (?b=) — highlighted row. */
		activeId: string | null;
		/** Route base — /messages/bookings or /messages/mentorship. */
		basePath: string;
		role: 'sent' | 'received';
		status: string;
		meta?: ListMeta;
		currentPage: number;
		loadError: boolean;
		emptyTitle: string;
		emptyBody: string;
		emptyAction?: { label: string; href: string };
	}

	let {
		bookings,
		activeId,
		basePath,
		role,
		status,
		meta,
		currentPage,
		loadError,
		emptyTitle,
		emptyBody,
		emptyAction
	}: Props = $props();

	const roleFilters = [
		{ id: 'sent', label: t('bookings.role_sent', 'Sent') },
		{ id: 'received', label: t('bookings.role_received', 'Received') }
	];

	// §111 booking statuses (plus the mentorship aliases surface through
	// connection_status on rows — the filter itself uses raw statuses).
	const statusFilters = [
		{ id: '', label: t('bookings.filter_all', 'All') },
		{ id: 'pending', label: t('bookings.status_pending', 'Pending') },
		{ id: 'confirmed', label: t('bookings.status_confirmed', 'Confirmed') },
		{ id: 'fulfilled', label: t('bookings.status_fulfilled', 'Fulfilled') },
		{ id: 'cancelled', label: t('bookings.status_cancelled', 'Cancelled') },
		{ id: 'declined', label: t('bookings.status_declined', 'Declined') },
		{ id: 'disputed', label: t('bookings.status_disputed', 'Disputed') },
		{ id: 'refunded', label: t('bookings.status_refunded', 'Refunded') }
	];

	/** Filter href — changing role/status resets the page + selection. */
	function filterHref(overrides: Record<string, string>): string {
		const sp = new URLSearchParams(page.url.searchParams);
		for (const [k, v] of Object.entries(overrides)) {
			if (v) sp.set(k, v);
			else sp.delete(k);
		}
		sp.delete('page');
		sp.delete('b');
		const qs = sp.toString();
		return `${basePath}${qs ? `?${qs}` : ''}`;
	}

	/** Row href — selects the booking in the detail pane (?b=), keeping the
	 *  current list filters so closing the detail returns to the same list. */
	function rowHref(id: string): string {
		const sp = new URLSearchParams(page.url.searchParams);
		sp.set('b', id);
		return `${basePath}?${sp.toString()}`;
	}

	function onPageChange(p: number) {
		const sp = new URLSearchParams(page.url.searchParams);
		if (p > 1) sp.set('page', String(p));
		else sp.delete('page');
		const qs = sp.toString();
		void goto(`${basePath}${qs ? `?${qs}` : ''}`, { noScroll: true });
	}
</script>

<div class="space-y-2 border-b border-[var(--color-border)] px-3 py-3">
	<!-- Role sub-filter: sent = bookings the user made; received = bookings
	     the user provides (mentorship requests). -->
	<div class="flex gap-1.5" role="group" aria-label={t('bookings.filter_role', 'Direction')}>
		{#each roleFilters as f (f.id)}
			<a
				href={filterHref({ role: f.id === 'sent' ? '' : f.id })}
				aria-current={role === f.id ? 'true' : undefined}
				class="flex-1 rounded-full border px-3 py-1 text-center text-xs font-medium transition-colors {role ===
				f.id
					? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
					: 'border-[var(--color-border)] text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
			>{f.label}</a>
		{/each}
	</div>
	<div class="flex flex-wrap items-center gap-1.5">
		{#each statusFilters as f (f.id)}
			<a
				href={filterHref({ status: f.id })}
				aria-current={status === f.id ? 'true' : undefined}
				class="rounded-full border px-2.5 py-1 text-xs font-medium transition-colors {status === f.id
					? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
					: 'border-[var(--color-border)] text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
			>{f.label}</a>
		{/each}
	</div>
</div>

{#if loadError}
	<div class="p-4">
		<EmptyState icon={Inbox} title={t('bookings.load_error', 'Could not load bookings')} />
	</div>
{:else if bookings.length === 0}
	<div class="p-4">
		<EmptyState icon={Inbox} title={emptyTitle} body={emptyBody} action={emptyAction} />
	</div>
{:else}
	<ul class="min-h-0 flex-1 overflow-y-auto">
		{#each bookings as b (b.id)}
			{@const active = b.id === activeId}
			{@const counterpart = bookingCounterpart(b, role)}
			<li>
				<a
					href={rowHref(b.id)}
					aria-current={active ? 'true' : undefined}
					class="flex w-full items-center gap-3 border-b border-[var(--color-border)] px-4 py-3 text-left transition-colors {active
						? 'bg-[var(--color-surface-subtle)]'
						: 'hover:bg-[var(--color-surface-subtle)]'}"
				>
					{#if counterpart.avatar}
						<img
							src={counterpart.avatar}
							alt=""
							class="h-10 w-10 shrink-0 rounded-full object-cover"
						/>
					{:else}
						<span
							class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]"
						>{(counterpart.name || '?').charAt(0).toUpperCase()}</span>
					{/if}
					<span class="min-w-0 flex-1">
						<span class="flex items-center justify-between gap-2">
							<span class="truncate text-sm font-medium text-[var(--color-text)]">
								{b.listing_title || t('bookings.untitled', 'Untitled listing')}
							</span>
							<span
								class="shrink-0 text-xs font-medium capitalize {bookingStatusColor(
									bookingStatus(b)
								)}"
							>{bookingStatusLabel(b)}</span>
						</span>
						<span class="flex items-center justify-between gap-2">
							<span class="truncate text-xs text-[var(--color-muted)]">
								{counterpart.name || b.listing_slug}
							</span>
							<span class="shrink-0 text-xs text-[var(--color-muted)]">
								<RelativeTime iso={b.created_at} />
							</span>
						</span>
					</span>
				</a>
			</li>
		{/each}
	</ul>

	{#if meta?.total && meta.total > (meta.limit ?? 20)}
		<div
			class="flex items-center justify-between gap-2 border-t border-[var(--color-border)] px-3 py-2"
		>
			<PaginationControls
				page={currentPage}
				totalPages={Math.ceil((meta.total ?? 0) / (meta.limit ?? 20))}
				{onPageChange}
			/>
			<span class="text-xs text-[var(--color-muted)]">{meta.total}</span>
		</div>
	{/if}
{/if}
