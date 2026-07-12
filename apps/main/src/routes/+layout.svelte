<script lang="ts">
	import '../app.css';
	import favicon from '$lib/assets/favicon.svg';
	import { Header, MobileNav, Footer, SideNav } from '$lib/components/nav';
	import { theme } from '$lib/stores/theme.svelte';
	import { locale } from '$lib/stores/i18n.svelte';
	import { setToken } from '$lib/api';
	import { setFeatureFlags } from '$lib/stores/features.svelte';
	import { trackEvent, AnalyticsEvents } from '$lib/analytics';
	import { onNavigate } from '$app/navigation';
	import { page } from '$app/state';
	import { wsClient } from '$lib/websocket';
	import { realtime, initRealtimeListeners } from '$lib/stores/realtime.svelte';
	import { showToast } from '$lib/stores/toast.svelte';
	import { initGlitchTip } from '$lib/glitchtip';
	import {
		PUBLIC_JC_UMAMI_URL,
		PUBLIC_JC_UMAMI_SITE_ID,
		PUBLIC_JC_SITE_URL
	} from '$env/static/public';
	import Toast from '$lib/components/Toast.svelte';

	const siteUrl = PUBLIC_JC_SITE_URL || 'https://japaconnect.com';

	let { children, data } = $props();

	let sidenavOpen = $state(false);
	let localNotificationCount = $state(0);

	$effect(() => {
		theme.init('system');
		initGlitchTip();
	});

	$effect(() => {
		locale.init(data.locale, data.translations);
	});

	$effect(() => {
		setToken(data.token ?? null);
	});

	$effect(() => {
		setFeatureFlags(data.flags);
	});

	// Initialize notification count from server data
	$effect(() => {
		localNotificationCount = data.notificationCount ?? 0;
		realtime.setNotificationCount(data.notificationCount ?? 0);
	});

	// Connect WebSocket when user is authenticated
	$effect(() => {
		if (!data.user) return;

		wsClient.connect();
		const cleanupRealtime = initRealtimeListeners();

		return () => {
			wsClient.disconnect();
			cleanupRealtime();
		};
	});

	// Listen for notification events to update count + show toast
	$effect(() => {
		if (!data.user) return;
		const unsub = wsClient.on('notification', (notification: { title?: string }) => {
			localNotificationCount++;
			realtime.setNotificationCount(localNotificationCount);
			if (notification?.title) {
				showToast(notification.title, 'info');
			}
		});
		return unsub;
	});

	// Listen for DM messages to show toast
	$effect(() => {
		if (!data.user) return;
		const unsub = wsClient.on('dm_message', (message: { sender_name?: string; conversation_id?: string }) => {
			const currentPath = typeof window !== 'undefined' ? window.location.pathname : '';
			const isInConversation = currentPath === `/messages/${message?.conversation_id}`;
			if (!isInConversation && message?.sender_name) {
				showToast(`New message from ${message.sender_name}`, 'info');
			}
		});
		return unsub;
	});

	// Sync notification count from realtime store (in case other components modify it)
	$effect(() => {
		localNotificationCount = realtime.notificationCount;
	});

	// Track page views on navigation
	$effect(() => {
		trackEvent(AnalyticsEvents.PAGE_VIEW, { path: page.url.pathname });
	});

	onNavigate((nav) => {
		if (!document.startViewTransition) return;
		return new Promise<void>((resolve) => {
			document.startViewTransition(async () => {
				resolve();
				await nav.complete;
			});
		});
	});
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<meta name="description" content="JapaConnect — community platform for the Nigerian diaspora" />
	{#each ['en', 'fr', 'yo', 'ha'] as langCode}
		<link rel="alternate" hreflang={langCode} href="{siteUrl}/{langCode}{page.url.pathname}" />
	{/each}
	<link rel="alternate" hreflang="x-default" href="{siteUrl}{page.url.pathname}" />
	<!-- Umami Analytics (Section 40) — only in production -->
	{#if import.meta.env.MODE !== 'development'}
		{#if PUBLIC_JC_UMAMI_URL}
			<script
				defer
				data-website-id={PUBLIC_JC_UMAMI_SITE_ID}
				src="{PUBLIC_JC_UMAMI_URL}/script.js"
			></script>
		{/if}
	{/if}
</svelte:head>

<div class="flex min-h-screen flex-col">
	<Header
		user={data.user}
		notificationCount={localNotificationCount}
		onsidenavToggle={() => (sidenavOpen = !sidenavOpen)}
	/>

	<SideNav
		open={sidenavOpen}
		user={data.user}
		onclose={() => (sidenavOpen = false)}
	/>

	<main class="flex-1 pb-16 md:pb-0">
		{@render children()}
	</main>

	<Footer />
</div>

<MobileNav notificationCount={localNotificationCount} />
<Toast />
