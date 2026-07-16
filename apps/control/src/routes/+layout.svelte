<script lang="ts">
	import { page } from '$app/state';
	import { setUser, setPermissions, clearUser } from '$lib/stores/auth.svelte';
	import { t, translations, initLocale, loadTranslations } from '$lib/stores/i18n';
	import { setToken } from '$lib/api';
	import TopBar from '$lib/components/layout/TopBar.svelte';
	import AccessDenied from '$lib/components/AccessDenied.svelte';
	import { Tooltip } from 'bits-ui';
	import '../app.css';
	import {
		Menu,
		X,
	} from 'lucide-svelte';
	import type { Component } from 'svelte';
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const iconModules: Record<string, () => Promise<{ default: any }>> = {
		LayoutDashboard: async () => ({ default: (await import('lucide-svelte/icons/layout-dashboard')).default }),
		ListTodo: async () => ({ default: (await import('lucide-svelte/icons/list-todo')).default }),
		Database: async () => ({ default: (await import('lucide-svelte/icons/database')).default }),
		ScrollText: async () => ({ default: (await import('lucide-svelte/icons/scroll-text')).default }),
		Shield: async () => ({ default: (await import('lucide-svelte/icons/shield')).default }),
		Trash2: async () => ({ default: (await import('lucide-svelte/icons/trash-2')).default }),
		Key: async () => ({ default: (await import('lucide-svelte/icons/key')).default }),
		AlertTriangle: async () => ({ default: (await import('lucide-svelte/icons/alert-triangle')).default }),
		Settings: async () => ({ default: (await import('lucide-svelte/icons/settings')).default }),
		Flag: async () => ({ default: (await import('lucide-svelte/icons/flag')).default }),
		Building2: async () => ({ default: (await import('lucide-svelte/icons/building-2')).default }),
		DollarSign: async () => ({ default: (await import('lucide-svelte/icons/dollar-sign')).default }),
		Clock: async () => ({ default: (await import('lucide-svelte/icons/clock')).default }),
	};

	let { children, data } = $props();

	let sidebarOpen = $state(false);
	let userPermissions = $state<string[]>([]);

	const currentPath = $derived(page.url.pathname);
	const isAuthPage = $derived(currentPath === '/login' || currentPath.startsWith('/logout'));

	// Initialize i18n with server-provided translations, or fetch client-side
	$effect(() => {
		const locale = data.locale ?? 'en';
		if (data.translations && Object.keys(data.translations).length > 0) {
			initLocale(locale, data.translations);
		} else {
			loadTranslations(locale);
		}
	});

	$effect(() => {
		if (data.user) {
			setUser(data.user);
			const perms = data.user.permissions ?? [];
			setPermissions(perms);
			userPermissions = perms;
		} else {
			clearUser();
			userPermissions = [];
		}
	});

	$effect(() => {
		setToken(data.token ?? null);
	});

	interface NavItem {
		label: string;
		href: string;
		iconName: string;
		permission: string;
		tooltip: string;
	}

	const navConfig: Omit<NavItem, 'label' | 'tooltip'>[] = [
		{ href: '/', iconName: 'LayoutDashboard', permission: 'control.dashboard' },
		{ href: '/queue', iconName: 'ListTodo', permission: 'control.queue' },
		{ href: '/dlq', iconName: 'AlertTriangle', permission: 'control.queue' },
		{ href: '/db', iconName: 'Database', permission: 'control.db' },
		{ href: '/audit', iconName: 'ScrollText', permission: 'control.audit' },
		{ href: '/security', iconName: 'Shield', permission: 'control.security' },
		{ href: '/api-keys', iconName: 'Key', permission: 'control.api_keys' },
		{ href: '/cache', iconName: 'Trash2', permission: 'control.cache' },
		{ href: '/settings', iconName: 'Settings', permission: 'control.settings' },
		{ href: '/feature-flags', iconName: 'Flag', permission: 'control.feature_flags' },
		{ href: '/tenants', iconName: 'Building2', permission: 'control.tenants' },
		{ href: '/ai-budget', iconName: 'DollarSign', permission: 'control.ai_budget' },
		{ href: '/cron', iconName: 'Clock', permission: 'control.cron' },
	];

	const navKeyMap: Record<string, { label: string; tooltip: string }> = {
		'/': { label: 'nav.dashboard', tooltip: 'System health overview' },
		'/queue': { label: 'nav.queue', tooltip: 'Background job queue stats' },
		'/dlq': { label: 'nav.dlq', tooltip: 'Failed job retry management' },
		'/db': { label: 'nav.db_health', tooltip: 'PostgreSQL health & metrics' },
		'/audit': { label: 'nav.audit', tooltip: 'Security audit log trail' },
		'/security': { label: 'nav.security', tooltip: 'Scan findings & IP blocks' },
		'/api-keys': { label: 'nav.api_keys', tooltip: 'Developer API key management' },
		'/cache': { label: 'nav.cache', tooltip: 'Cache stats & flush controls' },
		'/settings': { label: 'nav.settings', tooltip: 'System configuration' },
		'/feature-flags': { label: 'nav.feature_flags', tooltip: 'Feature flag toggles' },
		'/tenants': { label: 'nav.tenants', tooltip: 'Tenant organization management' },
		'/ai-budget': { label: 'nav.ai_budget', tooltip: 'AI cost monitoring & limits' },
		'/cron': { label: 'nav.cron', tooltip: 'Scheduled task management' },
	};

	const nav = $derived.by(() => {
		// Read translations store to establish reactive dependency
		const tr = $translations;
		return navConfig.map((cfg) => ({
			...cfg,
			label: tr[navKeyMap[cfg.href].label] ?? navKeyMap[cfg.href].label,
			tooltip: navKeyMap[cfg.href].tooltip,
		}));
	});

	let resolvedIcons: Record<string, Component> = $state({});
	$effect(() => {
		for (const item of nav) {
			if (!resolvedIcons[item.iconName]) {
				const loader = iconModules[item.iconName];
				if (loader) {
					loader().then((mod) => { resolvedIcons[item.iconName] = mod.default; });
				}
			}
		}
	});

	const pagePermission = $derived(
		nav.find((item) =>
			item.href === '/' ? currentPath === '/' : currentPath.startsWith(item.href)
		)?.permission
	);

	const userPerms = $derived(data.user?.permissions ?? []);
	const canAccess = $derived(
		!pagePermission || userPerms.includes(pagePermission) || userPerms.includes('*')
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
					{@const IconComponent = resolvedIcons[item.iconName]}
					<Tooltip.Root delayDuration={200}>
						<Tooltip.Trigger>
							<a
								href={item.href}
								class="hover:bg-dark-04 data-[state=active]:bg-accent flex items-center gap-3 rounded-9px px-3 py-2.5 text-base font-medium transition-colors {isActive ? 'bg-accent text-accent-foreground' : 'text-foreground-alt'}"
								onclick={() => (sidebarOpen = false)}
							>
								{#if IconComponent}
									<IconComponent class="size-4" />
								{:else}
									<span class="size-4"></span>
								{/if}
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
