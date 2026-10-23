<script lang="ts">
	import { Menu, MenuItem, MenuSeparator } from '@japaconnect/ui';
	import {
		User,
		LayoutDashboard,
		Bookmark,
		Trophy,
		Settings,
		LogOut,
		LogIn,
		GraduationCap,
		Compass
	} from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { loginUrl, profileUrl, MAIN_ORIGIN } from '$lib/appOrigins';
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
			aria-label={t('nav.account')}
		>
			{#if user?.avatar_url}
				<img src={user.avatar_url} alt="" class="h-9 w-9 rounded-full object-cover" />
			{:else if user}
				{user.displayName?.charAt(0)?.toUpperCase() ?? '?'}
			{:else}
				<User size={18} />
			{/if}
		</button>
	{/snippet}
	{#if user}
		<MenuItem href={profileUrl(user.username)} icon={User}>{t('nav.profile')}</MenuItem>
		<MenuItem href="/my-learning" icon={LayoutDashboard}>{t('academy.my_learning')}</MenuItem>
		<MenuItem href="/certificates" icon={Trophy}>{t('academy.certificates')}</MenuItem>
		<MenuItem href="/bookmarks" icon={Bookmark}>{t('academy.bookmarks')}</MenuItem>
		<MenuItem href="/dashboard/instructor" icon={GraduationCap}>{t('academy.dashboard')}</MenuItem>
		<MenuItem href={`${MAIN_ORIGIN}/settings`} icon={Settings}>{t('nav.settings')}</MenuItem>
		<MenuItem href="/?tour=replay" icon={Compass}>{t('nav.replay_tour')}</MenuItem>
		<MenuSeparator />
		<MenuItem href={`${MAIN_ORIGIN}/logout`} icon={LogOut} iconClass="rtl:-scale-x-100">{t('nav.logout')}</MenuItem>
	{:else}
		<MenuItem href={loginUrl()} icon={LogIn} iconClass="rtl:-scale-x-100">{t('nav.login')}</MenuItem>
		<MenuItem href={`${MAIN_ORIGIN}/signup`} icon={User}>{t('nav.signup')}</MenuItem>
	{/if}
</Menu>
