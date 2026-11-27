<script lang="ts">
	// Conversation-details rail for the Communications Hub — adapts to the
	// selected item's kind: counterpart card (DMs/requests/bookings), ticket
	// status/priority/assignment, shared files (resolved ticket attachments),
	// activity timestamps, and deep links (profile, full ticket page, Japa
	// AI, new-ticket CTA).
	//
	// No assignment/tags API exists on the member side (assignment lives on
	// staff tickets) — this rail displays the real fields each kind exposes.
	import { t } from '#lib/i18n/index.js';
	import { api } from '#lib/api/client.js';
	import { getMedia } from '#lib/api/create.js';
	import { ticketHref } from '#lib/tickets.js';
	import {
		ticketStatusLabel,
		ticketPriorityLabel,
		bookingStatusLabel,
		formatBytes
	} from './hub.js';
	import { bookingCounterpart, bookingStatus } from '../messages/bookingDisplay.js';
	import VerifiedBadge from '#lib/components/ui/VerifiedBadge.svelte';
	import RelativeTime from '#lib/components/ui/RelativeTime.svelte';
	import { remoteImageUrl } from '#lib/utils/remoteImage.js';
	import {
		User,
		FileText,
		ExternalLink,
		Sparkles,
		Headset,
		LifeBuoy
	} from 'lucide-svelte';
	import type { MediaAsset, PublicUser, TicketDetailResponse } from '#lib/types/index.js';
	import type { HubItem } from './hub.js';

	interface Props {
		item: HubItem;
		/** Full ticket detail for the open ticket (kind=ticket) — supplies
		 *  assigned_to + the attachment rows for Shared files. */
		ticketDetail?: TicketDetailResponse | null;
	}

	let { item, ticketDetail = null }: Props = $props();

	const attachmentMediaIds = $derived(
		(ticketDetail?.attachments ?? []).map((a) => a.media_id).filter(Boolean)
	);

	// Resolve shared-file media details for ticket attachments. Failed
	// lookups (deleted media, permission) are hidden rather than left as
	// eternal "Loading…" rows; mediaInFlight dedupes effect re-runs while
	// a fetch is still pending.
	let mediaById = $state<Map<string, MediaAsset>>(new Map());
	let mediaFailed = $state<Set<string>>(new Set());
	const mediaInFlight = new Set<string>();
	$effect(() => {
		const missing = attachmentMediaIds.filter(
			(id) => id && !mediaById.has(id) && !mediaFailed.has(id) && !mediaInFlight.has(id)
		);
		for (const id of missing) {
			mediaInFlight.add(id);
			void getMedia(id).then((res) => {
				mediaInFlight.delete(id);
				if (res.data) {
					const next = new Map(mediaById);
					next.set(id, res.data);
					mediaById = next;
				} else {
					mediaFailed = new Set(mediaFailed).add(id);
				}
			});
		}
	});
	const visibleAttachmentIds = $derived(
		attachmentMediaIds.filter((id) => !mediaFailed.has(id))
	);

	const dm = $derived(item.dm);
	const request = $derived(item.request);
	const booking = $derived(item.booking);
	const ticket = $derived(item.ticket);
	const ai = $derived(item.ai);

	const counterpart = $derived(
		booking ? bookingCounterpart(booking, item.bookingRole) : null
	);

	const profileHref = $derived(
		dm
			? dm.conversation_type === 'org'
				? `/org/${dm.other_username}`
				: `/u/${dm.other_username}`
			: request
				? `/u/${request.requester_username}`
				: counterpart?.username
					? `/u/${counterpart.username}`
					: ''
	);

	// The user-profile counterpart username (org DMs and entities without a
	// member profile are skipped) — feeds the member-since fetch.
	const profileUsername = $derived(
		dm
			? dm.conversation_type !== 'org'
				? dm.other_username
				: ''
			: request
				? request.requester_username
				: counterpart?.username ?? ''
	);

	// Member-since (mockup: "Member since Mar 2023") — the counterpart's
	// account creation date from GET /users/{username} (PublicUser).
	let memberSince = $state('');
	$effect(() => {
		const u = profileUsername;
		memberSince = '';
		if (!u) return;
		void api
			.get<PublicUser>(`/users/${encodeURIComponent(u)}`)
			.then((res) => {
				// Compare against the CURRENT derived — the item may have
				// changed while the fetch was in flight.
				if (profileUsername !== u || !res.data?.created_at) return;
				memberSince = new Date(res.data.created_at).toLocaleDateString(undefined, {
					month: 'short',
					year: 'numeric'
				});
			})
			.catch(() => {
				// Non-critical metadata — a failed fetch just hides the line.
			});
	});
</script>

