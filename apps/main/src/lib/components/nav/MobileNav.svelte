<script lang="ts">
	import { page } from '$app/state';
	import { Home, Grid3X3, Search, Bell, User } from 'lucide-svelte';

	let { notificationCount = 0 }: { notificationCount: number } = $props();

	const tabs = [
		{ href: '/', icon: Home, label: 'Home' },
		{ href: '/spaces', icon: Grid3X3, label: 'Spaces' },
		{ href: '/search', icon: Search, label: 'Search' },
		{ href: '/notifications', icon: Bell, label: 'Alerts', badge: true },
		{ href: '/profile', icon: User, label: 'Profile' }
	];
</script>

<nav
	class="fixed bottom-0 left-0 right-0 z-40 border-t md:hidden"
	style="background-color: var(--color-background); border-color: var(--color-border)"
	aria-label="Mobile navigation"
>
	<div class="flex items-center justify-around py-2">
		{#each tabs as tab}
			{@const isActive = page.url.pathname === tab.href || (tab.href !== '/' && page.url.pathname.startsWith(tab.href))}
			{@const badgeCount = tab.label === 'Alerts' ? notificationCount : 0}
			<a
				href={tab.href}
				class="relative flex flex-col items-center gap-0.5 px-3 py-1"
				aria-label={tab.label}
				aria-current={isActive ? 'page' : undefined}
			>
				<tab.icon
					class="h-5 w-5 transition-colors"
					style="color: {isActive ? 'var(--color-primary)' : 'var(--color-muted)'}"
				/>
				<span
					class="text-[10px] font-medium"
					style="color: {isActive ? 'var(--color-primary)' : 'var(--color-muted)'}"
				>
					{tab.label}
				</span>
				{#if badgeCount > 0}
					<span
						class="absolute -right-0.5 -top-0.5 flex h-4 min-w-4 items-center justify-center rounded-full px-1 text-[10px] font-bold text-white"
						style="background-color: var(--color-primary)"
					>
						{badgeCount > 99 ? '99+' : badgeCount}
					</span>
				{/if}
			</a>
		{/each}
	</div>
</nav>
