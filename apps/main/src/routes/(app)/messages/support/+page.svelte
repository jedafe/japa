<script lang="ts">
	import SeoHead from '#lib/seo/SeoHead.svelte';
	// Support inbox at /messages/support — the whole member support surface
	// inside the Messages tab family. Two-pane workbench like
	// /messages/bookings: a Tickets|Appeals toggle swaps the list between
	// tickets (thread pane carries replies + attachments via
	// TicketThreadPane) and appeals (eligible decisions + submitted appeals,
	// ?a=<id> opens the appeal detail; its linked ticket jumps back to the
	// tickets view).
	//
	// Ref: japaconnect-engineering/sections/38-ticket-system.md,
	//      japaconnect-engineering/sections/22-admin-panel.md (Appeals)
	import { t } from '#lib/i18n/index.js';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { AlertTriangle, LifeBuoy, Plus, Scale } from 'lucide-svelte';
	import {
		EmptyState,
		ListPageBar,
		AutocompleteSelect,
		PaginationControls,
		RelativeTime
	} from '#lib/components/ui/index.js';
	import MessagesTabs from '#lib/components/messages/MessagesTabs.svelte';
	import HubListItem from '#lib/components/hub/HubListItem.svelte';
	import TicketThreadPane from '#lib/components/hub/TicketThreadPane.svelte';
	import AppealButton from '#lib/components/appeals/AppealButton.svelte';
	import { ticketToHubItem, type HubItem } from '#lib/components/hub/hub.js';
	import { ticketHref } from '#lib/tickets.js';
	import type { AppealListItem } from '#lib/types/index.js';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const basePath = '/messages/support';
	const title = $derived(t('messages.support_title', 'Support — {site}'));
	const view = $derived(data.view);
	const selectedId = $derived(page.url.searchParams.get('t'));
	const selectedAppealId = $derived(page.url.searchParams.get('a'));

	const viewFilters = [
		{ id: 'tickets', label: t('tickets.list_title', 'Tickets') },
		{ id: 'appeals', label: t('appeals.list_title', 'Appeals') }
	];

	const statusOptions = $derived([
		{ value: '', label: t('tickets.status.all', 'All statuses') },
		{ value: 'open', label: t('tickets.status.open') },
		{ value: 'in_progress', label: t('tickets.status.in_progress') },
		{ value: 'waiting_on_user', label: t('tickets.status.waiting_on_user') },
		{ value: 'resolved', label: t('tickets.status.resolved') },
		{ value: 'closed', label: t('tickets.status.closed') }
	]);

	const items = $derived(data.tickets.map((tk) => ticketToHubItem(tk, t)));

	// Appeals — same eligible-drop-on-submit bookkeeping as the dashboard
	// surface.
	let submittedIds = $state(new Set<string>());
	const eligible = $derived(data.eligible.filter((i) => !submittedIds.has(i.entity_id)));
	const selectedAppeal = $derived(data.selectedAppeal);
	const restricted = $derived(view === 'appeals' && data.userStatus !== 'active');

	const APPEAL_STATUS_KEY: Record<AppealListItem['status'], string> = {
		pending: 'appeals.status.pending',
		upheld: 'appeals.status.upheld',
		overturn: 'appeals.status.overturn',
		dismissed: 'appeals.status.dismissed'
	};

	const ENTITY_KEY: Record<string, string> = {
		content: 'appeals.entity.content',
		content_removal: 'appeals.entity.content_removal',
		comment: 'appeals.entity.comment',
		user: 'appeals.entity.user',
		ban: 'appeals.entity.ban',
		suspension: 'appeals.entity.suspension',
		warning: 'appeals.entity.warning',
		operation_ban: 'appeals.entity.operation_ban',
		service_claim: 'appeals.entity.service_claim',
		org_claim: 'appeals.entity.org_claim',
		video_channel_claim: 'appeals.entity.video_channel_claim',
		media: 'appeals.entity.media',
		wiki_revision: 'appeals.entity.wiki_revision',
		content_report: 'appeals.entity.content_report',
		scam_report: 'appeals.entity.scam_report',
		academy_instructor: 'appeals.entity.academy_instructor',
		academy_track: 'appeals.entity.academy_track',
		review: 'appeals.entity.review',
		mutual_aid_post: 'appeals.entity.mutual_aid_post',
		video_source: 'appeals.entity.video_source',
		roadmap_item: 'appeals.entity.roadmap_item',
		organization: 'appeals.entity.organization',
		space: 'appeals.entity.space',
		affiliate_member: 'appeals.entity.affiliate_member',
		referral: 'appeals.entity.referral',
		withdrawal: 'appeals.entity.withdrawal',
		ad_campaign: 'appeals.entity.ad_campaign',
		content_review: 'appeals.entity.content_review',
		community_org: 'appeals.entity.community_org',
		community_org_claim: 'appeals.entity.community_org_claim',
		verification_request: 'appeals.entity.verification_request'
	};

	function appealStatusClass(status: AppealListItem['status']): string {
		switch (status) {
			case 'pending':
				return 'bg-[var(--color-warning)]/10 text-[var(--color-warning)]';
			case 'overturn':
				return 'bg-[var(--color-success)]/10 text-[var(--color-success)]';
			default:
				return 'bg-[var(--color-surface-subtle)] text-[var(--color-muted)]';
		}
	}

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
		goto(hrefFor(view === 'appeals' ? { a: null } : { t: null }), { reset: false });
	}

	const showPagination = $derived(data.page > 1 || data.hasMore);
	const paneOpen = $derived(view === 'appeals' ? !!selectedAppealId : !!selectedId);
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
				href="/messages/support/new"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				<Plus size={14} />
				{t('tickets.new')}
			</a>
		{/snippet}
	</ListPageBar>

	<div class="flex min-h-0 flex-1">
		<!-- List pane -->
		<aside
			class="w-full shrink-0 flex-col overflow-y-auto border-e border-[var(--color-border)] sm:flex sm:max-w-sm"
			class:flex={!paneOpen}
			class:hidden={paneOpen}
		>
			<!-- Tickets | Appeals — same 50/50 segmented row as the
			     Sent/Received chips on the bookings inboxes. -->
			<div class="space-y-2 border-b border-[var(--color-border)] px-3 py-2.5">
				<div class="flex gap-1.5" role="group" aria-label={t('messages.support_view', 'Support inbox')}>
					{#each viewFilters as f (f.id)}
						<a
							href={hrefFor({
								view: f.id === 'tickets' ? null : f.id,
								page: null,
								status: null,
								t: null,
								a: null
							})}
							aria-current={view === f.id ? 'true' : undefined}
							class="flex-1 rounded-full border px-3 py-1 text-center text-xs font-medium transition-colors {view ===
							f.id
								? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary-text)]'
								: 'border-[var(--color-border)] text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
						>{f.label}</a>
					{/each}
				</div>
				{#if view === 'tickets'}
					<AutocompleteSelect
						class="w-full"
						options={statusOptions}
						value={data.status}
						onValueChange={(v) => goto(hrefFor({ status: v || null, page: null, t: null }), { reset: false })}
						ariaLabel={t('tickets.filter_status', 'Filter by status')}
					/>
				{/if}
			</div>

			{#if view === 'tickets'}
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
					<ul>
						{#each items as item (item.sel)}
							<HubListItem {item} active={item.ticket!.id === selectedId} onSelect={selectItem} />
						{/each}
					</ul>
				{/if}
			{:else}
				<!-- Appeals view -->
				{#if restricted}
					<div
						class="m-3 flex items-start gap-3 rounded-[var(--radius-card)] border border-[var(--color-danger)]/40 bg-[var(--color-danger)]/10 p-3"
						role="alert"
					>
						<AlertTriangle size={16} class="mt-0.5 shrink-0 text-[var(--color-danger)]" aria-hidden="true" />
						<div class="min-w-0">
							<p class="text-sm font-medium text-[var(--color-text)]">
								{t(`appeals.restricted.${data.userStatus}`, 'Your account is restricted')}
							</p>
							<p class="mt-0.5 text-xs text-[var(--color-muted)]">
								{t(
									'appeals.restricted_body',
									'Most of {site} is unavailable while this restriction is active. You can contest it below — a staff member will review your appeal.'
								)}
							</p>
						</div>
					</div>
				{/if}

				{#if eligible.length > 0}
					<div class="border-b border-[var(--color-border)] px-3 py-3">
						<h2 class="text-xs font-medium uppercase tracking-wider text-[var(--color-muted)]">
							{t('appeals.eligible_title', 'Decisions you can appeal')}
						</h2>
						<ul class="mt-2 space-y-2">
							{#each eligible as item (item.entity_type + item.entity_id)}
								<li class="flex items-center justify-between gap-2">
									<div class="min-w-0">
										<p class="truncate text-sm text-[var(--color-text)]">{item.label}</p>
										{#if item.detail}
											<p class="mt-0.5 line-clamp-1 text-xs text-[var(--color-muted)]">{item.detail}</p>
										{/if}
									</div>
									<AppealButton
										entityType={item.entity_type}
										entityId={item.entity_id}
										subject={item.label}
										class="shrink-0 !px-3 !py-1.5"
										onSubmitted={() => submittedIds.add(item.entity_id)}
									/>
								</li>
							{/each}
						</ul>
					</div>
				{/if}

				{#if data.appeals.length === 0}
					<div class="p-4">
						<EmptyState
							icon={Scale}
							title={t('appeals.empty_title', 'No appeals')}
							body={t(
								'appeals.empty_body',
								'Appeals you submit against rejected claims or moderation decisions will appear here.'
							)}
						/>
					</div>
				{:else}
					<ul>
						{#each data.appeals as appeal (appeal.id)}
							{@const active = appeal.id === selectedAppealId}
							<li>
								<a
									href={hrefFor({ a: appeal.id })}
									aria-current={active ? 'true' : undefined}
									class="flex w-full items-center gap-3 border-b border-[var(--color-border)] px-4 py-3 text-start transition-colors {active
										? 'bg-[var(--color-surface-subtle)]'
										: 'hover:bg-[var(--color-surface-subtle)]'}"
								>
									<span
										class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]"
									>
										<Scale size={16} aria-hidden="true" />
									</span>
									<span class="min-w-0 flex-1">
										<span class="flex items-center justify-between gap-2">
											<span class="truncate text-sm font-medium text-[var(--color-text)]">
												{appeal.label ||
													t(ENTITY_KEY[appeal.entity_type] ?? 'appeals.entity.other', appeal.entity_type)}
											</span>
											<span
												class="shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium {appealStatusClass(
													appeal.status
												)}"
											>{t(APPEAL_STATUS_KEY[appeal.status])}</span>
										</span>
										<span class="flex items-center justify-between gap-2">
											<span class="truncate text-xs text-[var(--color-muted)]">{appeal.reason}</span>
											<span class="shrink-0 text-xs text-[var(--color-muted)]">
												<RelativeTime iso={appeal.created_at} />
											</span>
										</span>
									</span>
								</a>
							</li>
						{/each}
					</ul>
				{/if}
			{/if}

			{#if showPagination}
				<div
					class="mt-auto flex items-center justify-center gap-2 border-t border-[var(--color-border)] px-3 py-2"
				>
					<PaginationControls
						page={data.page}
						hasMore={data.hasMore}
						onPageChange={(p) =>
							goto(hrefFor({ page: String(p), t: null, a: null }), { reset: false })}
					/>
				</div>
			{/if}
		</aside>

		<!-- Detail pane -->
		<section
			class="min-w-0 flex-1 flex-col overflow-y-auto sm:flex"
			class:flex={paneOpen}
			class:hidden={!paneOpen}
		>
			{#if view === 'tickets'}
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
			{:else if selectedAppeal}
				<div class="p-4 sm:p-6">
					<button
						type="button"
						onclick={closeDetail}
						class="mb-3 text-sm text-[var(--color-muted)] hover:underline sm:hidden"
					>← {t('messages.back', 'Back')}</button>
					<div
						class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
					>
						<div class="flex items-start justify-between gap-3">
							<div class="min-w-0 flex-1">
								<p class="text-sm font-medium text-[var(--color-text)]">
									{selectedAppeal.label ||
										t(ENTITY_KEY[selectedAppeal.entity_type] ?? 'appeals.entity.other', selectedAppeal.entity_type)}
								</p>
								{#if selectedAppeal.label}
									<p class="mt-0.5 text-xs text-[var(--color-muted)]">
										{t(ENTITY_KEY[selectedAppeal.entity_type] ?? 'appeals.entity.other', selectedAppeal.entity_type)}
									</p>
								{/if}
							</div>
							<span
								class="shrink-0 rounded-full px-2 py-0.5 text-xs font-medium {appealStatusClass(
									selectedAppeal.status
								)}"
							>{t(APPEAL_STATUS_KEY[selectedAppeal.status])}</span>
						</div>
						<div class="mt-3 space-y-3">
							<div>
								<p class="text-xs font-medium uppercase tracking-wider text-[var(--color-muted)]">
									{t('appeals.reason_label', 'Your reason')}
								</p>
								<p class="mt-1 whitespace-pre-wrap text-sm text-[var(--color-text)]">{selectedAppeal.reason}</p>
							</div>
							{#if selectedAppeal.resolution_note}
								<div>
									<p class="text-xs font-medium uppercase tracking-wider text-[var(--color-muted)]">
										{t('appeals.resolution', 'Resolution')}
									</p>
									<p class="mt-1 whitespace-pre-wrap text-sm text-[var(--color-text-subtle)]">
										{selectedAppeal.resolution_note}
									</p>
								</div>
							{/if}
						</div>
						<div class="mt-3 flex items-center gap-3 text-xs text-[var(--color-muted)]">
							<span>#{selectedAppeal.id.slice(0, 8)}</span>
							<span>·</span>
							<RelativeTime iso={selectedAppeal.created_at} />
						</div>
						{#if selectedAppeal.ticket_id}
							<a
								href={ticketHref({
									id: selectedAppeal.ticket_id,
									ticket_number: selectedAppeal.ticket_number
								})}
								class="mt-4 inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
							>
								{t('appeals.view_ticket', 'View ticket')}
							</a>
						{/if}
					</div>
				</div>
			{:else if selectedAppealId}
				<div class="grid flex-1 place-items-center p-8">
					<EmptyState
						icon={Scale}
						title={t('appeals.not_found', 'Appeal not found')}
						action={{ label: t('messages.back', 'Back'), href: hrefFor({ a: null }) }}
					/>
				</div>
			{:else}
				<div class="grid flex-1 place-items-center p-8">
					<EmptyState
						icon={Scale}
						title={t('appeals.select_appeal', 'Select an appeal')}
					/>
				</div>
			{/if}
		</section>
	</div>
</div>
