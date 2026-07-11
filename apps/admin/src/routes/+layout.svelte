<script lang="ts">
	import { onMount } from 'svelte';
	import { setUser, setPermissions, clearUser, permissions } from '$lib/stores/auth';
	import { initNotifications, addNotification } from '$lib/stores/notifications';
	import { wsClient } from '$lib/websocket';
	import { initGlitchTip } from '$lib/glitchtip';
	import Sidebar from '$lib/components/Sidebar.svelte';
	import TopBar from '$lib/components/layout/TopBar.svelte';
	import AccessDenied from '$lib/components/AccessDenied.svelte';
	import '../app.css';
	import { page } from '$app/state';

	let { children, data } = $props();

	$effect(() => {
		if (data.user) {
			setUser(data.user);
			setPermissions(data.permissions ?? []);
		} else {
			clearUser();
		}
	});

	const PAGE_PERMISSIONS: Record<string, string> = {
		'/users': 'admin.users.view',
		'/content': 'admin.content.view',
		'/moderation': 'admin.moderation.view',
		'/analytics': 'admin.analytics.view',
		'/settings': 'admin.settings.view',
		'/roles': 'admin.roles.view',
		'/audit-logs': 'admin.audit.view',
		'/advertising': 'admin.advertising.view',
		'/payments': 'admin.payments.view',
		'/ai': 'admin.ai.view',
		'/feature-flags': 'admin.feature_flags.view',
		'/gdpr': 'admin.moderation.view',
		'/moderation/alerts': 'admin.moderation.view'
	};

	let requiredPermission = $derived.by(() => {
		const path = page.url.pathname;
		for (const [prefix, perm] of Object.entries(PAGE_PERMISSIONS)) {
			if (path === prefix || path.startsWith(prefix + '/')) {
				return perm;
			}
		}
		return '';
	});

	// Subscribe to the store directly ($permissions) so the gate re-evaluates
	// when permissions load — a plain hasPermission() call inside $derived is
	// not tracked and would only recompute on navigation.
	let hasAccess = $derived(
		!requiredPermission ||
			$permissions.includes(requiredPermission) ||
			$permissions.includes('*')
	);

	onMount(() => {
		initGlitchTip();
		initNotifications();
		wsClient.connect(data.token ?? undefined);

		const unsubNotification = wsClient.on('notification', (msg: any) => {
			addNotification({
				title: msg.title ?? 'Notification',
				body: msg.body ?? '',
				type: msg.type ?? 'info',
				link: msg.link
			});
		});

		const unsubAudit = wsClient.on('audit_log', (msg: any) => {
			addNotification({
				title: 'Audit Event',
				body: `${msg.action_type ?? 'action'} on ${msg.entity_type ?? 'entity'}`,
				type: 'info'
			});
		});

		return () => {
			unsubNotification();
			unsubAudit();
			wsClient.disconnect();
		};
	});
</script>

<svelte:head>
	<title>JapaConnect Admin</title>
</svelte:head>

<div class="flex h-screen overflow-hidden bg-gray-50 dark:bg-gray-900">
	<Sidebar currentPath={page.url.pathname} />

	<div class="flex flex-1 flex-col overflow-hidden">
		<TopBar />
		<main class="flex-1 overflow-y-auto">
			<div class="mx-auto max-w-7xl px-4 py-6 sm:px-6 md:px-8">
				{#if hasAccess}
					{@render children()}
				{:else}
					<AccessDenied {requiredPermission} />
				{/if}
			</div>
		</main>
	</div>
</div>
