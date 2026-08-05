<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	// L1: redirectViaLogin bypasses the SvelteKit router for auth
	// redirects, avoiding the get_navigation_intent crash that occurs
	// when goto() is called before the router is initialized. This
	// eliminates the brief "Loading..." flash.
	function redirectViaLogin() {
		if (typeof window !== 'undefined') {
			window.location.href = '/login';
		}
	}
	import { setUser, setPermissions, clearUser } from '$lib/stores/auth.svelte';
	import { t, translations, initLocale, loadTranslations } from '$lib/stores/i18n.svelte';
	import { setToken } from '$lib/api';
	import TopBar from '$lib/components/layout/TopBar.svelte';
	import AccessDenied from '$lib/components/AccessDenied.svelte';
	import { Tooltip } from 'bits-ui';
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
		Flag,
		Building2,
		DollarSign,
		Clock,
		Rocket,
		KeyRound,
		Puzzle,
		Webhook,
		DatabaseBackup,
		ShieldCheck,
		Coins,
		Cpu,
	} from 'lucide-svelte';
	type LucideIcon = typeof LayoutDashboard;

	let { children } = $props();

	let sidebarOpen = $state(false);
	let userPermissions = $state<string[]>([]);
	let authChecked = $state(false);
	let authInflight = false;
	let mounted = $state(false);

	const currentPath = $derived(page.url.pathname);
	const isAuthPage = $derived(currentPath === '/login' || currentPath.startsWith('/logout'));

	$effect(() => {
		loadTranslations('en');
	});

	onMount(() => {
		mounted = true;
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
		// H4: read the persisted JWT back on app load. The backend does not issue
		// an httpOnly session cookie, so localStorage is the only way to re-establish
		// the session across reloads. (See login/+page.svelte for the residual XSS risk.)
		const storedToken = typeof window !== 'undefined' ? localStorage.getItem('jc_token') : null;
		if (storedToken) {
			setToken(storedToken);
			authChecked = false;
			authInflight = true;
			initAuth(storedToken);
		} else {
			redirectViaLogin();
		}
	});

	async function initAuth(token: string) {
		try {
			const { api } = await import('$lib/api');
			const result = await api<{ id: string; email: string; username: string; display_name: string; user_group_name: string; avatar_url: string | null; user_group_id: string; theme_preference: string; timezone: string; is_verified: boolean; created_at: string }>('/api/v1/me');
			if (result.ok) {
				const userData = result.data;
				if (userData.user_group_name !== 'super_admin') {
					localStorage.removeItem('jc_token');
					setToken(null);
					clearUser();
					if (!isAuthPage) redirectViaLogin();
					authChecked = true;
					return;
				}
				// C1: Fetch the resolved permission set from /me/permissions
				// and verify the control_panel.access node (or '*' wildcard).
				// The backend RBAC resolver returns '*' for super_admin_role,
				// plus any explicit grants like control_panel.access (migration
				// 000184). This prevents a revoked super_admin from entering.
				let perms: string[] = [];
				const permResult = await api<{ permissions: string[] }>('/api/v1/me/permissions');
				if (permResult.ok && permResult.data?.permissions) {
					perms = permResult.data.permissions;
				}
				const hasControlPanelAccess = perms.includes('*') || perms.includes('control_panel.access');
				if (!hasControlPanelAccess) {
					localStorage.removeItem('jc_token');
					setToken(null);
					clearUser();
					if (!isAuthPage) redirectViaLogin();
					authChecked = true;
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
				localStorage.removeItem('jc_token');
				setToken(null);
				clearUser();
				if (!isAuthPage) redirectViaLogin();
			}
		} catch {
			localStorage.removeItem('jc_token');
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
		{ href: '/cache', icon: Trash2 },
		{ href: '/settings', icon: Settings },
		{ href: '/feature-flags', icon: Flag },
		{ href: '/tenants', icon: Building2 },
		{ href: '/ai-budget', icon: DollarSign },
		{ href: '/ai-providers', icon: Cpu },
		{ href: '/cron', icon: Clock },
		{ href: '/deployments', icon: Rocket },
		{ href: '/currencies', icon: Coins }
	];

	const navKeyMap: Record<string, { label: string; tooltip: string; defaultLabel: string; defaultTooltip: string }> = {
		'/': { label: 'nav.dashboard', tooltip: 'nav.dashboard_tooltip', defaultLabel: 'Dashboard', defaultTooltip: 'System health overview' },
		'/queue': { label: 'nav.queue', tooltip: 'nav.queue_tooltip', defaultLabel: 'Queue', defaultTooltip: 'Background job queue stats' },
		'/dlq': { label: 'nav.dlq', tooltip: 'nav.dlq_tooltip', defaultLabel: 'Dead Letter Queue', defaultTooltip: 'Dead letter queue entries' },
		'/db': { label: 'nav.db_health', tooltip: 'nav.db_tooltip', defaultLabel: 'DB Health', defaultTooltip: 'PostgreSQL health & metrics' },
		'/audit': { label: 'nav.audit', tooltip: 'nav.audit_tooltip', defaultLabel: 'Audit Log', defaultTooltip: 'Audit log history' },
		'/security': { label: 'nav.security', tooltip: 'nav.security_tooltip', defaultLabel: 'Security', defaultTooltip: 'Security overview & blocks' },
		'/api-keys': { label: 'nav.api_keys', tooltip: 'nav.api_keys_tooltip', defaultLabel: 'API Keys', defaultTooltip: 'API key management' },
		'/cache': { label: 'nav.cache', tooltip: 'nav.cache_tooltip', defaultLabel: 'Cache', defaultTooltip: 'Cache management & stats' },
		'/settings': { label: 'nav.settings', tooltip: 'nav.settings_tooltip', defaultLabel: 'Settings', defaultTooltip: 'Platform configuration' },
		'/feature-flags': { label: 'nav.feature_flags', tooltip: 'nav.feature_flags_tooltip', defaultLabel: 'Feature Flags', defaultTooltip: 'Feature flag management' },
		'/tenants': { label: 'nav.tenants', tooltip: 'nav.tenants_tooltip', defaultLabel: 'Tenants', defaultTooltip: 'Multi-tenant management' },
		'/ai-budget': { label: 'nav.ai_budget', tooltip: 'nav.ai_budget_tooltip', defaultLabel: 'AI Budget', defaultTooltip: 'AI cost tracking & limits' },
		'/ai-providers': { label: 'nav.ai_providers', tooltip: 'nav.ai_providers_tooltip', defaultLabel: 'AI Providers', defaultTooltip: 'Manage AI provider configurations' },
		'/cron': { label: 'nav.cron', tooltip: 'nav.cron_tooltip', defaultLabel: 'Cron Jobs', defaultTooltip: 'Scheduled job management' },
		'/deployments': { label: 'nav.deployments', tooltip: 'nav.deployments_tooltip', defaultLabel: 'Deployments', defaultTooltip: 'Deployment version history' },
		'/oauth-clients': { label: 'nav.oauth_clients', tooltip: 'nav.oauth_clients_tooltip', defaultLabel: 'OAuth Clients', defaultTooltip: 'OAuth provider client management' },
		'/extensions': { label: 'nav.extensions', tooltip: 'nav.extensions_tooltip', defaultLabel: 'Extensions', defaultTooltip: 'Platform extensions and integrations' },
		'/webhooks': { label: 'nav.webhooks', tooltip: 'nav.webhooks_tooltip', defaultLabel: 'Webhooks', defaultTooltip: 'Webhook endpoint management' },
		'/backup-configs': { label: 'nav.backup_configs', tooltip: 'nav.backup_configs_tooltip', defaultLabel: 'Backup Configs', defaultTooltip: 'Database backup configuration' },
		'/permission-nodes': { label: 'nav.permission_nodes', tooltip: 'nav.permission_nodes_tooltip', defaultLabel: 'Permission Nodes', defaultTooltip: 'RBAC permission key viewer' },
		'/currencies': { label: 'nav.currencies', tooltip: 'nav.currencies_tooltip', defaultLabel: 'Currencies', defaultTooltip: 'Credit currency exchange rates' },
	};

	const nav = $derived.by(() => {
		const tr = translations();
		return navConfig.map((cfg) => ({
			...cfg,
			label: tr[navKeyMap[cfg.href].label] ?? navKeyMap[cfg.href].defaultLabel,
			tooltip: tr[navKeyMap[cfg.href].tooltip] ?? navKeyMap[cfg.href].defaultTooltip,
		}));
	});

	// C1/L2: Access is gated by the control_panel.access RBAC node (or '*'
	// wildcard) verified in initAuth via /me/permissions. The derived check
	// here mirrors the gate so AccessDenied renders if permissions change.
	const canAccess = $derived(
		userPermissions.includes('*') || userPermissions.includes('control_panel.access')
	);
</script>

<svelte:head>
	<title>JapaConnect Control</title>
</svelte:head>

<Tooltip.Provider>
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
			class="bg-background-alt border-border fixed left-4 top-4 z-50 rounded-lg border p-2 shadow-mini md:hidden"
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
			class="border-border-card bg-background-alt fixed inset-y-0 left-0 z-40 flex w-60 flex-shrink-0 flex-col transition-transform md:static md:translate-x-0"
			class:translate-x-0={sidebarOpen}
			class:-translate-x-full={!sidebarOpen}
		>
			<div class="border-border-card flex h-14 items-center border-b px-4">
				<span class="text-accent-foreground text-base font-bold tracking-wide uppercase">{t('control.sidebar_title', 'Control Panel')}</span>
			</div>
			<nav class="flex-1 space-y-0.5 overflow-y-auto p-3">
				{#each nav as item}
					{@const isActive = item.href === '/' ? currentPath === '/' : currentPath.startsWith(item.href)}
					<Tooltip.Root delayDuration={200}>
						<Tooltip.Trigger>
							<a
								href={item.href}
								class="hover:bg-dark-04 data-[state=active]:bg-accent flex items-center gap-3 rounded-9px px-3 py-2.5 text-base font-medium transition-colors {isActive ? 'bg-accent text-accent-foreground' : 'text-foreground-alt'}"
								onclick={() => (sidebarOpen = false)}
							>
								<item.icon class="size-4" />
								{item.label}
							</a>
						</Tooltip.Trigger>
						<Tooltip.Portal>
							<Tooltip.Content
								side="right"
								sideOffset={8}
								class="border-dark-10 bg-background shadow-popover z-50 rounded-input border px-3 py-1.5 text-xs font-medium"
							>
								{item.tooltip}
							</Tooltip.Content>
						</Tooltip.Portal>
					</Tooltip.Root>
				{/each}
			</nav>
		</aside>

		{#if sidebarOpen}
			<button
				class="fixed inset-0 z-30 bg-black/50 md:hidden"
				aria-label="Close sidebar"
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
</Tooltip.Provider>
