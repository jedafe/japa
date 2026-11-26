<script lang="ts">
	import SeoHead from '#lib/seo/SeoHead.svelte';
	// Ref: japaconnect-engineering/sections/102-mentorship-matching.md (000824)
	// Two-pane mentorship inbox — kind=mentorship service_bookings rows for
	// both roles. Sent = the caller's mentee requests; received = requests to
	// the caller as mentor (accept/decline/complete in the detail pane).
	import { t } from '#lib/i18n/index.js';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { GraduationCap } from 'lucide-svelte';
	import { EmptyState, ListPageBar, Modal } from '#lib/components/ui/index.js';
	import MessagesTabs from '#lib/components/messages/MessagesTabs.svelte';
	import BookingListPane from '#lib/components/messages/BookingListPane.svelte';
	import BookingDetailPane from '#lib/components/messages/BookingDetailPane.svelte';
	import ConversationDetailsPanel from '#lib/components/hub/ConversationDetailsPanel.svelte';
	import { bookingToHubItem } from '#lib/components/hub/hub.js';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const basePath = '/messages/mentorship';
	const title = $derived(t('messages.mentorship_title', 'Mentorship — {site}'));

	const features = $derived((page.data.features ?? {}) as Record<string, boolean>);
	const dmEnabled = $derived(!!features['feature.dm']);
	const packagesEnabled = $derived(!!features['service_packages.enabled']);
	// viewerId is the root layout's dedicated viewer id — immune to page-level
	// `user` overrides.
	const viewerId = $derived(page.data.viewerId ?? '');
	const viewerEmail = $derived(page.data.user?.email ?? '');
	const selectedId = $derived(page.url.searchParams.get('b'));

	// Details rail — same behavior as /messages: a third column on lg+ that
	// re-opens on every selection, and a drawer modal on narrower screens
	// opened via the pane's PanelRight toggle.
	let detailsOpen = $state(true);
	let mobileDetailsOpen = $state(false);
	let isNarrow = $state(false);
	let lastSelKey = '';
	$effect(() => {
		const key = selectedId ?? '';
		if (key !== lastSelKey) {
			lastSelKey = key;
			if (key) detailsOpen = true;
			mobileDetailsOpen = false;
		}
	});
	onMount(() => {
		const mq = matchMedia('(max-width: 1023.5px)');
		const sync = () => (isNarrow = mq.matches);
		sync();
		mq.addEventListener('change', sync);
		return () => mq.removeEventListener('change', sync);
	});
	function toggleDetails() {
		if (isNarrow) mobileDetailsOpen = !mobileDetailsOpen;
		else detailsOpen = !detailsOpen;
	}

	// Rail item — build the same HubItem the unified stream uses, with the
	// effective role derived from the booking (not the inbox tab) so a ?b=
	// deep link under the "wrong" role still shows the right counterpart.
	const itemRole = $derived<'sent' | 'received'>(
		data.selected?.provider_user_id === viewerId
			? 'received'
			: data.selected?.booker_user_id === viewerId
				? 'sent'
				: data.role
	);
	const selectedItem = $derived(
		data.selected ? bookingToHubItem(data.selected, itemRole) : null
	);

	const emptyTitle = $derived(
		data.role === 'received'
			? t('mentors.me_incoming_empty', 'No incoming requests')
			: t('mentors.me_outgoing_empty', 'No mentorship requests yet')
	);
	const emptyBody = $derived(
		data.role === 'received'
			? t(
					'mentors.me_incoming_empty_body',
					'When someone requests you as a mentor, it will appear here.'
				)
			: t(
					'mentors.me_outgoing_empty_body',
					'Browse mentors and send a request to get started.'
				)
	);
	const emptyAction = $derived(
		data.role === 'received'
			? { label: t('mentors.become_mentor', 'Become a mentor'), href: '/settings/mentorship' }
			: { label: t('mentors.browse', 'Browse mentors'), href: '/services?kind=mentorship' }
	);

	function closeDetail() {
		const sp = new URLSearchParams(page.url.search);
		sp.delete('b');
		const qs = sp.toString();
		void goto(`${basePath}${qs ? `?${qs}` : ''}`, { reset: false });
	}
</script>

<SeoHead title={title} robots="noindex, nofollow" description={t('messages.mentorship_description', 'Mentorship requests and conversations.')} />

<div
	class="mx-auto flex w-full max-w-full flex-col overflow-hidden h-[calc(100dvh-8.5rem-env(safe-area-inset-top)-env(safe-area-inset-bottom))] md:h-dvh"
	style="--page-gutter: 1rem"
>
	<ListPageBar>
		<MessagesTabs value="mentorship" />
	</ListPageBar>

	<div class="flex min-h-0 flex-1" style="margin-inline: var(--page-gutter)">
		<aside
			class="w-full shrink-0 flex-col border-e border-[var(--color-border)] sm:flex sm:max-w-sm"
			class:flex={!selectedId}
			class:hidden={!!selectedId}
		>
			<BookingListPane
				bookings={data.bookings}
				activeId={selectedId}
				{basePath}
				role={data.role}
				status={data.status}
				meta={data.meta}
				currentPage={data.currentPage}
				loadError={data.loadError}
				{emptyTitle}
				{emptyBody}
				{emptyAction}
			/>
		</aside>

		<section
			class="min-w-0 flex-1 flex-col overflow-y-auto sm:flex"
			class:flex={!!selectedId}
			class:hidden={!selectedId}
		>
			{#if data.selected}
				<BookingDetailPane
					booking={data.selected}
					role={data.role}
					{viewerId}
					{viewerEmail}
					{dmEnabled}
					{packagesEnabled}
					onClose={closeDetail}
					onToggleDetails={toggleDetails}
				/>
			{:else if data.selectedMissing}
				<div class="grid flex-1 place-items-center p-8">
					<EmptyState
						icon={GraduationCap}
						title={t('bookings.not_found', 'Booking not found')}
						action={{ label: t('messages.back', 'Back'), href: basePath }}
					/>
				</div>
			{:else}
				<div class="grid flex-1 place-items-center p-8">
					<EmptyState
						icon={GraduationCap}
						title={t('bookings.select_booking', 'Select a booking')}
					/>
				</div>
			{/if}
		</section>

		<!-- Details rail — third column on lg+ like /messages. -->
		{#if selectedItem && detailsOpen}
			<div class="hidden w-72 shrink-0 border-s border-[var(--color-border)] lg:block">
				<ConversationDetailsPanel item={selectedItem} />
			</div>
		{/if}
	</div>
</div>

<!-- Details drawer for smaller viewports — the lg+ rail is inline above. -->
{#if isNarrow && selectedItem}
	<Modal
		bind:open={mobileDetailsOpen}
		title={t('hub.details_toggle', 'Conversation details')}
		size="md"
		zIndex={50}
	>
		<ConversationDetailsPanel item={selectedItem} />
	</Modal>
{/if}
