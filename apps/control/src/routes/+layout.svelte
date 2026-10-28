<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	// L1: redirectViaLogin uses goto() for SPA navigation when the router
	// is ready (the auth guard runs after mount + async API calls), with a
	// window.location.href fallback in case goto() throws — which can happen
	// if the router is not yet initialized (get_navigation_intent crash).
	// Ref: §1.2 (SvelteKit client nav uses window.location redirects instead
	// of goto() → full reload kills SPA auth/session context)
	async function redirectViaLogin() {
		if (typeof window === 'undefined') return;
		try {
			await goto('/login', { replaceState: true });
		} catch {
			// Router not ready — fall back to a full navigation.
			window.location.href = '/login';
		}
	}
	import { setUser, setPermissions, clearUser } from '$lib/stores/auth.svelte';
	import { t, translations, initLocale, loadTranslations } from '$lib/stores/i18n.svelte';
	import { initSiteSettings, siteName, faviconUrl, faviconForTheme } from '$lib/stores/site.svelte';
	import { getTheme } from '$lib/stores/theme.svelte';
	import { setToken, getToken } from '$lib/api';
	import { apiFetch } from '$lib/api/client';
	import { env } from '$env/dynamic/public';
	import TopBar from '$lib/components/layout/TopBar.svelte';
	import AccessDenied from '$lib/components/AccessDenied.svelte';
	import Tooltip from '$lib/components/ui/Tooltip.svelte';
	import '../app.css';
	import {
		Menu,
		X,
		LayoutDashboard,
		ListTodo,
		Database,
		ScrollText,
		Shield,
		Trash2,
		Key,
		AlertTriangle,
		Settings,
		DollarSign,
		Clock,
		Sprout,
		Rocket,
		KeyRound,
		Puzzle,
		Webhook,
		DatabaseBackup,
		ShieldCheck,
		Coins,
		Cpu,
		BrainCircuit,
		BarChart3,
		Compass,
	} from 'lucide-svelte';
	type LucideIcon = typeof LayoutDashboard;

	let { children } = $props();

	let sidebarOpen = $state(false);
	let userPermissions = $state<string[]>([]);
	let authChecked = $state(false);
	let authInflight = false;
	let mounted = $state(false);

	const currentPath = $derived(page.url.pathname);
	// /verify-totp is an auth page (2FA challenge) — it must be exempt from
	// the redirect-to-/login guard so the user can complete the second factor.
	// Ref: sections/02-authentication-sessions.md (TOTP login challenge)
	const isAuthPage = $derived(
		currentPath === '/login' ||
		currentPath === '/verify-totp' ||
		currentPath.startsWith('/logout')
	);

	$effect(() => {
		loadTranslations('en');
	});

	// Apply the custom favicon from appearance settings once settings load;
	// re-runs on theme change so the dark/light variant follows the toggle.
	$effect(() => {
		const url = faviconForTheme(getTheme());
		if (!url) return;
		// Update every icon link — the app.html fallbacks carry
		// prefers-color-scheme media attributes, so mutating only the
		// first link would leave the bundled icon winning in dark mode.
		const links = document.querySelectorAll<HTMLLinkElement>('link[rel="icon"]');
		const ext = url.split('.').pop()?.toLowerCase();
		const mime = ext === 'svg' ? 'image/svg+xml' : ext === 'ico' ? 'image/x-icon' : 'image/png';
		if (links.length === 0) {
			const link = document.createElement('link');
			link.rel = 'icon';
			link.href = url;
			link.type = mime;
			document.head.appendChild(link);
			return;
		}
		links.forEach((link) => { link.href = url; link.type = mime; });
	});

	onMount(() => {
		mounted = true;
		// Brand name (general.site_name) for titles and {site} i18n substitution.
		initSiteSettings();
	});

	$effect(() => {
		// Defer the auth guard until after mount: calling goto() during the layout's
		// first effect run fires before SvelteKit's `app` global is initialized, which
		// crashes `get_navigation_intent` (reading `app.hash` of undefined). After mount
		// the router is ready and `page.url` reflects the real path.
		if (!mounted) return;
		if (isAuthPage) {
			authChecked = true;
			authInflight = false;
			return;
		}
		if (authInflight) return;
		// Re-establish the session on reload by calling /auth/refresh, which
		// uses the httpOnly jc_refresh cookie (scoped to /api/v1/auth) set
		// during login. The access token is stored in-memory only — never
		// in localStorage — to prevent XSS token theft.
		// Ref: sections/02-authentication-sessions.md (Token model)
		authChecked = false;
		authInflight = true;
		initAuth();
	});

	async function initAuth() {
		try {
			const { api } = await import('$lib/api');
			// If we don't have an in-memory token, try to refresh using
			// the httpOnly jc_refresh cookie.
			if (!getToken()) {
				const refreshRes = await apiFetch<{ access_token: string }>('/api/v1/auth/refresh', {
					method: 'POST',
					headers: { 'X-Skip-Refresh': '1' }
				});
				if (!refreshRes.error && refreshRes.data?.access_token) {
					setToken(refreshRes.data.access_token);
				}
			}
			const result = await api<{ id: string; email: string; username: string; display_name: string; user_group_name: string; avatar_url: string | null; user_group_id: string; theme_preference: string; timezone: string; is_verified: boolean; created_at: string }>('/api/v1/me');
			if (result.ok) {
				const userData = result.data;
				if (userData.user_group_name !== 'super_admin') {
					setToken(null);
					clearUser();
					if (!isAuthPage) redirectViaLogin();
					authChecked = true;
					authInflight = false;
					return;
				}
				// C1: Fetch the resolved permission set from /me/permissions
				// and verify the control.panel.access node (or '*' wildcard).
				// The backend RBAC resolver returns '*' for super_admin_role,
				// plus any explicit grants like control.panel.access (migration
				// 000184). This prevents a revoked super_admin from entering.
				let perms: string[] = [];
				const permResult = await api<{ permissions: string[] }>('/api/v1/me/permissions');
				if (permResult.ok && permResult.data?.permissions) {
					perms = permResult.data.permissions;
				}
				const hasControlPanelAccess = perms.includes('*') || perms.includes('control.panel.access');
				if (!hasControlPanelAccess) {
					setToken(null);
					clearUser();
					if (!isAuthPage) redirectViaLogin();
					authChecked = true;
					authInflight = false;
					return;
				}
				setUser({
					id: userData.id,
					email: userData.email,
					username: userData.username,
					display_name: userData.display_name,
					user_group_name: userData.user_group_name,
					avatar_url: userData.avatar_url,
					user_group_id: userData.user_group_id,
					theme_preference: (userData.theme_preference as 'light' | 'dark' | 'system') ?? 'dark',
					timezone: userData.timezone ?? 'UTC',
					is_verified: userData.is_verified,
					created_at: userData.created_at,
					permissions: perms,
				});
				setPermissions(perms);
				userPermissions = perms;
			} else {
				setToken(null);
				clearUser();
				if (!isAuthPage) redirectViaLogin();
			}
		} catch {
			setToken(null);
			clearUser();
			if (!isAuthPage) redirectViaLogin();
		}
		authChecked = true;
		authInflight = false;
	}

	interface NavItem {
		label: string;
		href: string;
		icon: LucideIcon;
		tooltip: string;
	}

	// The Control Panel is super_admin-only: initAuth (above) redirects any
	// non-super_admin to /login, and super_admins are granted the '*' permission
	// set. Per-item permission strings are therefore decorative and were removed
	// to avoid implying a granular RBAC the backend does not enforce on /control/*.
	const navConfig: Omit<NavItem, 'label' | 'tooltip'>[] = [
		{ href: '/', icon: LayoutDashboard },
		{ href: '/analytics', icon: BarChart3 },
		{ href: '/queue', icon: ListTodo },
		{ href: '/dlq', icon: AlertTriangle },
		{ href: '/db', icon: Database },
		{ href: '/audit', icon: ScrollText },
		{ href: '/security', icon: Shield },
		{ href: '/api-keys', icon: Key },
		{ href: '/oauth-clients', icon: KeyRound },
		{ href: '/extensions', icon: Puzzle },
		{ href: '/webhooks', icon: Webhook },
		{ href: '/backup-configs', icon: DatabaseBackup },
		{ href: '/permission-nodes', icon: ShieldCheck },
		{ href: '/next-actions', icon: Compass },
		{ href: '/cache', icon: Trash2 },
		{ href: '/settings', icon: Settings },
		{ href: '/ai-budget', icon: DollarSign },
		{ href: '/ai-providers', icon: Cpu },
		{ href: '/ai-byok-providers', icon: KeyRound },
		{ href: '/cron', icon: Clock },
		{ href: '/seeds', icon: Sprout },
		{ href: '/deployments', icon: Rocket },
		{ href: '/currencies', icon: Coins },
		{ href: '/intelligence', icon: BrainCircuit }
	];

	const navKeyMap: Record<string, { label: string; tooltip: string; defaultLabel: string; defaultTooltip: string }> = {
		'/': { label: 'nav.dashboard', tooltip: 'nav.dashboard_tooltip', defaultLabel: 'Dashboard', defaultTooltip: 'System health overview' },
		'/analytics': { label: 'nav.analytics', tooltip: 'nav.analytics_tooltip', defaultLabel: 'Analytics', defaultTooltip: 'Umami page-view analytics dashboard' },
		'/queue': { label: 'nav.queue', tooltip: 'nav.queue_tooltip', defaultLabel: 'Queue', defaultTooltip: 'Background job queue stats' },
		'/dlq': { label: 'nav.dlq', tooltip: 'nav.dlq_tooltip', defaultLabel: 'Dead Letter Queue', defaultTooltip: 'Dead letter queue entries' },
		'/db': { label: 'nav.db_health', tooltip: 'nav.db_tooltip', defaultLabel: 'DB Health', defaultTooltip: 'PostgreSQL health & metrics' },
		'/audit': { label: 'nav.audit', tooltip: 'nav.audit_tooltip', defaultLabel: 'Audit Log', defaultTooltip: 'Audit log history' },
		'/security': { label: 'nav.security', tooltip: 'nav.security_tooltip', defaultLabel: 'Security', defaultTooltip: 'Security overview & blocks' },
		'/api-keys': { label: 'nav.api_keys', tooltip: 'nav.api_keys_tooltip', defaultLabel: 'API Keys', defaultTooltip: 'API key management' },
		'/cache': { label: 'nav.cache', tooltip: 'nav.cache_tooltip', defaultLabel: 'Cache', defaultTooltip: 'Cache management & stats' },
		'/settings': { label: 'nav.settings', tooltip: 'nav.settings_tooltip', defaultLabel: 'Settings', defaultTooltip: 'Platform configuration' },
		'/ai-budget': { label: 'nav.ai_budget', tooltip: 'nav.ai_budget_tooltip', defaultLabel: 'AI Budget', defaultTooltip: 'AI cost tracking & limits' },
		'/ai-providers': { label: 'nav.ai_providers', tooltip: 'nav.ai_providers_tooltip', defaultLabel: 'AI Providers', defaultTooltip: 'Manage AI provider configurations' },
		'/ai-byok-providers': { label: 'nav.ai_byok_providers', tooltip: 'nav.ai_byok_providers_tooltip', defaultLabel: 'AI BYOK Providers', defaultTooltip: 'Manage the bring-your-own-key provider allowlist' },
		'/cron': { label: 'nav.cron', tooltip: 'nav.cron_tooltip', defaultLabel: 'Cron Jobs', defaultTooltip: 'Scheduled job management' },
		'/seeds': { label: 'nav.seeds', tooltip: 'nav.seeds_tooltip', defaultLabel: 'Seeds', defaultTooltip: 'Seed & enrichment jobs (geo, wiki, attractions)' },
		'/deployments': { label: 'nav.deployments', tooltip: 'nav.deployments_tooltip', defaultLabel: 'Deployments', defaultTooltip: 'Deployment version history' },
		'/oauth-clients': { label: 'nav.oauth_clients', tooltip: 'nav.oauth_clients_tooltip', defaultLabel: 'OAuth Clients', defaultTooltip: 'OAuth provider client management' },
		'/extensions': { label: 'nav.extensions', tooltip: 'nav.extensions_tooltip', defaultLabel: 'Extensions', defaultTooltip: 'Platform extensions and integrations' },
		'/webhooks': { label: 'nav.webhooks', tooltip: 'nav.webhooks_tooltip', defaultLabel: 'Webhooks', defaultTooltip: 'Webhook endpoint management' },
		'/backup-configs': { label: 'nav.backup_configs', tooltip: 'nav.backup_configs_tooltip', defaultLabel: 'Backup Configs', defaultTooltip: 'Database backup configuration' },
		'/permission-nodes': { label: 'nav.permission_nodes', tooltip: 'nav.permission_nodes_tooltip', defaultLabel: 'Permission Nodes', defaultTooltip: 'RBAC permission key viewer' },
		'/next-actions': { label: 'nav.next_actions', tooltip: 'nav.next_actions_tooltip', defaultLabel: 'Next Actions', defaultTooltip: 'Next-action suggestion rules (§122)' },
		'/currencies': { label: 'nav.currencies', tooltip: 'nav.currencies_tooltip', defaultLabel: 'Currencies', defaultTooltip: 'Credit currency exchange rates' },
		'/intelligence': { label: 'nav.intelligence', tooltip: 'nav.intelligence_tooltip', defaultLabel: 'Intelligence', defaultTooltip: 'Data intelligence pipeline & review' },
	};

	const nav = $derived.by(() => {
		const tr = translations();
		return navConfig.map((cfg) => ({
			...cfg,
			label: tr[navKeyMap[cfg.href].label] ?? navKeyMap[cfg.href].defaultLabel,
			tooltip: tr[navKeyMap[cfg.href].tooltip] ?? navKeyMap[cfg.href].defaultTooltip,
		}));
	});

	// C1/L2: Access is gated by the control.panel.access RBAC node (or '*'
	// wildcard) verified in initAuth via /me/permissions. The derived check
	// here mirrors the gate so AccessDenied renders if permissions change.
	const canAccess = $derived(
		userPermissions.includes('*') || userPermissions.includes('control.panel.access')
	);

	// Umami analytics — Control Panel surface (§40).
	// Ref: japaconnect-engineering/ADR-040-platform-analytics-dual-layer.md
	const umamiEnabled = $derived(!!env.PUBLIC_JC_UMAMI_URL && !!env.PUBLIC_JC_UMAMI_SITE_ID_CONTROL);
