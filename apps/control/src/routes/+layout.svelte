<script lang="ts">
	import { page } from '$app/state';
	import { setUser, setPermissions, clearUser, hasPermission } from '$lib/stores/auth';
	import { initGlitchTip } from '$lib/glitchtip';
	import TopBar from '$lib/components/layout/TopBar.svelte';
	import AccessDenied from '$lib/components/AccessDenied.svelte';
	import '../app.css';
	import {
		LayoutDashboard,
		ListTodo,
		Database,
		ScrollText,
		Shield,
		Trash2,
		Key,
		Menu,
		X,
		AlertTriangle,
		Settings,
		Flag,
		Building2,
		DollarSign,
		Clock,
	} from 'lucide-svelte';

	let { children, data } = $props();

	$effect(() => {
		initGlitchTip();
	});

	let sidebarOpen = $state(false);

	const currentPath = $derived(page.url.pathname);

	$effect(() => {
		if (data.user) {
			setUser(data.user);
			setPermissions(data.user.permissions ?? []);
		} else {
			clearUser();
		}
	});

	const nav = $derived([
		{ label: 'Dashboard', href: '/', icon: LayoutDashboard, permission: 'control.dashboard' },
		{ label: 'Queue', href: '/queue', icon: ListTodo, permission: 'control.queue' },
		{ label: 'Dead Letter Queue', href: '/dlq', icon: AlertTriangle, permission: 'control.queue' },
		{ label: 'DB Health', href: '/db', icon: Database, permission: 'control.db' },
		{ label: 'Audit', href: '/audit', icon: ScrollText, permission: 'control.audit' },
		{ label: 'Security', href: '/security', icon: Shield, permission: 'control.security' },
		{ label: 'API Keys', href: '/api-keys', icon: Key, permission: 'control.api_keys' },
		{ label: 'Cache', href: '/cache', icon: Trash2, permission: 'control.cache' },
		{ label: 'Settings', href: '/settings', icon: Settings, permission: 'control.settings' },
		{ label: 'Feature Flags', href: '/feature-flags', icon: Flag, permission: 'control.feature_flags' },
		{ label: 'Tenants', href: '/tenants', icon: Building2, permission: 'control.tenants' },
		{ label: 'AI Budget', href: '/ai-budget', icon: DollarSign, permission: 'control.ai_budget' },
		{ label: 'Cron Jobs', href: '/cron', icon: Clock, permission: 'control.cron' },
	]);

	const pagePermission = $derived(
		nav.find((item) =>
			item.href === '/' ? currentPath === '/' : currentPath.startsWith(item.href)
		)?.permission
	);

	const canAccess = $derived(!pagePermission || hasPermission(pagePermission) || hasPermission('*'));
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
				<a
					href={item.href}
					class="flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors {isActive ? 'bg-blue-50 text-blue-600 dark:bg-blue-900/20 dark:text-blue-400' : 'text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700'}"
					onclick={() => (sidebarOpen = false)}
				>
					<item.icon class="h-4 w-4" />
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
