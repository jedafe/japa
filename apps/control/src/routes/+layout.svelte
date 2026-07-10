<script lang="ts">
	import { page } from '$app/state';
	import { setUser, setPermissions, clearUser } from '$lib/stores/auth';
	import TopBar from '$lib/components/layout/TopBar.svelte';
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
	} from 'lucide-svelte';

	let { children, data } = $props();

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
		{ label: 'Dashboard', href: '/', icon: LayoutDashboard },
		{ label: 'Queue', href: '/queue', icon: ListTodo },
		{ label: 'DB Health', href: '/db', icon: Database },
		{ label: 'Audit', href: '/audit', icon: ScrollText },
		{ label: 'Security', href: '/security', icon: Shield },
		{ label: 'API Keys', href: '/api-keys', icon: Key },
		{ label: 'Cache', href: '/cache', icon: Trash2 }
	]);
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
		<nav class="flex-1 space-y-1 p-3">
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
				{@render children()}
			</div>
		</main>
	</div>
</div>