</script>

<svelte:head>
	<title>{siteName()} Control</title>
	<!-- Internal surface: never index. Defense-in-depth alongside robots.txt Disallow:/. -->
	<meta name="robots" content="noindex,nofollow" />
	{#if umamiEnabled}
		<!-- Umami page-view analytics (§40, ADR-040). Cookieless, privacy-first. -->
		<script
			defer
			src="{env.PUBLIC_JC_UMAMI_URL}/script.js"
			data-website-id={env.PUBLIC_JC_UMAMI_SITE_ID_CONTROL}
		></script>
	{/if}
</svelte:head>

{#if isAuthPage}
	<main class="bg-background text-foreground flex min-h-screen items-center justify-center">
		{@render children()}
	</main>
{:else if !authChecked}
	<main class="bg-background text-foreground flex min-h-screen items-center justify-center">
		<p class="text-muted-foreground text-sm">{t('common.loading', 'Loading...')}</p>
	</main>
{:else}
	<div class="bg-background text-foreground flex h-screen overflow-hidden">
		<button
			class="bg-background-alt border-border fixed start-4 top-4 z-50 rounded-lg border p-2 shadow-mini md:hidden"
			aria-label={t('common.toggle_menu')}
			onclick={() => (sidebarOpen = !sidebarOpen)}
		>
			{#if sidebarOpen}
				<X class="size-5" />
			{:else}
				<Menu class="size-5" />
			{/if}
		</button>

		<aside
			class="border-border-card bg-background-alt fixed inset-y-0 start-0 z-40 flex w-60 flex-shrink-0 flex-col transition-transform md:static md:translate-x-0"
			class:translate-x-0={sidebarOpen}
			class:-translate-x-full={!sidebarOpen}
		>
			<div class="border-border-card flex h-14 items-center border-b px-4">
				<span class="text-accent-foreground text-base font-bold tracking-wide uppercase">{t('control.sidebar_title', 'Control Panel')}</span>
			</div>
			<nav class="flex-1 space-y-0.5 overflow-y-auto p-3">
				{#each nav as item}
					{@const isActive = item.href === '/' ? currentPath === '/' : currentPath.startsWith(item.href)}
					<Tooltip content={item.tooltip} side="right">
						<a
							href={item.href}
							class="hover:bg-dark-04 data-[state=active]:bg-accent flex items-center gap-3 rounded-9px px-3 py-2.5 text-base font-medium transition-colors {isActive ? 'bg-accent text-accent-foreground' : 'text-foreground-alt'}"
							onclick={() => (sidebarOpen = false)}
						>
							<item.icon class="size-4" />
							{item.label}
						</a>
					</Tooltip>
				{/each}
			</nav>
		</aside>

		{#if sidebarOpen}
			<button
				class="fixed inset-0 z-30 bg-black/50 md:hidden"
				aria-label={t('common.close_sidebar', 'Close sidebar')}
				onclick={() => (sidebarOpen = false)}
			></button>
		{/if}

		<div class="flex flex-1 flex-col overflow-hidden">
			<TopBar />
			<main class="flex-1 overflow-y-auto">
				<div class="mx-auto max-w-7xl px-4 py-6 sm:px-6 md:px-8">
					{#if canAccess}
						{@render children()}
					{:else}
						<AccessDenied />
					{/if}
				</div>
			</main>
		</div>
	</div>
{/if}
