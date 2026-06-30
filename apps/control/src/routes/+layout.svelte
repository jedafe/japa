<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { setToken } from '$lib/api';
	import { locale, t } from '$lib/stores/i18n.svelte';
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
	 LogOut
	} from 'lucide-svelte';

	let { children, data } = $props();

	let sidebarOpen = $state(false);

	const currentPath = $derived(page.url.pathname);

	$effect(() => {
		locale.init(data.locale, data.translations);
	});

	onMount(() => {
		if (data.token) {
			setToken(data.token);
		}
	});

	function logout() {
		document.cookie = 'control_token=; path=/; max-age=0';
		setToken(null);
		window.location.href = '/login';
	}

	 const nav = $derived([
	 { label: t('nav.dashboard'), href: '/', icon: LayoutDashboard },
	 { label: t('nav.queue'), href: '/queue', icon: ListTodo },
	 { label: t('nav.db_health'), href: '/db', icon: Database },
	 { label: t('nav.audit'), href: '/audit', icon: ScrollText },
	 { label: t('nav.security'), href: '/security', icon: Shield },
	 { label: t('nav.api_keys'), href: '/api-keys', icon: Key },
	 { label: t('nav.cache'), href: '/cache', icon: Trash2 }
	]);
</script>

<svelte:head>
	<title>JapaConnect Control</title>
</svelte:head>

<div class="flex h-screen overflow-hidden">
	<!-- Mobile toggle -->
	<button
		class="fixed left-4 top-4 z-50 rounded-md p-2 md:hidden"
		style="background-color: var(--color-surface); border: 1px solid var(--color-border)"
		onclick={() => (sidebarOpen = !sidebarOpen)}
	>
		{#if sidebarOpen}
			<X class="h-5 w-5" />
		{:else}
			<Menu class="h-5 w-5" />
		{/if}
	</button>

	<!-- Sidebar -->
	<aside
		class="fixed inset-y-0 left-0 z-40 w-60 flex-shrink-0 flex-col border-r transition-transform md:static md:translate-x-0"
		style="background-color: var(--color-surface); border-color: var(--color-border)"
		class:translate-x-0={sidebarOpen}
		class:-translate-x-full={!sidebarOpen}
	>
		<div class="flex h-14 items-center border-b px-4" style="border-color: var(--color-border)">
			<span class="text-sm font-bold tracking-wide uppercase" style="color: var(--color-primary)">{t('control.brand')}</span>
		</div>
		<nav class="flex-1 space-y-1 p-3">
			{#each nav as item}
				{@const isActive = item.href === '/' ? currentPath === '/' : currentPath.startsWith(item.href)}
				<a
					href={item.href}
					class="flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors {isActive ? 'bg-black/5 dark:bg-white/10' : 'hover:bg-black/5 dark:hover:bg-white/10'}"
					style={isActive ? `color: var(--color-primary)` : undefined}
					onclick={() => (sidebarOpen = false)}
				>
					<item.icon class="h-4 w-4" style={isActive ? `color: var(--color-primary)` : `color: var(--color-muted)`} />
					{item.label}
				</a>
			{/each}
		</nav>
		<div class="border-t p-3" style="border-color: var(--color-border)">
			<p class="mb-2 text-xs" style="color: var(--color-muted)">{data.user?.displayName ?? 'super_admin'}</p>
			<button
				class="flex w-full items-center gap-2 rounded-md px-3 py-1.5 text-xs font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
				onclick={logout}
			>
				<LogOut class="h-3.5 w-3.5" style="color: var(--color-muted)" />
				{t('nav.sign_out')}
			</button>
		</div>
	</aside>

	<!-- Backdrop -->
	{#if sidebarOpen}
		<button
			class="fixed inset-0 z-30 bg-black/50 md:hidden"
			aria-label="Close sidebar"
			onclick={() => (sidebarOpen = false)}
		></button>
	{/if}

	<!-- Main -->
	<main class="flex-1 overflow-y-auto">
		<div class="mx-auto max-w-7xl px-4 py-6 sm:px-6 md:px-8">
			{@render children()}
		</div>
	</main>
</div>
