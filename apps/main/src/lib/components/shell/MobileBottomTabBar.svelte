<script lang="ts">
	import { page } from '$app/state';
	import { Home, Search, Plus, Bell, User } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { realtime } from '$lib/realtime/realtime.svelte';
	import { compose } from './compose.svelte';
	import UserDropdown from './UserDropdown.svelte';
	import { resolveIcon } from '$lib/components/ui';
	import type { IconComponent } from '$lib/components/ui/icons';
	import type { SessionUser, Menu } from '$lib/types';
	import { routeFeatureGateVisible } from './routeFeatureFlags';

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

	const unread = $derived(realtime.unreadNotifications);
	const notificationsEnabled = $derived(features['feature.notifications'] === true);
	// The signed-in account tab highlights while on any account surface
	// (own profile, settings, dashboard) rather than only /u/{username}.
	const accountActive = $derived(
		page.url.pathname.startsWith(`/u/${user?.username}`) ||
			page.url.pathname.startsWith('/settings') ||
			page.url.pathname.startsWith('/dashboard')
	);

	type NavEntry =
		| { kind: 'link'; href: string; icon: IconComponent; label: string; notifications?: boolean }
		| { kind: 'account' }
		| { kind: 'compose' };

	// Admin-managed items take precedence when configured. The literal url
	// `compose` marks the compose slot; notifications items get the unread
	// badge when their url targets /notifications.
	const entries = $derived.by<NavEntry[]>(() => {
		const items = (menu?.items ?? []).filter(
			(i) =>
				i.status === 'active' &&
				(i.visibility === 'all' || (i.visibility === 'authenticated' ? !!user : !user)) &&
				routeFeatureGateVisible(i.url, features)
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
		// Account — the account tab opens the same dropdown as the desktop
		// sidebar avatar (Profile, Dashboard, Settings, Logout) since the
		// right-sidebar topbar is hidden on mobile. Notifications is
		// auth-gated and hidden when its feature flag is disabled.
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
			user
				? ({ kind: 'account' } as NavEntry)
				: ({ kind: 'link', href: '/login', icon: User, label: t('nav.profile', 'Profile') } as NavEntry)
		];
	});

	function isActive(href: string): boolean {
		const path = page.url.pathname;
		if (href === '/') return path === '/';
		if (!href.startsWith('/')) return false;
		return path === href || path.startsWith(href + '/');
	}

	// Section 09 Product Tour: map tab-bar URLs to the same per-feature tour
	// targets as the desktop LeftNav so mobile users get the same steps.
	function tabTourKey(href: string): string | undefined {
		if (href === '/') return 'home';
		if (href === '/search') return 'search';
		if (href === '/notifications') return 'notifications';
		if (href === '/profile' || href.startsWith('/u/')) return 'account';
		return undefined;
	}
</script>

<nav
	class="fixed inset-x-0 bottom-0 z-40 flex items-stretch border-t border-[var(--color-border)] bg-[var(--color-surface)]/95 pb-[calc(0.5rem+env(safe-area-inset-bottom))] backdrop-blur sm:hidden"
	aria-label={t('nav.main', 'Main navigation')}
	data-tour="primary-nav"
>
	{#each entries as entry (entry.kind === 'link' ? entry.href : entry.kind)}
		{#if entry.kind === 'account'}
			<UserDropdown {user} variant="tab" active={accountActive} />
		{:else if entry.kind === 'compose'}
			<!-- Compose button (center, elevated) — guests get a login link with
			     returnTo instead of opening the auth-gated composer modal. The
			     data-tour hook lets the product tour (§09) highlight compose on
			     mobile, where the left nav is hidden. -->
			{#if user}
				<button
					data-tour="compose"
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
					data-tour="compose"
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
				data-tour={tabTourKey(entry.href)}
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
							class="absolute -end-2 -top-1.5 grid h-4 min-w-4 place-items-center rounded-full bg-[var(--color-primary)] px-1 text-[9px] font-semibold text-[var(--color-primary-contrast)]"
							aria-label={t('a11y.unread_count', '{count} unread', { count: unread })}
						>{unread > 99 ? '99+' : unread}</span>
					{/if}
				</span>
				<span class="leading-none">{entry.label}</span>
			</a>
		{/if}
	{/each}
</nav>
