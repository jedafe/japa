<script lang="ts">
	import { page } from '$app/state';
	import { X, Home, Grid3X3, Search, Bell, User, Settings, LogOut } from 'lucide-svelte';
	import { t } from '$lib/stores/i18n.svelte';

	let {
		open = false,
		user = null,
		onclose = () => {}
	}: {
		open: boolean;
		user: any;
		onclose: () => void;
	} = $props();

	const navLinks = $derived([
		{ href: '/', icon: Home, label: t('nav.home') },
		{ href: '/spaces', icon: Grid3X3, label: t('nav.spaces') },
		{ href: '/search', icon: Search, label: t('nav.search') },
		{ href: '/notifications', icon: Bell, label: t('nav.notifications') }
	]);

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') onclose();
	}
</script>

<svelte:window onkeydown={handleKeydown} />

{#if open}
	<!-- Backdrop -->
	<button
		class="fixed inset-0 z-50 bg-black/50 md:hidden"
		onclick={onclose}
		aria-label="Close navigation"
	></button>

	<!-- Panel -->
	<div
		class="fixed inset-y-0 left-0 z-50 flex w-72 flex-col md:hidden"
		style="background-color: var(--color-background)"
		role="dialog"
		aria-label="Navigation"
	>
		<!-- Header -->
		<div
			class="flex h-14 items-center justify-between border-b px-4"
			style="border-color: var(--color-border)"
		>
			<span class="text-lg font-bold" style="color: var(--color-primary)">JapaConnect</span>
			<button class="rounded-md p-1 hover:bg-black/5 dark:hover:bg-white/10" onclick={onclose} aria-label="Close">
				<X class="h-5 w-5" />
			</button>
		</div>

		<!-- Links -->
		<nav class="flex-1 overflow-y-auto py-4" aria-label="Side navigation">
			{#each navLinks as link}
				{@const isActive = page.url.pathname === link.href || (link.href !== '/' && page.url.pathname.startsWith(link.href))}
				<a
					href={link.href}
					class="flex items-center gap-3 px-4 py-3 text-sm font-medium transition-colors"
					style="color: {isActive ? 'var(--color-primary)' : 'var(--color-foreground)'}; background-color: {isActive ? 'var(--color-surface)' : 'transparent'}"
					onclick={onclose}
				>
					<link.icon class="h-5 w-5" />
					{link.label}
				</a>
			{/each}

			{#if user}
				<hr class="my-4" style="border-color: var(--color-border)" />
				<a
					href="/settings"
					class="flex items-center gap-3 px-4 py-3 text-sm font-medium"
					style="color: var(--color-foreground)"
					onclick={onclose}
				>
					<Settings class="h-5 w-5" />
					{t('nav.settings')}
				</a>
				<a
					href="/auth/logout"
					class="flex items-center gap-3 px-4 py-3 text-sm font-medium"
					style="color: var(--color-foreground)"
					onclick={onclose}
				>
					<LogOut class="h-5 w-5" />
					{t('nav.logout')}
				</a>
			{/if}
		</nav>
	</div>
{/if}
