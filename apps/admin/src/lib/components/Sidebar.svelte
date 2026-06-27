<script lang="ts">
	import {
		LayoutDashboard,
		Users,
		ShieldCheck,
		Settings,
		Ticket,
		Radio,
		Menu,
		X
	} from 'lucide-svelte';

	let {
		currentPath = '/'
	}: {
		currentPath?: string;
	} = $props();

	let open = $state(false);

	const nav = [
		{ label: 'Dashboard', href: '/', icon: LayoutDashboard },
		{ label: 'Users', href: '/users', icon: Users },
		{ label: 'Moderation', href: '/moderation', icon: ShieldCheck },
		{ label: 'Settings', href: '/settings', icon: Settings },
		{ label: 'Tickets', href: '/tickets', icon: Ticket },
		{ label: 'Broadcast', href: '/broadcast', icon: Radio }
	];
</script>

<!-- Mobile toggle -->
<button
	class="fixed left-4 top-4 z-50 rounded-md p-2 md:hidden"
	style="background-color: var(--color-surface); border: 1px solid var(--color-border)"
	onclick={() => (open = !open)}
	aria-label={open ? 'Close menu' : 'Open menu'}
>
	{#if open}
		<X class="h-5 w-5" />
	{:else}
		<Menu class="h-5 w-5" />
	{/if}
</button>

<!-- Sidebar -->
<aside
	class="fixed inset-y-0 left-0 z-40 w-60 flex-shrink-0 flex-col border-r transition-transform md:static md:translate-x-0"
	style="background-color: var(--color-surface); border-color: var(--color-border)"
	class:translate-x-0={open}
	class:-translate-x-full={!open}
>
	<div class="flex h-14 items-center border-b px-4" style="border-color: var(--color-border)">
		<span class="text-sm font-bold tracking-wide uppercase" style="color: var(--color-primary)">JapaAdmin</span>
	</div>
	<nav class="flex-1 space-y-1 p-3">
		{#each nav as item}
			<a
				href={item.href}
				class="flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
				onclick={() => (open = false)}
			>
				<item.icon class="h-4 w-4" style="color: var(--color-muted)" />
				{item.label}
			</a>
		{/each}
	</nav>
	<div class="border-t p-3" style="border-color: var(--color-border)">
		<p class="text-xs" style="color: var(--color-muted)">admin</p>
	</div>
</aside>

{#if open}
	<button
		class="fixed inset-0 z-30 bg-black/50 md:hidden"
		aria-label="Close sidebar"
		onclick={() => (open = false)}
	></button>
{/if}
