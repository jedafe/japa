<script lang="ts">
	import { page } from '$app/state';
	import { Home, Search, Plus, Bell, User } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { realtime } from '$lib/realtime/realtime.svelte';
	import type { SessionUser } from '$lib/types';
	import ComposeModal from './ComposeModal.svelte';

	interface Props {
		user: SessionUser | null;
	}

	let { user }: Props = $props();

	let composeOpen = $state(false);

	const profileHref = $derived(user ? `/u/${user.username}` : '/login');
	const unread = $derived(realtime.unreadNotifications);

	const tabs = $derived([
		{ href: '/', icon: Home, label: t('nav.home', 'Home'), active: page.url.pathname === '/' },
		{ href: '/search', icon: Search, label: t('nav.search', 'Search'), active: page.url.pathname.startsWith('/search') },
		{ href: '/notifications', icon: Bell, label: t('nav.notifications', 'Notifications'), active: page.url.pathname.startsWith('/notifications') },
		{ href: profileHref, icon: User, label: t('nav.profile', 'Profile'), active: page.url.pathname.startsWith('/u/') }
	]);
</script>

<ComposeModal bind:open={composeOpen} />

<nav
	class="fixed inset-x-0 bottom-0 z-40 flex items-stretch border-t border-[var(--color-border)] bg-[var(--color-surface)]/95 backdrop-blur sm:hidden"
	aria-label={t('nav.main', 'Main navigation')}
>
	{#each tabs as tab (tab.href)}
		<a
			href={tab.href}
			class="relative flex flex-1 flex-col items-center justify-center gap-0.5 py-2 text-[10px] {tab.active
				? 'text-[var(--color-primary)]'
				: 'text-[var(--color-muted)]'}"
			aria-current={tab.active ? 'page' : undefined}
		>
			<span class="relative">
				<tab.icon size={22} />
				{#if tab.href === '/notifications' && unread > 0}
					<span
						class="absolute -right-2 -top-1.5 grid h-4 min-w-4 place-items-center rounded-full bg-[var(--color-primary)] px-1 text-[9px] font-semibold text-[var(--color-primary-contrast)]"
						aria-label="{unread} unread"
					>{unread > 99 ? '99+' : unread}</span>
				{/if}
			</span>
			<span class="leading-none">{tab.label}</span>
		</a>
	{/each}

	<!-- Compose button (center, elevated) -->
	<button
		type="button"
		onclick={() => (composeOpen = true)}
		class="flex flex-1 flex-col items-center justify-center gap-0.5 py-2 text-[10px] text-[var(--color-primary)]"
		aria-label={t('nav.compose', 'Compose')}
	>
		<span
			class="grid h-9 w-9 place-items-center rounded-full bg-[var(--color-primary)] text-[var(--color-primary-contrast)] shadow-sm"
		>
			<Plus size={20} />
		</span>
		<span class="leading-none">{t('nav.compose', 'Compose')}</span>
	</button>
</nav>
