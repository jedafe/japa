<script lang="ts">
	// Ref: japaconnect-engineering/sections/111-service-packages-bookings.md
	// Two-pane bookings inbox — left list (BookingListPane) selects ?b=<id>,
	// right detail (BookingDetailPane) carries the booker/provider actions.
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { Package } from 'lucide-svelte';
	import { EmptyState, ListPageBar } from '$lib/components/ui';
	import MessagesTabs from '$lib/components/messages/MessagesTabs.svelte';
	import BookingListPane from '$lib/components/messages/BookingListPane.svelte';
	import BookingDetailPane from '$lib/components/messages/BookingDetailPane.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const basePath = '/messages/bookings';
	const title = $derived(t('messages.bookings_title', 'Bookings — JapaConnect'));

	const features = $derived((page.data.features ?? {}) as Record<string, boolean>);
	const dmEnabled = $derived(!!features['feature.dm']);
	const packagesEnabled = $derived(!!features['service_packages.enabled']);
	// viewerId is the root layout's dedicated viewer id — immune to page-level
	// `user` overrides.
	const viewerId = $derived(page.data.viewerId ?? '');
	const viewerEmail = $derived(page.data.user?.email ?? '');
	const selectedId = $derived(page.url.searchParams.get('b'));

	// Empty-state copy per role; the browse action prefers the
	// mentorship kind when that flag is on, else the directory.
	const emptyTitle = $derived(
		data.role === 'received'
			? t('bookings.empty_received', 'Nothing received yet')
			: t('bookings.empty', 'No bookings yet')
	);
	const emptyBody = $derived(
		data.role === 'received'
			? t(
					'bookings.empty_received_body',
					'Mentorship requests sent to you land here.'
				)
			: t(
					'bookings.empty_body',
					'Browse services to send an inquiry or book a session.'
				)
	);
	const emptyAction = $derived(
		data.role === 'received'
			? { label: t('mentors.browse', 'Browse mentors'), href: '/services?kind=mentorship' }
			: features['mentorship.enabled']
				? { label: t('mentors.browse', 'Browse mentors'), href: '/services?kind=mentorship' }
				: { label: t('bookings.browse', 'Browse services'), href: '/services' }
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

<!-- Exact-height column like /messages so the panes scroll internally;
     below md the shell's top bar + bottom tab bar are subtracted. -->
<div
	class="mx-auto flex w-full max-w-full flex-col overflow-hidden h-[calc(100dvh-8.5rem-env(safe-area-inset-top)-env(safe-area-inset-bottom))] md:h-dvh"
>
	<ListPageBar>
		<MessagesTabs value="bookings" />
	</ListPageBar>

	<div class="flex min-h-0 flex-1">
		<!-- Booking list -->
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
				intent={data.intent}
				meta={data.meta}
				currentPage={data.currentPage}
				loadError={data.loadError}
				{emptyTitle}
				{emptyBody}
				{emptyAction}
			/>
		</aside>

		<!-- Booking detail -->
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
				/>
			{:else if data.selectedMissing}
				<div class="grid flex-1 place-items-center p-8">
					<EmptyState
						icon={Package}
						title={t('bookings.not_found', 'Booking not found')}
						action={{ label: t('messages.back', 'Back'), href: basePath }}
					/>
				</div>
			{:else}
				<div class="grid flex-1 place-items-center p-8">
					<EmptyState
						icon={Package}
						title={t('bookings.select_booking', 'Select a booking')}
					/>
				</div>
			{/if}
		</section>
	</div>
</div>
