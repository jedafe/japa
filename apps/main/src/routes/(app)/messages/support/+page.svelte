<script lang="ts">
	import SeoHead from '#lib/seo/SeoHead.svelte';
	// Support inbox at /messages/support — the tickets surface inside the
	// Messages tab family. Two-pane workbench like /messages/bookings:
	// list (status filter + pagination) selects ?t=<id>, right thread pane
	// carries replies + attachments via the shared TicketThreadPane.
	//
	// Ref: japaconnect-engineering/sections/38-ticket-system.md
	import { t } from '#lib/i18n/index.js';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { LifeBuoy, Plus } from 'lucide-svelte';
	import {
		EmptyState,
		ListPageBar,
		AutocompleteSelect,
		PaginationControls
	} from '#lib/components/ui/index.js';
	import MessagesTabs from '#lib/components/messages/MessagesTabs.svelte';
	import HubListItem from '#lib/components/hub/HubListItem.svelte';
	import TicketThreadPane from '#lib/components/hub/TicketThreadPane.svelte';
	import { ticketToHubItem, type HubItem } from '#lib/components/hub/hub.js';
	import type { TicketStatus } from '#lib/types/index.js';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const basePath = '/messages/support';
	const title = $derived(t('messages.support_title', 'Support — {site}'));
	const selectedId = $derived(page.url.searchParams.get('t'));

	const statusOptions = $derived([
		{ value: '', label: t('tickets.status.all', 'All statuses') },
		{ value: 'open', label: t('tickets.status.open') },
		{ value: 'in_progress', label: t('tickets.status.in_progress') },
		{ value: 'waiting_on_user', label: t('tickets.status.waiting_on_user') },
		{ value: 'resolved', label: t('tickets.status.resolved') },
		{ value: 'closed', label: t('tickets.status.closed') }
	]);

	const items = $derived(data.tickets.map((tk) => ticketToHubItem(tk, t)));

	function hrefFor(overrides: Record<string, string | null>): string {
		const params = new URLSearchParams(page.url.search);
		for (const [key, value] of Object.entries(overrides)) {
			if (value === null || value === '') params.delete(key);
			else params.set(key, value);
		}
		const qs = params.toString();
		return `${basePath}${qs ? `?${qs}` : ''}`;
	}

	function selectItem(item: HubItem) {
		goto(hrefFor({ t: item.ticket!.id }), { reset: false });
	}

	function closeDetail() {
		goto(hrefFor({ t: null }), { reset: false });
	}

	const showPagination = $derived(data.page > 1 || data.hasMore);
</script>

<SeoHead
	title={title}
	robots="noindex, nofollow"
	description={t('messages.support_description', 'Your support tickets and replies.')}
/>

<!-- Exact-height column like /messages so the panes scroll internally;
     below md the shell's top bar + bottom tab bar are subtracted. -->
<div
	class="mx-auto flex w-full max-w-full flex-col overflow-hidden h-[calc(100dvh-8.5rem-env(safe-area-inset-top)-env(safe-area-inset-bottom))] md:h-dvh"
>
	<ListPageBar>
		<MessagesTabs value="support" />
		{#snippet actions()}
			<a
				href="/dashboard/tickets/new"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				<Plus size={14} />
				{t('tickets.new')}
			</a>
		{/snippet}
	</ListPageBar>

	<div class="flex min-h-0 flex-1">
		<!-- Ticket list -->
		<aside
			class="w-full shrink-0 flex-col border-e border-[var(--color-border)] sm:flex sm:max-w-sm"
			class:flex={!selectedId}
			class:hidden={!!selectedId}
		>
			<div class="border-b border-[var(--color-border)] px-3 py-2.5">
				<AutocompleteSelect
					class="w-full"
					options={statusOptions}
					value={data.status}
					onValueChange={(v) => goto(hrefFor({ status: v || null, page: null, t: null }), { reset: false })}
					ariaLabel={t('tickets.filter_status', 'Filter by status')}
				/>
			</div>

			{#if data.loadError}
				<div class="p-4">
					<EmptyState
						icon={LifeBuoy}
						title={t('messages.load_error', 'Could not load conversations')}
					/>
				</div>
			{:else if items.length === 0}
				<div class="p-4">
					<EmptyState
						icon={LifeBuoy}
						title={t('tickets.empty_title')}
						body={t('tickets.empty_body')}
					/>
				</div>
			{:else}
				<ul class="min-h-0 flex-1 overflow-y-auto">
					{#each items as item (item.sel)}
						<HubListItem {item} active={item.ticket!.id === selectedId} onSelect={selectItem} />
					{/each}
				</ul>

				{#if showPagination}
					<div
						class="flex items-center justify-center gap-2 border-t border-[var(--color-border)] px-3 py-2"
					>
						<PaginationControls
							page={data.page}
							hasMore={data.hasMore}
							onPageChange={(p) => goto(hrefFor({ page: String(p), t: null }), { reset: false })}
						/>
					</div>
				{/if}
			{/if}
		</aside>

		<!-- Ticket thread -->
		<section
			class="min-w-0 flex-1 flex-col overflow-y-auto sm:flex"
			class:flex={!!selectedId}
			class:hidden={!selectedId}
		>
			{#if data.selected}
				<TicketThreadPane detail={data.selected} onBack={closeDetail} />
			{:else if data.selectedMissing}
				<div class="grid flex-1 place-items-center p-8">
					<EmptyState
						icon={LifeBuoy}
						title={t('tickets.not_found', 'Ticket not found')}
						action={{ label: t('messages.back', 'Back'), href: basePath }}
					/>
				</div>
			{:else}
				<div class="grid flex-1 place-items-center p-8">
					<EmptyState
						icon={LifeBuoy}
						title={t('hub.select_item', 'Select a conversation')}
					/>
				</div>
			{/if}
		</section>
	</div>
</div>
