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
	}

	let { user }: Props = $props();
</script>

<Menu contentClass="min-w-44">
	{#snippet trigger(props)}
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
