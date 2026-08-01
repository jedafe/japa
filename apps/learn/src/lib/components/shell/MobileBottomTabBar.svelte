<script lang="ts">
	// Learn mobile bottom bar — learning-oriented tabs (no compose button).
	// Ref: ADR-058 (mobile single-column + bottom bar), ADR-060 (Learn nav).
	import { page } from '$app/state';
	import { Compass, LayoutDashboard, Trophy, Bookmark, User } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { loginUrl, profileUrl } from '$lib/appOrigins';
	import type { SessionUser } from '$lib/types';

	interface Props {
		user: SessionUser | null;
	}
	let { user }: Props = $props();

	const profileHref = $derived(user ? profileUrl(user.username) : loginUrl());

	const tabs = $derived([
		{ href: '/', icon: Compass, label: t('learn.explore'), active: page.url.pathname === '/' },
		{ href: '/my-learning', icon: LayoutDashboard, label: t('learn.my_learning'), active: page.url.pathname.startsWith('/my-learning') },
		{ href: '/certificates', icon: Trophy, label: t('learn.certificates'), active: page.url.pathname.startsWith('/certificates') },
		{ href: '/bookmarks', icon: Bookmark, label: t('learn.bookmarks'), active: page.url.pathname.startsWith('/bookmarks') },
		{ href: profileHref, icon: User, label: t('nav.profile'), active: page.url.pathname.startsWith('/u/') }
	]);
</script>

<nav
	class="fixed inset-x-0 bottom-0 z-40 flex items-stretch border-t border-[var(--color-border)] bg-[var(--color-surface)]/95 backdrop-blur sm:hidden"
	aria-label={t('a11y.main_nav')}
>
	{#each tabs as tab (tab.href)}
		<a
			href={tab.href}
			class="relative flex flex-1 flex-col items-center justify-center gap-0.5 py-2 text-[10px] {tab.active
				? 'text-[var(--color-primary)]'
				: 'text-[var(--color-muted)]'}"
			aria-current={tab.active ? 'page' : undefined}
		>
			<tab.icon size={22} />
			<span class="leading-none">{tab.label}</span>
		</a>
	{/each}
</nav>
