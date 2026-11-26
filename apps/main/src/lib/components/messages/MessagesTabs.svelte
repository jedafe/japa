<script lang="ts">
	// Ref: japaconnect-engineering/sections/48-direct-messages.md,
	// sections/111-service-packages-bookings.md, sections/102-mentorship-matching.md
	//
	// Shared Messages-section tab bar — Messages | Bookings | Mentorship |
	// Support — one tab per route. Each tab is hidden when its feature flag
	// is off (features map comes from the root layout's GET /feature-flags
	// payload):
	//   inbox          → feature.dm — DM requests live inside the inbox
	//                    stream (accept/decline/block via RequestPane), so
	//                    /messages/requests is reachable from the hub bar's
	//                    pending badge rather than a permanent tab
	//   bookings       → service_directory.enabled (fallback:
	//                    service_packages.enabled) or mentorship.enabled —
	//                    mentorship bookings are served to the booker even
	//                    when packages are off
	//   mentorship     → mentorship.enabled
	//   support        → feature.tickets (routeFeatureFlags /messages/support)
	import { t } from '#lib/i18n/index.js';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { UrlTabs, type UrlTab } from '#lib/components/ui/index.js';
	import { bookingsInboxEnabled } from '#lib/features.js';

	interface Props {
		value: 'inbox' | 'requests' | 'bookings' | 'mentorship' | 'support';
	}

	let { value }: Props = $props();

	const features = $derived((page.data.features ?? {}) as Record<string, boolean>);
	const dmEnabled = $derived(!!features['feature.dm']);
	const bookingsEnabled = $derived(bookingsInboxEnabled(features));
	const mentorshipEnabled = $derived(!!features['mentorship.enabled']);
	const ticketsEnabled = $derived(!!features['feature.tickets']);

	const tabs = $derived<UrlTab[]>([
		...(dmEnabled ? [{ value: 'inbox', label: t('messages.tab_inbox', 'Messages') }] : []),
		...(bookingsEnabled ? [{ value: 'bookings', label: t('messages.tab_bookings', 'Bookings') }] : []),
		...(mentorshipEnabled
			? [{ value: 'mentorship', label: t('messages.tab_mentorship', 'Mentorship') }]
			: []),
		...(ticketsEnabled ? [{ value: 'support', label: t('messages.tab_support', 'Support') }] : [])
	]);

	const hrefs: Record<string, string> = {
		inbox: '/messages',
		bookings: '/messages/bookings',
		mentorship: '/messages/mentorship',
		support: '/messages/support'
	};

	function onValueChange(v: string) {
		const href = hrefs[v];
		if (href && v !== value) void goto(href);
	}
</script>

<UrlTabs {tabs} {value} {onValueChange} autoOverflow class="!border-b-0 !bg-transparent" />
