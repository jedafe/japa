<script lang="ts">
	import {
		User,
		LayoutDashboard,
		Bookmark,
		Wallet,
		Settings,
		LogOut,
		LogIn
	} from 'lucide-svelte';
	import { Menu, MenuItem, MenuSeparator } from '$lib/components/ui';
	import { t } from '$lib/i18n';
	import type { SessionUser } from '$lib/types';

	interface Props {
		user: SessionUser | null;
		/** 'icon' renders the standalone avatar button (sidebar topbar).
		 *  'tab' renders the avatar + label stacked like a bottom-tab-bar
		 *  entry — used for the mobile account tab. */
		variant?: 'icon' | 'tab';
		/** Active-tab styling for the 'tab' variant. */
		active?: boolean;
	}

	let { user, variant = 'icon', active = false }: Props = $props();
</script>

<Menu contentClass="min-w-44" side={variant === 'tab' ? 'top' : 'bottom'} align={variant === 'tab' ? 'center' : 'end'}>
	{#snippet trigger(props)}
		{#if variant === 'tab'}
			<button
				type="button"
				{...props}
				class="relative flex flex-1 flex-col items-center justify-center gap-0.5 py-2 text-[10px] {active ? 'text-[var(--color-primary)]' : 'text-[var(--color-muted)]'}"
				aria-label={t('nav.account', 'Account')}
			>
				{#if user?.avatar_url}
					<img src={user.avatar_url} alt={`Avatar of ${user.display_name ?? user.username ?? "user"}`} class="h-6 w-6 rounded-full object-cover" />
				{:else if user}
					<span class="grid h-6 w-6 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[10px] font-semibold text-[var(--color-text-subtle)]">
						{user.display_name?.charAt(0)?.toUpperCase() ?? '?'}
					</span>
				{:else}
					<User size={22} />
				{/if}
				<span class="leading-none">{t('nav.account', 'Account')}</span>
			</button>
		{:else}
			<button
				type="button"
				{...props}
				class="grid h-9 w-9 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)] hover:ring-2 hover:ring-[var(--color-border)]"
				aria-label={t('nav.account', 'Account')}
			>
				{#if user?.avatar_url}
					<img src={user.avatar_url} alt={`Avatar of ${user.display_name ?? user.username ?? "user"}`} class="h-9 w-9 rounded-full object-cover" />
				{:else if user}
					{user.display_name?.charAt(0)?.toUpperCase() ?? '?'}
				{:else}
					<User size={18} />
				{/if}
			</button>
		{/if}
	{/snippet}
	{#if user}
		<MenuItem href="/u/{user.username}" icon={User}>{t('nav.profile', 'Profile')}</MenuItem>
		<MenuItem href="/dashboard" icon={LayoutDashboard}>{t('nav.dashboard', 'Dashboard')}</MenuItem>
		<MenuItem href="/dashboard/bookmarks" icon={Bookmark}>{t('nav.bookmarks', 'Bookmarks')}</MenuItem>
		<MenuItem href="/settings/wallet" icon={Wallet}>{t('nav.wallet', 'Wallet')}</MenuItem>
		<MenuItem href="/settings/profile" icon={Settings}>{t('nav.settings', 'Settings')}</MenuItem>
		<MenuSeparator />
		<MenuItem href="/logout" icon={LogOut} iconClass="rtl:-scale-x-100">{t('nav.logout', 'Logout')}</MenuItem>
	{:else}
		<MenuItem href="/login" icon={LogIn} iconClass="rtl:-scale-x-100">{t('nav.login', 'Login')}</MenuItem>
		<MenuItem href="/signup" icon={User}>{t('nav.signup', 'Sign up')}</MenuItem>
	{/if}
</Menu>
