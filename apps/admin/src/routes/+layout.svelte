<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { onMount } from 'svelte';
	import { setUser, setPermissions, clearUser } from '$lib/stores/auth.svelte';
	import { getPermissions } from '$lib/stores/permissions.svelte';

	import { initNotifications, addNotification } from '$lib/stores/notifications.svelte';
	import { wsClient } from '$lib/websocket';
	import Sidebar from '$lib/components/layout/sidebar.svelte';
	import TopBar from '$lib/components/layout/TopBar.svelte';
	import Breadcrumb from '$lib/components/layout/Breadcrumb.svelte';
	import AccessDenied from '$lib/components/AccessDenied.svelte';
	import '../app.css';
	import { page } from '$app/state';
	import { api } from '$lib/api';

	let { children } = $props();
	let loading = $state(true);
	let sidebarMobileOpen = $state(false);
	let currentPermissions = $state<string[]>([]);

	// Fetch user data client-side on mount
	onMount(async () => {
		try {
			// Fetch user info
			const userResult = await api<{ user: any }>('/me');
			if (userResult.ok) {
				setUser(userResult.data.user);
				
				// Fetch permissions separately
				const permResult = await api<{ permissions: string[] }>('/me/permissions');
				if (permResult.ok) {
					setPermissions(permResult.data.permissions ?? []);
					currentPermissions = permResult.data.permissions ?? [];
				} else {
					setPermissions([]);
					currentPermissions = [];
				}
			} else {
				clearUser();
			}
		} catch {
			clearUser();
		} finally {
			loading = false;
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
		'/blog': 'blog.review.manage',
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

	// Read permissions directly so the gate re-evaluates when permissions load —
	// a plain hasPermission() call inside $derived is not tracked and would only
	// recompute on navigation.
	let hasAccess = $derived(
		!requiredPermission ||
			getPermissions().includes(requiredPermission) ||
			getPermissions().includes('*')
	);

	// Connect WebSocket after user is loaded
	$effect(() => {
		if (!loading) {
			initNotifications();
			wsClient.connect();

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
		}
	});
	// Check if we're on the login or logout page
	let isAuthPage = $derived(
		page.url.pathname === '/login' || 
		page.url.pathname.startsWith('/login/') ||
		page.url.pathname === '/logout' ||
		page.url.pathname.startsWith('/logout/')
	);
</script>

<svelte:head>
	<title>JapaConnect Admin</title>
</svelte:head>

{#if isAuthPage}
	{@render children()}
{:else}
	<div class="flex h-screen overflow-hidden" style="background-color: var(--color-background); color: var(--color-foreground)">
		<Sidebar bind:mobileOpen={sidebarMobileOpen} permissions={currentPermissions} />

		<div class="flex flex-1 flex-col overflow-hidden">
			<TopBar onMenuClick={() => sidebarMobileOpen = !sidebarMobileOpen} />
			<main class="flex-1 overflow-y-auto" style="background-color: var(--color-background)">
				<div class="mx-auto max-w-screen-2xl px-4 py-6 sm:px-6 md:px-8">
					<Breadcrumb />
					{#if hasAccess}
						{@render children()}
					{:else}
						<AccessDenied {requiredPermission} />
					{/if}
				</div>
			</main>
		</div>
	</div>
{/if}
