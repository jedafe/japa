<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { onMount } from 'svelte';
	import { clearUser } from '$lib/stores/auth.svelte';
	import { getPermissions } from '$lib/stores/permissions.svelte';
	import { restoreSession } from '$lib/stores/session.svelte';

	import { initNotifications, addNotification } from '$lib/stores/notifications.svelte';
	import { setGlobalErrorHandler } from '$lib/api/client';
	import { wsClient } from '$lib/websocket';
	import { initSidebarFromStorage, toggleSidebar, sidebarCollapsed } from '$lib/stores/sidebar.svelte';
	import Sidebar from '$lib/components/layout/sidebar.svelte';
	import TopBar from '$lib/components/layout/TopBar.svelte';
	import Breadcrumb from '$lib/components/layout/Breadcrumb.svelte';
	import ImpersonationBanner from '$lib/components/layout/ImpersonationBanner.svelte';
	import AccessDenied from '$lib/components/AccessDenied.svelte';
	import { CardSkeleton } from '$lib/components/ui/skeleton';
	import '../app.css';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { fetchCsrfToken } from '$lib/api/csrf';
	import { env } from '$env/dynamic/public';

	let { children } = $props();
	let loading = $state(true);
	let sidebarMobileOpen = $state(false);

	// Reactive — picks up permission changes after login/impersonation-end
	// without requiring a layout remount.
	let currentPermissions = $derived(getPermissions());

	// Check if we're on the login or logout page — computed first so onMount
	// can skip session restoration on auth pages (avoids noisy 401s when no
	// session exists yet). /verify-totp is an auth page (2FA challenge) and
	// must be exempt from the redirect-to-/login guard so the user can
	// complete the second factor.
	// Ref: sections/02-authentication-sessions.md (TOTP login challenge)
	let isAuthPage = $derived(
		page.url.pathname === '/login' ||
		page.url.pathname.startsWith('/login/') ||
		page.url.pathname === '/verify-totp' ||
		page.url.pathname.startsWith('/verify-totp/') ||
		page.url.pathname === '/logout' ||
		page.url.pathname.startsWith('/logout/')
	);

	onMount(async () => {
		fetchCsrfToken().catch(() => {});
		initSidebarFromStorage();

		// Register a global 403 error handler so permission errors from any
		// API call surface as a notification rather than a silent blank screen.
		setGlobalErrorHandler((err) => {
			addNotification({
				title: t('common.permission_denied', 'Permission Denied'),
				body: err.message || t('common.no_permission_action', 'You do not have permission to perform this action'),
				type: 'error'
			});
		});

		// Skip session restoration on auth pages — no session exists yet and
		// the calls would generate console 401 noise. The login page sets the
		// access token in-memory and calls restoreSession() + goto('/'), which
		// re-runs this onMount via invalidateAll.
		if (isAuthPage) {
			loading = false;
			return;
		}

		const ok = await restoreSession({ tryRefresh: true });
		if (!ok) {
			clearUser();
			goto('/login');
		}
		loading = false;
	});

	// Clear the global error handler when the root layout unmounts.
	$effect(() => {
		return () => setGlobalErrorHandler(null);
	});

	const PAGE_PERMISSIONS: Record<string, string> = {
		'/users': 'admin.users.view',
		'/content': 'admin.content.view',
		'/content-types': 'admin.settings',
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
		'/share-platforms': 'content.reaction_types.manage',
		'/imports': 'import.manage',
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
		'/newsletter': 'newsletter.manage',
		'/comparisons': 'comparison.create',
		'/comparison-types': 'comparison.type.create',
		'/comparison-data-sources': 'jce.data_source.view',
		'/comparison-ingestion-jobs': 'jce.ingestion_job.view',
		'/comparison-profiles': 'jce.profile.list',
		'/coupons': 'coupons.manage',
		'/discount-codes': 'payments.discount_codes.manage',
		'/referral': 'referral.manage',
		'/mentorship': 'mentorship.admin',
		'/learning': 'learning.admin.manage',
		'/reviews': 'reviews.moderate',
		'/reports': 'reports.review',
		'/notifications': 'notifications.manage',
		'/notification-types': 'notifications.manage',
		'/forms': 'forms.create',
		'/onboarding': 'admin.settings.view',
		'/custom-fields': 'custom_fields.group.view',
		'/guide-categories': 'custom_fields.group.view',
		'/widgets': 'widgets.manage',
		'/jsonld-templates': 'content.jsonld.manage',
		'/page-builder': 'pagebuilder.blocks.manage',
		'/page-builder/pages': 'pages.manage',
		'/page-builder/themes': 'pagebuilder.themes.manage',
		'/page-templates': 'pagebuilder.templates.manage',
		'/translations': 'i18n.manage',
		'/search': 'search.analytics.view',
		'/professional-types': 'professional.manage',
		'/space-types': 'spaces.types.create',
		'/ads': 'ad.campaign.manage',
		'/ads/slot-types': 'ad.campaign.manage',
		'/ads/networks': 'ad.network.manage',
		'/data-exports': 'gdpr.data_exports.view',
		'/data-imports': 'gdpr.data_exports.view',
		'/i18n': 'i18n.manage',
		'/wallet': 'wallet.admin',
		'/wallet/credit-packs': 'wallet.credit_pack.manage',
		'/wallet/purchases': 'wallet.purchases.view_any',
		'/affiliate': 'affiliate.admin.view',
		'/docs': 'docs.manage',
		'/deletions': 'gdpr.deletions.view',
		'/sidebars': 'sidebars.manage',
		'/plans': 'payments.plans.manage',
		'/travel': 'travel.manage',
		'/remittance': 'remittance.manage',
		'/personas': 'persona.view',
		'/studio': 'studio.view',
		'/rag': 'ai.rag.manage',
		'/gamification': 'gamification.view',
		'/jobs': 'jobs.admin',
		'/moderation-queue': 'admin.moderation',
		'/moderation/content': 'moderation.content.review',
		'/moderation/reports': 'reports.review',
		'/moderation/appeals': 'reports.review',
		'/gdpr/audit-logs': 'gdpr.audit_logs.view',
		'/gdpr/consents': 'gdpr.consents.view',
		'/blog/review-queue': 'blog.review.manage',
		// V2 community features (§§79–89, §98) — HITL review queues
		'/community/story-wall': 'story.verify',
		'/community/wiki': 'wiki.review',
		'/community/scam-alerts': 'scam_alert.review',
		'/community/planning-groups': 'planning_group.manage_any',
		'/community/mutual-aid': 'mutual_aid.moderate',
		'/community/calendar': 'calendar.review',
		'/community/achievements': 'achievement.moderate',
		'/community/services': 'service.verify',
		'/community/digest': 'digest.manage',
		// V2 functional tools (§§90–97, §§99–101) — HITL review queues
		'/tools/appointment-alert': 'embassy_monitor.manage',
		'/tools/cv-formatter': 'cv_formatter.manage',
		'/tools/visa-checklist': 'checklist.manage',
		'/tools/readiness': 'assessment.view_results',
		'/tools/cost-of-living': 'col_calculator.manage',
		'/tools/scholarships': 'scholarship.manage',
		'/tools/processing-tracker': 'visa_timeline.manage_any',
		'/tools/visa-routes': 'visa_routes.manage',
		'/tools/arrival-checklist': 'arrival_checklist.manage',
		'/tools/credential-checker': 'credential_checker.manage',
		'/tools/housing-guide': 'housing_guide.manage',
		'/tools/attestation-guide': 'attestation_guide.manage',
		'/tools/language-test-hub': 'language_test_hub.manage',
		'/tools/tax-calculator': 'tax_calculator.manage',
		'/tools/community-locator': 'community_locator.manage',
		// Intelligence review queue
		'/intelligence/review': 'intel.review.view'
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

	// Umami analytics — Admin surface (§40).
	// Ref: japaconnect-engineering/ADR-040-platform-analytics-dual-layer.md
	const umamiEnabled = $derived(!!env.PUBLIC_JC_UMAMI_URL && !!env.PUBLIC_JC_UMAMI_SITE_ID_ADMIN);

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
</script>

<svelte:head>
	<title>JapaConnect Admin</title>
	{#if umamiEnabled}
		<!-- Umami page-view analytics (§40, ADR-040). Cookieless, privacy-first. -->
		<script
			defer
			src="{env.PUBLIC_JC_UMAMI_URL}/script.js"
			data-website-id={env.PUBLIC_JC_UMAMI_SITE_ID_ADMIN}
		></script>
	{/if}
</svelte:head>

<svelte:window
	onkeydown={(e) => {
		if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'b') {
			e.preventDefault();
			toggleSidebar();
		}
	}}
/>

{#if isAuthPage}
	{@render children()}
{:else}
	<div class="flex h-screen overflow-hidden" style="background-color: var(--color-background); color: var(--color-foreground)">
		<Sidebar bind:mobileOpen={sidebarMobileOpen} collapsed={sidebarCollapsed()} permissions={currentPermissions} />

		<div class="flex flex-1 flex-col overflow-hidden">
			<TopBar onMenuClick={() => sidebarMobileOpen = !sidebarMobileOpen} onCollapseToggle={toggleSidebar} />
			<ImpersonationBanner />
			<main class="flex-1 overflow-y-auto" style="background-color: var(--color-background)">
				<div class="mx-auto max-w-screen-2xl px-4 py-6 sm:px-6 md:px-8">
					<Breadcrumb />
					{#if loading}
						<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4 pt-6">
							{#each Array(4) as _, i (i)}
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
