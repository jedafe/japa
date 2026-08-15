<script lang="ts">
	import clsx from 'clsx';
	import { page } from '$app/state';
	import { DropdownMenu } from 'bits-ui';
	import {
		PanelLeftClose,
		PanelLeftOpen,
		PenSquare,
		LogIn,
		Hash,
		ChevronsUpDown,
		ExternalLink
	} from 'lucide-svelte';
	import { resolveIcon } from '$lib/components/ui/icons';
	import NotificationCentre from './NotificationCentre.svelte';
	import { compose } from './compose.svelte';
	import { t } from '$lib/i18n';
	import { realtime } from '$lib/realtime/realtime.svelte';
	import type { Menu, MenuItem, SessionUser, UserSpaceRef } from '$lib/types';
	import { routeFeatureFlagForPath } from '$lib/components/shell/routeFeatureFlags';

	interface Props {
		menu: Menu | null;
		user: SessionUser | null;
		collapsed: boolean;
		userSpaces?: UserSpaceRef[];
		/** Platform logo URL from appearance.logo_url (control panel). */
		logoUrl?: string;
		/** Feature flags from /feature-flags — used to hide nav items for
		 *  disabled features so no trace of a toggled-off feature leaks. */
		features?: Record<string, boolean>;
		onToggle: () => void;
	}

	let { menu, user, collapsed, userSpaces = [], logoUrl = '', features = {}, onToggle }: Props = $props();

	// The API already filters by auth/permissions, but guard against visibility
	// drift so an 'authenticated' item never leaks to a guest and vice-versa.
	// Also filter by feature flags: if a nav item's route maps to a feature
	// flag that is disabled, the item is hidden entirely.
	const items = $derived(
		(menu?.items ?? []).filter((item: MenuItem) => {
			if (item.visibility === 'authenticated') return !!user;
			if (item.visibility === 'guest') return !user;
			const flag = routeFeatureFlagForPath(item.url);
			if (flag && features[flag] !== true) return false;
			return true;
		})
	);

	function isActive(url: string): boolean {
		if (url === '/') return page.url.pathname === '/';
		return page.url.pathname === url || page.url.pathname.startsWith(url + '/');
	}

	function itemClass(active: boolean): string {
		return clsx(
			'my-0.5 flex items-center gap-3 rounded-[var(--radius)] px-3 py-2 text-sm transition-colors',
			collapsed && 'justify-center',
			active
				? 'bg-[var(--color-surface-subtle)] font-medium text-[var(--color-primary)]'
				: 'text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]'
		);
	}
</script>

<nav
	class="flex h-full w-full flex-col border-r border-[var(--color-border)] bg-[var(--color-surface)]"
	aria-label={t('a11y.main_nav')}
