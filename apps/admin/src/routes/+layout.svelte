<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { onMount } from 'svelte';
	import { setUser, setPermissions, clearUser } from '$lib/stores/auth.svelte';
	import { getPermissions } from '$lib/stores/permissions.svelte';
	import { getAccessToken, setAccessToken } from '$lib/api/token';

	import { initNotifications, addNotification } from '$lib/stores/notifications.svelte';
	import { wsClient } from '$lib/websocket';
	import Sidebar from '$lib/components/layout/sidebar.svelte';
	import TopBar from '$lib/components/layout/TopBar.svelte';
	import Breadcrumb from '$lib/components/layout/Breadcrumb.svelte';
	import AccessDenied from '$lib/components/AccessDenied.svelte';
	import { CardSkeleton } from '$lib/components/ui/skeleton';
	import '../app.css';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api, apiFetch } from '$lib/api';
	import { fetchCsrfToken } from '$lib/api/csrf';

	let { children } = $props();
	let loading = $state(true);
	let sidebarMobileOpen = $state(false);
	let currentPermissions = $state<string[]>([]);

	onMount(async () => {
		fetchCsrfToken().catch(() => {});

		if (!getAccessToken()) {
			const refreshRes = await apiFetch<{ access_token: string }>('/auth/refresh', {
				method: 'POST',
				headers: { 'X-Skip-Refresh': '1' }
			});
			if (!refreshRes.error && refreshRes.data?.access_token) {
				setAccessToken(refreshRes.data.access_token);
			}
		}

		try {
			const userResult = await api<{ user: any }>('/me');
			if (userResult.ok) {
				setUser(userResult.data.user);
				
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
				if (!isAuthPage) goto('/login');
			}
		} catch {
			clearUser();
			if (!isAuthPage) goto('/login');
		} finally {
			loading = false;
		}
	});

	const PAGE_PERMISSIONS: Record<string, string> = {
		'/users': 'admin.users.view',
		'/content': 'admin.content.view',
		'/moderation': 'admin.moderation',
		'/analytics': 'admin.analytics.view',
		'/settings': 'admin.settings.view',
		'/seo': 'admin.settings',
		'/roles': 'admin.roles.view',
		'/audit-logs': 'admin.audit.view',
		'/advertising': 'admin.advertising.view',
		'/payments': 'admin.payments.view',
		'/ai': 'admin.ai.view',
		'/blog': 'blog.review.manage',
		'/feature-flags': 'admin.feature_flags.view',
		'/gdpr': 'gdpr.deletions.view',
		'/moderation/alerts': 'moderation.alerts.manage',
		'/spaces': 'spaces.admin',
		'/geo': 'geo.lookup',
		'/tags': 'tags.manage',
		'/universities': 'admin.settings',
		'/university/instructors': 'university.instructor.approve',
		'/university/tracks': 'university.course.approve',
		'/university/categories': 'university.category.manage',
		'/university/payouts': 'university.payout.process',
		'/university/revenue-splits': 'university.revenue.list',
		'/organisations': 'admin.moderation',
		'/organisations/claims': 'admin.moderation',
		'/profiles': 'professional.manage',
		'/reactions': 'content.reaction_types.manage',
		'/media': 'media.view_all',
		'/tickets': 'tickets.admin',
		'/ticket-types': 'tickets.type.manage',
		'/bots': 'bots.admin',
		'/broadcasts': 'admin.broadcast',
		'/broadcast': 'admin.broadcast',
		'/webhooks': 'webhooks.manage',
		'/menus': 'menus.manage',
		'/redirects': 'redirects.manage',
		'/email-templates': 'email.templates.manage',
		'/spam': 'spam.ip_block.manage',
		'/newsletter': 'newsletter.admin',
		'/comparisons': 'comparison.create',
		'/comparison-types': 'comparison.type.create',
		'/coupons': 'coupons.manage',
		'/discount-codes': 'payments.discount_codes.manage',
		'/referral': 'referral.admin',
		'/mentorship': 'mentorship.admin',
		'/learning': 'learning.admin.manage',
		'/reviews': 'reviews.moderate',
		'/notifications': 'notifications.manage',
		'/notification-types': 'notifications.manage',
		'/forms': 'forms.create',
		'/onboarding': 'admin.settings.view',
		'/custom-fields': 'custom_fields.group.view',
		'/guide-categories': 'custom_fields.group.view',
		'/widgets': 'pagebuilder.blocks.manage',
		'/jsonld-templates': 'content.jsonld.manage',
		'/page-builder': 'pagebuilder.blocks.manage',
		'/page-templates': 'pagebuilder.templates.manage',
		'/translations': 'i18n.manage',
		'/search': 'search.analytics.view',
		'/professional-types': 'professional.manage',
		'/space-types': 'space_types.create',
		'/ads': 'ad.campaign.manage',
		'/data-exports': 'gdpr.data_exports.view',
		'/i18n': 'i18n.manage',
		'/wallet': 'wallet.admin',
		'/affiliate': 'affiliate.admin.view',
		'/docs': 'docs.manage',
		'/deletions': 'gdpr.deletions.view',
		'/moderation-queue': 'admin.moderation',
		'/moderation/content': 'moderation.content.review',
		'/moderation/reports': 'reports.review',
		'/moderation/appeals': 'reports.review',
		'/gdpr/audit-logs': 'gdpr.audit_logs.view',
		'/gdpr/consents': 'gdpr.consents.view',
		'/blog/review-queue': 'blog.review.manage'
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
			const unsubNotifications = initNotifications();
			wsClient.connect();

			const unsubNotification = wsClient.on('notification', (msg: any) => {
				addNotification({
					title: msg.title ?? t('layout.ws.notification_title', 'Notification'),
					body: msg.body ?? '',
					type: msg.type ?? 'info',
					link: msg.link
				});
			});

			const unsubAudit = wsClient.on('audit_log', (msg: any) => {
				addNotification({
					title: t('layout.ws.audit_event', 'Audit Event'),
					body: `${msg.action_type ?? 'action'} on ${msg.entity_type ?? 'entity'}`,
					type: 'info'
				});
			});

			return () => {
				unsubNotification();
				unsubAudit();
				unsubNotifications?.();
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
					{#if loading}
						<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4 pt-6">
							{#each Array(4) as _}
								<CardSkeleton />
							{/each}
						</div>
					{:else if hasAccess}
						{@render children()}
					{:else}
						<AccessDenied {requiredPermission} />
					{/if}
				</div>
			</main>
		</div>
	</div>
{/if}
