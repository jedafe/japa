<script lang="ts">
	import { page } from '$app/state';
	import { setUser, setPermissions, clearUser } from '$lib/stores/auth';
	import TopBar from '$lib/components/layout/TopBar.svelte';
	import AccessDenied from '$lib/components/AccessDenied.svelte';
	import '../app.css';
	import {
		Menu,
		X,
	} from 'lucide-svelte';
	// Dynamic icon imports — avoids lucide-svelte SSR module runner crash.
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

	const nav = $derived([
		{ label: 'Dashboard', href: '/', iconName: 'LayoutDashboard', permission: 'control.dashboard' },
		{ label: 'Queue', href: '/queue', iconName: 'ListTodo', permission: 'control.queue' },
		{ label: 'Dead Letter Queue', href: '/dlq', iconName: 'AlertTriangle', permission: 'control.queue' },
		{ label: 'DB Health', href: '/db', iconName: 'Database', permission: 'control.db' },
		{ label: 'Audit', href: '/audit', iconName: 'ScrollText', permission: 'control.audit' },
		{ label: 'Security', href: '/security', iconName: 'Shield', permission: 'control.security' },
		{ label: 'API Keys', href: '/api-keys', iconName: 'Key', permission: 'control.api_keys' },
		{ label: 'Cache', href: '/cache', iconName: 'Trash2', permission: 'control.cache' },
		{ label: 'Settings', href: '/settings', iconName: 'Settings', permission: 'control.settings' },
		{ label: 'Feature Flags', href: '/feature-flags', iconName: 'Flag', permission: 'control.feature_flags' },
		{ label: 'Tenants', href: '/tenants', iconName: 'Building2', permission: 'control.tenants' },
		{ label: 'AI Budget', href: '/ai-budget', iconName: 'DollarSign', permission: 'control.ai_budget' },
		{ label: 'Cron Jobs', href: '/cron', iconName: 'Clock', permission: 'control.cron' },
	]);

	// Resolve icon component from name
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

<div class="flex h-screen overflow-hidden bg-gray-50 dark:bg-gray-900">
	<button
		class="fixed left-4 top-4 z-50 rounded-md bg-white p-2 shadow-md md:hidden dark:bg-gray-800"
		onclick={() => (sidebarOpen = !sidebarOpen)}
	>
		{#if sidebarOpen}
			<X class="h-5 w-5" />
		{:else}
			<Menu class="h-5 w-5" />
		{/if}
	</button>

	<aside
		class="fixed inset-y-0 left-0 z-40 w-60 flex-shrink-0 flex-col border-r border-gray-200 bg-white transition-transform md:static md:translate-x-0 dark:border-gray-700 dark:bg-gray-800"
		class:translate-x-0={sidebarOpen}
		class:-translate-x-full={!sidebarOpen}
	>
		<div class="flex h-14 items-center border-b border-gray-200 px-4 dark:border-gray-700">
			<span class="text-sm font-bold tracking-wide uppercase text-blue-600">Control Panel</span>
		</div>
		<nav class="flex-1 space-y-1 overflow-y-auto p-3">
			{#each nav as item}
				{@const isActive = item.href === '/' ? currentPath === '/' : currentPath.startsWith(item.href)}
				{@const IconComponent = resolvedIcons[item.iconName]}
				<a
					href={item.href}
					class="flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors {isActive ? 'bg-blue-50 text-blue-600 dark:bg-blue-900/20 dark:text-blue-400' : 'text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700'}"
					onclick={() => (sidebarOpen = false)}
				>
					{#if IconComponent}
						<IconComponent class="h-4 w-4" />
					{:else}
						<span class="h-4 w-4"></span>
					{/if}
					{item.label}
				</a>
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
