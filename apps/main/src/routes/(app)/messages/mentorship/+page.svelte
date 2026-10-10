<script lang="ts">
	// Ref: japaconnect-engineering/sections/102-mentorship-matching.md (000824)
	// Two-pane mentorship inbox — kind=mentorship service_bookings rows for
	// both roles. Sent = the caller's mentee requests; received = requests to
	// the caller as mentor (accept/decline/complete in the detail pane).
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { GraduationCap } from 'lucide-svelte';
	import { EmptyState, ListPageBar } from '$lib/components/ui';
	import MessagesTabs from '$lib/components/messages/MessagesTabs.svelte';
	import BookingListPane from '$lib/components/messages/BookingListPane.svelte';
	import BookingDetailPane from '$lib/components/messages/BookingDetailPane.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const basePath = '/messages/mentorship';
	const title = $derived(t('messages.mentorship_title', 'Mentorship — JapaConnect'));

	const features = $derived((page.data.features ?? {}) as Record<string, boolean>);
	const dmEnabled = $derived(!!features['feature.dm']);
	const packagesEnabled = $derived(!!features['service_packages.enabled']);
	// viewerId is the root layout's dedicated viewer id — immune to page-level
	// `user` overrides.
	const viewerId = $derived(page.data.viewerId ?? '');
	const selectedId = $derived(page.url.searchParams.get('b'));

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
			? undefined
			: { label: t('mentors.browse', 'Browse mentors'), href: '/services?kind=mentorship' }
	);

	function closeDetail() {
		const sp = new URLSearchParams(page.url.searchParams);
		sp.delete('b');
		const qs = sp.toString();
		void goto(`${basePath}${qs ? `?${qs}` : ''}`, { noScroll: true });
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div
	class="mx-auto flex w-full max-w-full flex-col overflow-hidden h-[calc(100dvh-8.5rem-env(safe-area-inset-top)-env(safe-area-inset-bottom))] md:h-dvh"
>
	<ListPageBar>
		<MessagesTabs value="mentorship" />
	</ListPageBar>

	<div class="flex min-h-0 flex-1">
		<aside
			class="w-full shrink-0 flex-col border-r border-[var(--color-border)] sm:flex sm:max-w-sm"
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
					{dmEnabled}
					{packagesEnabled}
					onClose={closeDetail}
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
	</div>
</div>
