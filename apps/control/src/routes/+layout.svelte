<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
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
			goto('/login');
		}
	});

	async function initAuth(token: string) {
		try {
			const { api } = await import('$lib/api');
			const result = await api<{ id: string; email: string; username: string; display_name: string; user_group_name: string; avatar_url: string | null; user_group_id: string; theme_preference: string; timezone: string; is_verified: boolean; created_at: string; permissions: string[] }>('/api/v1/me');
			if (result.ok) {
				const userData = result.data;
				if (userData.user_group_name !== 'super_admin') {
					localStorage.removeItem('jc_token');
					setToken(null);
					clearUser();
					if (!isAuthPage) goto('/login');
					authChecked = true;
					return;
				}
				if (userData.user_group_name === 'super_admin') {
					userData.permissions = ['*'];
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
					permissions: userData.permissions ?? [],
				});
				const perms = userData.permissions ?? [];
				setPermissions(perms);
				userPermissions = perms;
			} else {
				localStorage.removeItem('jc_token');
				setToken(null);
				clearUser();
				if (!isAuthPage) goto('/login');
			}
		} catch {
			localStorage.removeItem('jc_token');
			setToken(null);
			clearUser();
			if (!isAuthPage) goto('/login');
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
		{ href: '/cache', icon: Trash2 },
		{ href: '/settings', icon: Settings },
		{ href: '/feature-flags', icon: Flag },
		{ href: '/tenants', icon: Building2 },
		{ href: '/ai-budget', icon: DollarSign },
		{ href: '/cron', icon: Clock },
		{ href: '/deployments', icon: Rocket }
	];

	const navKeyMap: Record<string, { label: string; tooltip: string }> = {
		'/': { label: 'nav.dashboard', tooltip: 'nav.dashboard_tooltip' },
		'/queue': { label: 'nav.queue', tooltip: 'nav.queue_tooltip' },
		'/dlq': { label: 'nav.dlq', tooltip: 'nav.dlq_tooltip' },
		'/db': { label: 'nav.db_health', tooltip: 'nav.db_tooltip' },
		'/audit': { label: 'nav.audit', tooltip: 'nav.audit_tooltip' },
		'/security': { label: 'nav.security', tooltip: 'nav.security_tooltip' },
		'/api-keys': { label: 'nav.api_keys', tooltip: 'nav.api_keys_tooltip' },
		'/cache': { label: 'nav.cache', tooltip: 'nav.cache_tooltip' },
		'/settings': { label: 'nav.settings', tooltip: 'nav.settings_tooltip' },
		'/feature-flags': { label: 'nav.feature_flags', tooltip: 'nav.feature_flags_tooltip' },
		'/tenants': { label: 'nav.tenants', tooltip: 'nav.tenants_tooltip' },
		'/ai-budget': { label: 'nav.ai_budget', tooltip: 'nav.ai_budget_tooltip' },
		'/cron': { label: 'nav.cron', tooltip: 'nav.cron_tooltip' },
		'/deployments': { label: 'nav.deployments', tooltip: 'nav.deployments_tooltip' },
	};

	const nav = $derived.by(() => {
		const tr = translations();
		return navConfig.map((cfg) => ({
			...cfg,
			label: tr[navKeyMap[cfg.href].label] ?? navKeyMap[cfg.href].label,
			tooltip: tr[navKeyMap[cfg.href].tooltip] ?? navKeyMap[cfg.href].tooltip,
		}));
	});

	// Access is gated by the super_admin check in initAuth; admitted users carry
	// the '*' permission set, so the Control Panel is effectively all-or-nothing.
	const canAccess = $derived(userPermissions.includes('*'));
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