<aside class="flex h-full w-full flex-col overflow-y-auto">
	<!-- Member / entity card -->
	<div class="border-b border-[var(--color-border)] p-4 text-center">
		{#if item.avatarUrl}
			<img
				src={remoteImageUrl(item.avatarUrl)}
				alt=""
				class="mx-auto h-16 w-16 rounded-full object-cover"
			/>
		{:else}
			<span
				class="mx-auto grid h-16 w-16 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]"
			>
				{#if item.avatarIcon === 'headset'}
					<Headset size={26} aria-hidden="true" />
				{:else if item.avatarIcon === 'sparkles'}
					<Sparkles size={26} aria-hidden="true" />
				{:else}
					<User size={26} aria-hidden="true" />
				{/if}
			</span>
		{/if}
		<p class="mt-2 flex items-center justify-center gap-1 text-sm font-medium text-[var(--color-text)]">
			{item.title}
			<VerifiedBadge
				verified={item.verified}
				type={item.verificationType}
				scope={item.verifiedScope}
				size={14}
			/>
		</p>
		{#if item.subtitle}
			<p class="truncate text-xs text-[var(--color-muted)]">{item.subtitle}</p>
		{/if}
		{#if memberSince}
			<p class="mt-0.5 text-xs text-[var(--color-muted)]">
				{t('hub.member_since', 'Member since {date}', { date: memberSince })}
			</p>
		{/if}
		{#if profileHref}
			<a
				href={profileHref}
				class="mt-2 inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
			>
				<ExternalLink size={12} aria-hidden="true" />
				{t('hub.view_profile', 'View profile')}
			</a>
		{/if}
	</div>

	<!-- Status section — ticket status/priority/assignment or booking status. -->
	{#if ticket}
		<div class="space-y-2 border-b border-[var(--color-border)] p-4">
			<p class="text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
				{t('hub.details_ticket', 'Ticket')}
			</p>
			<dl class="space-y-1.5 text-sm">
				<div class="flex justify-between gap-2">
					<dt class="text-[var(--color-muted)]">{t('hub.field_status', 'Status')}</dt>
					<dd class="font-medium text-[var(--color-text)]">
						{ticketStatusLabel(ticket.status, t)}
					</dd>
				</div>
				<div class="flex justify-between gap-2">
					<dt class="text-[var(--color-muted)]">{t('hub.field_priority', 'Priority')}</dt>
					<dd class="font-medium text-[var(--color-text)]">
						{ticketPriorityLabel(ticket.priority, t)}
					</dd>
				</div>
				<div class="flex justify-between gap-2">
					<dt class="text-[var(--color-muted)]">{t('hub.field_type', 'Type')}</dt>
					<dd class="font-medium text-[var(--color-text)]">{ticket.type_name}</dd>
				</div>
				<div class="flex justify-between gap-2">
					<dt class="text-[var(--color-muted)]">{t('hub.assigned_to', 'Assigned to')}</dt>
					<dd class="font-medium text-[var(--color-text)]">
						{ticketDetail?.ticket.assigned_to
							? t('hub.assigned_support', 'Support team')
							: t('hub.assigned_queue', 'Unassigned queue')}
					</dd>
				</div>
			</dl>
			<a
				href={ticketHref(ticket)}
				class="mt-1 inline-flex items-center gap-1 text-xs font-medium text-[var(--color-primary-text)] hover:underline"
			>
				{t('hub.open_ticket_page', 'Open ticket page')}
				<ExternalLink size={12} aria-hidden="true" />
			</a>
		</div>
	{:else if booking}
		<div class="space-y-2 border-b border-[var(--color-border)] p-4">
			<p class="text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
				{item.kind === 'mentorship'
					? t('hub.details_mentorship', 'Mentorship')
					: t('hub.details_booking', 'Booking')}
			</p>
			<dl class="space-y-1.5 text-sm">
				<div class="flex justify-between gap-2">
					<dt class="text-[var(--color-muted)]">{t('hub.field_status', 'Status')}</dt>
					<dd class="font-medium text-[var(--color-text)]">
						{bookingStatusLabel(bookingStatus(booking), t)}
					</dd>
				</div>
				<div class="flex justify-between gap-2">
					<dt class="text-[var(--color-muted)]">{t('hub.details_listing', 'Listing')}</dt>
					<dd class="truncate font-medium text-[var(--color-text)]">{booking.listing_title}</dd>
				</div>
				{#if booking.departure_starts_on}
					<div class="flex justify-between gap-2">
						<dt class="text-[var(--color-muted)]">{t('hub.details_dates', 'Dates')}</dt>
						<dd class="font-medium text-[var(--color-text)]">
							{new Date(booking.departure_starts_on).toLocaleDateString()}
							{#if booking.departure_ends_on}
								– {new Date(booking.departure_ends_on).toLocaleDateString()}{/if}
						</dd>
					</div>
				{/if}
			</dl>
		</div>
	{:else if item.kind === 'ai'}
		<div class="space-y-2 border-b border-[var(--color-border)] p-4">
			<p class="text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
				{t('hub.details_ai', 'AI conversation')}
			</p>
			<p class="text-sm text-[var(--color-muted)]">
				{t('hub.details_ai_hint', 'Tools, projects and streaming live on the full AI surface.')}
			</p>
			{#if ai}
				<a
					href="/ai?c={ai.conversation_id}"
					class="inline-flex items-center gap-1 text-xs font-medium text-[var(--color-primary-text)] hover:underline"
				>
					{t('hub.ai_open_full', 'Open in Japa AI')}
					<ExternalLink size={12} aria-hidden="true" />
				</a>
			{/if}
		</div>
	{/if}

	<!-- Shared files — ticket attachments resolved through GET /media/{id}. -->
	{#if visibleAttachmentIds.length > 0}
		<div class="border-b border-[var(--color-border)] p-4">
			<p class="mb-2 text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
				{t('hub.shared_files', 'Shared files')}
			</p>
			<ul class="space-y-1.5">
				{#each visibleAttachmentIds as mid (mid)}
					{@const media = mediaById.get(mid)}
					<li>
						{#if media?.url}
							<a
								href={media.url}
								target="_blank"
								rel="noopener noreferrer"
								class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-2.5 py-1.5 text-xs text-[var(--color-primary-text)] hover:border-[var(--color-primary)]"
							>
								<FileText size={13} class="shrink-0" aria-hidden="true" />
								<span class="min-w-0 truncate font-medium">{media.filename ?? 'Attachment'}</span>
								{#if media.file_size}
									<span class="ms-auto shrink-0 text-[var(--color-muted)]"
										>{formatBytes(media.file_size)}</span
									>
								{/if}
							</a>
						{:else}
							<span
								class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-2.5 py-1.5 text-xs text-[var(--color-muted)]"
							>
								<FileText size={13} class="shrink-0" aria-hidden="true" />
								{t('common.loading', 'Loading…')}
							</span>
						{/if}
					</li>
				{/each}
			</ul>
		</div>
	{/if}

	<!-- Activity — the real timestamps each kind exposes. -->
	<div class="border-b border-[var(--color-border)] p-4">
		<p class="mb-2 text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
			{t('hub.activity', 'Activity')}
		</p>
		<ul class="space-y-1.5 text-xs text-[var(--color-muted)]">
			{#if ticket}
				<li class="flex justify-between gap-2">
					<span>{t('tickets.created', 'Created')}</span>
					<span><RelativeTime iso={ticket.created_at} /></span>
				</li>
				<li class="flex justify-between gap-2">
					<span>{t('hub.updated', 'Updated')}</span>
					<span><RelativeTime iso={ticket.updated_at} /></span>
				</li>
				{#if ticket.resolved_at}
					<li class="flex justify-between gap-2">
						<span>{t('tickets.resolved', 'Resolved')}</span>
						<span><RelativeTime iso={ticket.resolved_at} /></span>
					</li>
				{/if}
			{:else if booking}
				<li class="flex justify-between gap-2">
					<span>{t('hub.updated', 'Updated')}</span>
					<span><RelativeTime iso={item.at ?? booking.created_at} /></span>
				</li>
			{:else if dm}
				{#if dm.last_message_at}
					<li class="flex justify-between gap-2">
						<span>{t('hub.last_message', 'Last message')}</span>
						<span><RelativeTime iso={dm.last_message_at} /></span>
					</li>
				{/if}
				<li class="flex justify-between gap-2">
					<span>{t('tickets.created', 'Created')}</span>
					<span><RelativeTime iso={dm.created_at} /></span>
				</li>
			{:else if request}
				<li class="flex justify-between gap-2">
					<span>{t('hub.request_received', 'Message request')}</span>
					<span><RelativeTime iso={request.created_at} /></span>
				</li>
			{:else if ai}
				<li class="flex justify-between gap-2">
					<span>{t('hub.updated', 'Updated')}</span>
					<span><RelativeTime iso={item.at ?? ''} /></span>
				</li>
			{/if}
		</ul>
	</div>

	<!-- Support CTA — real route, no fabricated endpoint. -->
	<div class="p-4">
		<a
			href="/messages/support?new=1"
			class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
		>
			<LifeBuoy size={14} aria-hidden="true" />
			{t('hub.new_ticket', 'Open a support ticket')}
		</a>
	</div>
</aside>