>
	<!-- Logo + collapse toggle -->
	<div class={clsx('flex h-[var(--nav-height)] shrink-0 items-center gap-2 px-3', collapsed && 'justify-center')}>
		<a href="/" class="flex items-center gap-2 font-medium text-[var(--color-text)]">
			{#if logoUrl}
				<img src={logoUrl} alt={t('app.name')} class="h-8 w-auto max-w-[140px] shrink-0 object-contain" />
			{:else}
				<span
					class="grid h-8 w-8 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-primary)] text-sm font-semibold text-[var(--color-primary-contrast)]"
					>JC</span
				>
			{/if}
			{#if !collapsed && !logoUrl}<span>{t('app.name')}</span>{/if}
		</a>
		{#if !collapsed}
			<button
				type="button"
				onclick={onToggle}
				class="ml-auto grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
				aria-label={t('nav.collapse')}
			>
				<PanelLeftClose size={18} />
			</button>
		{/if}
	</div>

	{#if collapsed}
		<button
			type="button"
			onclick={onToggle}
			class="mx-auto mb-1 grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
			aria-label={t('nav.expand')}
		>
			<PanelLeftOpen size={18} />
		</button>
	{/if}

	<!-- Nav items (from GET /menus/main_sidebar) -->
	<ul class="flex-1 overflow-y-auto px-2 py-2" data-tour="primary-nav">
		{#each items as item (item.id)}
			{@const Icon = resolveIcon(item.icon)}
			{@const active = isActive(item.url)}
			{@const badge = item.url === '/notifications' ? realtime.unreadNotifications : 0}
			{@const isNotifications = item.url === '/notifications' && !!user}
			<li>
				{#if isNotifications}
					<!-- Phase 13.01 — Notifications nav item opens the dropdown -->
					<NotificationCentre side="right" align="start" {collapsed} class={itemClass(active)}>
						{#snippet children()}
							<span
								class={clsx('flex items-center gap-3', collapsed && 'justify-center')}
								title={collapsed ? item.label : undefined}
								aria-current={active ? 'page' : undefined}
							>
								<span class="relative">
									<Icon size={20} strokeWidth={active ? 2.2 : 1.8} />
									{#if badge > 0}
										<span
											class="absolute -right-1.5 -top-1.5 grid h-4 min-w-4 place-items-center rounded-full bg-[var(--color-primary)] px-1 text-[10px] font-semibold text-[var(--color-primary-contrast)]"
											aria-label="{badge} unread"
										>{badge > 99 ? '99+' : badge}</span>
									{/if}
								</span>
								{#if !collapsed}
									<span class="min-w-0 flex-1">{item.label}</span>
									{#if badge > 0}
										<span
											class="shrink-0 rounded-full bg-[var(--color-primary)] px-1.5 py-0.5 text-[10px] font-semibold text-[var(--color-primary-contrast)]"
										>{badge > 99 ? '99+' : badge}</span>
									{/if}
								{/if}
							</span>
						{/snippet}
					</NotificationCentre>
				{:else}
					<a
						href={item.url}
						target={item.target}
						title={collapsed ? item.label : undefined}
						aria-current={active ? 'page' : undefined}
						class={itemClass(active)}
					>
						<span class="relative">
							<Icon size={20} strokeWidth={active ? 2.2 : 1.8} />
							{#if badge > 0}
								<span
									class="absolute -right-1.5 -top-1.5 grid h-4 min-w-4 place-items-center rounded-full bg-[var(--color-primary)] px-1 text-[10px] font-semibold text-[var(--color-primary-contrast)]"
									aria-label="{badge} unread"
								>{badge > 99 ? '99+' : badge}</span>
							{/if}
						</span>
						{#if !collapsed}
							<span class="min-w-0 flex-1">{item.label}</span>
							{#if badge > 0}
								<span
									class="shrink-0 rounded-full bg-[var(--color-primary)] px-1.5 py-0.5 text-[10px] font-semibold text-[var(--color-primary-contrast)]"
								>{badge > 99 ? '99+' : badge}</span>
							{/if}
						{/if}
					</a>
				{/if}
			</li>
		{/each}
	</ul>

	<!-- Compose — guests are redirected to login with a returnTo so they land
	     on the composer after authenticating instead of hitting a 403.
	     Authed users get the modal composer (ADR-058). -->
	<div class="px-2 py-2" data-tour="compose">
		{#if collapsed}
			{#if user}
				<button
					type="button"
					onclick={() => compose.openMenu()}
					class="mx-auto grid h-11 w-11 place-items-center rounded-full bg-[var(--color-primary)] text-[var(--color-primary-contrast)] shadow-sm transition-colors hover:bg-[var(--color-primary-hover)]"
					aria-label={t('nav.compose')}
				>
					<PenSquare size={20} />
				</button>
			{:else}
				<a
					href="/login?redirect=/create"
					class="mx-auto grid h-11 w-11 place-items-center rounded-full bg-[var(--color-primary)] text-[var(--color-primary-contrast)] shadow-sm transition-colors hover:bg-[var(--color-primary-hover)]"
					aria-label={t('nav.compose')}
				>
					<PenSquare size={20} />
				</a>
			{/if}
		{:else}
			{#if user}
				<button
					type="button"
					onclick={() => compose.openMenu()}
					class="flex w-full items-center justify-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
				>
					<PenSquare size={18} />
					<span>{t('nav.compose')}</span>
				</button>
			{:else}
				<a
					href="/login?redirect=/create"
					class="flex w-full items-center justify-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
				>
					<PenSquare size={18} />
					<span>{t('nav.compose')}</span>
				</a>
			{/if}
		{/if}
	</div>

	<!-- Account / Space switcher (bottom) -->
	<div class="border-t border-[var(--color-border)] p-2" data-tour="space-switcher">
		{#if user && userSpaces.length > 0}
			<!-- Space switcher — ADR-058: stays in the left nav, not the account menu -->
			<DropdownMenu.Root>
				<DropdownMenu.Trigger
					class={clsx(
						'mb-1 flex w-full items-center gap-2 rounded-[var(--radius)] p-2 text-sm text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)]',
						collapsed && 'justify-center'
					)}
					aria-label={t('nav.space_switcher')}
				>
					<Hash size={18} class="shrink-0 text-[var(--color-primary)]" />
					{#if !collapsed}
						<span class="min-w-0 flex-1 truncate">{t('nav.space_switcher')}</span>
						<ChevronsUpDown size={14} class="shrink-0 text-[var(--color-muted)]" />
					{/if}
				</DropdownMenu.Trigger>
				<DropdownMenu.Portal>
					<DropdownMenu.Content
						side="right"
						align="start"
						sideOffset={8}
						class="z-50 min-w-52 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-lg"
					>
						{#each userSpaces as sp (sp.id)}
							<DropdownMenu.Item
								class="flex cursor-pointer items-center gap-2 rounded-[6px] px-3 py-2 text-sm text-[var(--color-text)] outline-none data-[highlighted]:bg-[var(--color-surface-subtle)]"
							>
								{#snippet child({ props })}
									<a href={`/spaces/${sp.slug}`} {...props}>
										<Hash size={14} class="shrink-0 text-[var(--color-muted)]" />
										<span class="min-w-0 flex-1 truncate">{sp.name}</span>
									</a>
								{/snippet}
							</DropdownMenu.Item>
						{/each}
						<DropdownMenu.Separator class="my-1 h-px bg-[var(--color-border)]" />
						<DropdownMenu.Item
							class="flex cursor-pointer items-center gap-2 rounded-[6px] px-3 py-2 text-sm font-medium text-[var(--color-primary)] outline-none data-[highlighted]:bg-[var(--color-surface-subtle)]"
						>
							{#snippet child({ props })}
								<a href="/discover?tab=spaces" {...props}>
									<ExternalLink size={14} />
									{t('nav.space_switcher_browse')}
								</a>
							{/snippet}
						</DropdownMenu.Item>
					</DropdownMenu.Content>
				</DropdownMenu.Portal>
			</DropdownMenu.Root>
		{/if}

		{#if user}
			<a
				href="/u/{user.username}"
				title={collapsed ? user.display_name : undefined}
				class={clsx('flex items-center gap-3 rounded-[var(--radius)] p-2 hover:bg-[var(--color-surface-subtle)]', collapsed && 'justify-center')}
			>
				{#if user.avatar_url}
					<img src={user.avatar_url} alt={`Avatar of ${user.display_name ?? user.username ?? "user"}`} class="h-8 w-8 shrink-0 rounded-full object-cover" />
				{:else}
					<span
						class="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-xs font-medium text-[var(--color-text-subtle)]"
						>{user.display_name?.charAt(0)?.toUpperCase() ?? '?'}</span
					>
				{/if}
				{#if !collapsed}
					<span class="min-w-0 flex-1">
						<span class="block truncate text-sm font-medium text-[var(--color-text)]">{user.display_name}</span>
						<span class="block truncate text-xs text-[var(--color-muted)]">@{user.username}</span>
					</span>
				{/if}
			</a>
		{:else}
			<a
				href="/login"
				class="flex items-center justify-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
			>
				<LogIn size={18} />
				{#if !collapsed}<span>{t('nav.account')}</span>{/if}
			</a>
		{/if}
	</div>
</nav>
