<script lang="ts">
	import { page } from '$app/state';
	import { Home, Search, Plus, Bell, User } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { realtime } from '$lib/realtime/realtime.svelte';
	import { compose } from './compose.svelte';
	import { resolveIcon } from '$lib/components/ui';
	import type { IconComponent } from '$lib/components/ui/icons';
	import type { SessionUser, Menu } from '$lib/types';

	interface Props {
		user: SessionUser | null;
		/** Feature flags from /feature-flags — used to hide the notifications
		 *  tab when feature.notifications is disabled. */
		features?: Record<string, boolean>;
		/** Admin-managed menu (bottom_bar placement, fetched as
		 *  /menus/mobile_bottom_bar in the root layout). When it has active
		 *  items it replaces the built-in tab set. An item whose url is the
		 *  literal `compose` renders the elevated compose button in place. */
		menu?: Menu | null;
	}

	let { user, features = {}, menu = null }: Props = $props();

	const profileHref = $derived(user ? `/u/${user.username}` : '/login');
	const unread = $derived(realtime.unreadNotifications);
	const notificationsEnabled = $derived(features['feature.notifications'] === true);

	type NavEntry =
		| { kind: 'link'; href: string; icon: IconComponent; label: string; notifications?: boolean }
		| { kind: 'compose' };

	// Admin-managed items take precedence when configured. The literal url
	// `compose` marks the compose slot; notifications items get the unread
	// badge when their url targets /notifications.
	const entries = $derived.by<NavEntry[]>(() => {
		const items = (menu?.items ?? []).filter(
			(i) =>
				i.status === 'active' &&
				(i.visibility === 'all' || (i.visibility === 'authenticated' ? !!user : !user))
		);
		if (items.length > 0) {
			return items.map<NavEntry>((i) =>
				i.url === 'compose'
					? { kind: 'compose' }
					: {
							kind: 'link',
							href: i.url,
							icon: resolveIcon(i.icon),
							label: i.label,
							notifications: i.url === '/notifications' || i.url.startsWith('/notifications/')
						}
			);
		}

		// Built-in fallback: Home, Search, Compose (center), Notifications,
		// Profile — notifications is auth-gated and hidden when its feature
		// flag is disabled so no trace of it leaks into the mobile tab bar.
		return [
			{ kind: 'link', href: '/', icon: Home, label: t('nav.home', 'Home') },
			{ kind: 'link', href: '/search', icon: Search, label: t('nav.search', 'Search') },
			{ kind: 'compose' },
			...(notificationsEnabled
				? ([
						{
							kind: 'link',
							href: user ? '/notifications' : '/login?redirect=/notifications',
							icon: Bell,
							label: t('nav.notifications', 'Notifications'),
							notifications: true
						}
					] as NavEntry[])
				: []),
			{ kind: 'link', href: profileHref, icon: User, label: t('nav.profile', 'Profile') }
		];
	});

	function isActive(href: string): boolean {
		const path = page.url.pathname;
		if (href === '/') return path === '/';
		if (!href.startsWith('/')) return false;
		return path === href || path.startsWith(href + '/');
	}
</script>

<nav
	class="fixed inset-x-0 bottom-0 z-40 flex items-stretch border-t border-[var(--color-border)] bg-[var(--color-surface)]/95 pb-[calc(0.5rem+env(safe-area-inset-bottom))] backdrop-blur sm:hidden"
	aria-label={t('nav.main', 'Main navigation')}
>
	{#each entries as entry (entry.kind === 'link' ? entry.href : 'compose')}
		{#if entry.kind === 'compose'}
			<!-- Compose button (center, elevated) — guests get a login link with
			     returnTo instead of opening the auth-gated composer modal. -->
			{#if user}
				<button
					type="button"
					onclick={() => compose.openMenu()}
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
			{:else}
				<a
					href="/login?redirect=/create"
					class="flex flex-1 flex-col items-center justify-center gap-0.5 py-2 text-[10px] text-[var(--color-primary)]"
					aria-label={t('nav.compose', 'Compose')}
				>
					<span
						class="grid h-9 w-9 place-items-center rounded-full bg-[var(--color-primary)] text-[var(--color-primary-contrast)] shadow-sm"
					>
						<Plus size={20} />
					</span>
					<span class="leading-none">{t('nav.compose', 'Compose')}</span>
				</a>
			{/if}
		{:else}
			<a
				href={entry.href}
				class="relative flex flex-1 flex-col items-center justify-center gap-0.5 py-2 text-[10px] {isActive(
					entry.href
				)
					? 'text-[var(--color-primary)]'
					: 'text-[var(--color-muted)]'}"
				aria-current={isActive(entry.href) ? 'page' : undefined}
			>
				<span class="relative">
					<entry.icon size={22} />
					{#if entry.notifications && unread > 0}
						<span
							class="absolute -right-2 -top-1.5 grid h-4 min-w-4 place-items-center rounded-full bg-[var(--color-primary)] px-1 text-[9px] font-semibold text-[var(--color-primary-contrast)]"
							aria-label="{unread} unread"
						>{unread > 99 ? '99+' : unread}</span>
					{/if}
				</span>
				<span class="leading-none">{entry.label}</span>
			</a>
		{/if}
	{/each}
</nav>
