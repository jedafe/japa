<script lang="ts">
	import '../app.css';
	import {
	 LayoutDashboard,
	 Activity,
	 Database,
	 ScrollText,
	 Shield,
	 Trash2,
	 Menu,
	 X
	} from 'lucide-svelte';

	let { children } = $props();

	let sidebarOpen = $state(false);

	const nav = [
	 { label: 'Dashboard', href: '/', icon: LayoutDashboard },
	 { label: 'Queue', href: '/queue', icon: Activity },
	 { label: 'DB Health', href: '/db', icon: Database },
	 { label: 'Audit Log', href: '/audit', icon: ScrollText },
	 { label: 'Security', href: '/security', icon: Shield },
	 { label: 'Cache', href: '/cache', icon: Trash2 }
	];
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
			<span class="text-sm font-bold tracking-wide uppercase" style="color: var(--color-primary)">JapaControl</span>
		</div>
		<nav class="flex-1 space-y-1 p-3">
			{#each nav as item}
				<a
					href={item.href}
					class="flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
					onclick={() => (sidebarOpen = false)}
				>
					<item.icon class="h-4 w-4" style="color: var(--color-muted)" />
					{item.label}
				</a>
			{/each}
		</nav>
		<div class="border-t p-3" style="border-color: var(--color-border)">
			<p class="text-xs" style="color: var(--color-muted)">super_admin</p>
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
