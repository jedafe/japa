<script lang="ts">
	import { Menu, MenuItem, MenuSeparator } from '@japaconnect/ui';
	import {
		User,
		LayoutDashboard,
		Settings,
		LogOut,
		LogIn,
		Compass
	} from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { env } from '$env/dynamic/public';
	import { page } from '$app/state';
	import type { SessionUser } from '$lib/types';

	const mainOrigin = (env.PUBLIC_MAIN_ORIGIN ?? 'http://localhost:3000').replace(/\/$/, '');

	interface Props {
		user: SessionUser | null;
	}

	let { user }: Props = $props();

	// Recompute the redirect URL on client-side navigation so login/signup
	// return links point to the current docs page.
	let returnTo = $derived(encodeURIComponent(page.url.href));

	let initials = $derived(
		user?.displayName
			? user.displayName
					.split(' ')
					.map((p) => p[0])
					.slice(0, 2)
					.join('')
					.toUpperCase()
			: ''
	);
</script>

<div class="dropdown">
	<!-- Avatar dropdown menu (shared Menu wraps bits-ui — portals to body, keyboard accessible). -->
	<Menu contentClass="min-w-44">
		{#snippet trigger(props)}
			<button
				type="button"
				{...props}
				class="grid h-9 w-9 shrink-0 cursor-pointer place-items-center overflow-hidden rounded-full border border-[var(--color-border)] bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)] hover:outline hover:outline-2 hover:outline-[var(--color-border)] hover:outline-offset-1"
				aria-label={t('nav.account', 'Account')}
			>
				{#if user?.avatar_url}
					<img src={user.avatar_url} alt={user.displayName} class="h-full w-full object-cover" />
				{:else if user}
					<span class="grid h-full w-full place-items-center bg-[var(--color-primary)] text-xs font-semibold text-[var(--color-primary-contrast)]">{initials}</span>
				{:else}
					<User size={18} />
				{/if}
			</button>
		{/snippet}
		{#if user}
			<MenuItem href={`${mainOrigin}/u/${user.username}`} icon={User}>{t('nav.profile', 'Profile')}</MenuItem>
			<MenuItem href={`${mainOrigin}/dashboard`} icon={LayoutDashboard}>{t('nav.dashboard', 'Dashboard')}</MenuItem>
			<MenuItem href={`${mainOrigin}/settings/profile`} icon={Settings}>{t('nav.settings', 'Settings')}</MenuItem>
			<!-- Section 09 Product Tour: replay the docs walkthrough. -->
			<MenuItem href="/docs?tour=replay" icon={Compass}>{t('nav.replay_tour', 'Replay tour')}</MenuItem>
			<MenuSeparator />
			<MenuItem href={`${mainOrigin}/logout`} icon={LogOut}>{t('nav.logout', 'Logout')}</MenuItem>
		{:else}
			<MenuItem href={`${mainOrigin}/login?redirect=${returnTo}`} icon={LogIn}>{t('auth.login')}</MenuItem>
			<MenuItem href={`${mainOrigin}/signup?redirect=${returnTo}`} icon={User}>{t('auth.signup')}</MenuItem>
		{/if}
	</Menu>
</div>

<style>
	.dropdown {
		display: flex;
		align-items: center;
		gap: 0.375rem;
	}
</style>
