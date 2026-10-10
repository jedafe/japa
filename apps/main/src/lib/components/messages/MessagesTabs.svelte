<script lang="ts">
	// Ref: japaconnect-engineering/sections/48-direct-messages.md,
	// sections/111-service-packages-bookings.md, sections/102-mentorship-matching.md
	//
	// Shared Messages-section tab bar — Messages | Requests | Bookings |
	// Mentorship — one tab per route. Each tab is hidden when its feature
	// flag is off (features map comes from the root layout's
	// GET /feature-flags payload):
	//   inbox/requests → feature.dm
	//   bookings       → service_directory.enabled (fallback:
	//                    service_packages.enabled) or mentorship.enabled —
	//                    mentorship bookings are served to the booker even
	//                    when packages are off
	//   mentorship     → mentorship.enabled
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { UrlTabs, type UrlTab } from '$lib/components/ui';
	import { bookingsInboxEnabled } from '$lib/features';

	interface Props {
		value: 'inbox' | 'requests' | 'bookings' | 'mentorship';
		/** Pending DM-request count — badges the Requests tab (inbox page only). */
		requestCount?: number;
	}

	let { value, requestCount = 0 }: Props = $props();

	const features = $derived((page.data.features ?? {}) as Record<string, boolean>);
	const dmEnabled = $derived(!!features['feature.dm']);
	const bookingsEnabled = $derived(bookingsInboxEnabled(features));
	const mentorshipEnabled = $derived(!!features['mentorship.enabled']);

	const tabs = $derived<UrlTab[]>([
		...(dmEnabled
			? [
					{ value: 'inbox', label: t('messages.tab_inbox', 'Messages') },
					{
						value: 'requests',
						label:
							requestCount > 0
								? `${t('messages.tab_requests', 'Requests')} (${requestCount}${requestCount >= 50 ? '+' : ''})`
								: t('messages.tab_requests', 'Requests')
					}
				]
			: []),
		...(bookingsEnabled ? [{ value: 'bookings', label: t('messages.tab_bookings', 'Bookings') }] : []),
		...(mentorshipEnabled
			? [{ value: 'mentorship', label: t('messages.tab_mentorship', 'Mentorship') }]
			: [])
	]);

	const hrefs: Record<string, string> = {
		inbox: '/messages',
		requests: '/messages/requests',
		bookings: '/messages/bookings',
		mentorship: '/messages/mentorship'
	};

	function onValueChange(v: string) {
		const href = hrefs[v];
		if (href && v !== value) void goto(href);
	}
</script>

<UrlTabs {tabs} {value} {onValueChange} class="!border-b-0 !bg-transparent" />
