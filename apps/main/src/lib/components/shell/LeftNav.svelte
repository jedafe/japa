<script lang="ts">
	import clsx from 'clsx';
	import { page } from '$app/state';
	import { Menu as UiMenu, MenuItem as UiMenuItem, MenuSeparator as UiMenuSeparator } from '$lib/components/ui';
	import {
		PanelLeftOpen,
		PenSquare,
		Pin,
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
	import ThemeLogo from '$lib/components/branding/ThemeLogo.svelte';
	import { theme } from '$lib/stores/theme.svelte';
	import AdSlot from '$lib/components/ui/AdSlot.svelte';

	interface Props {
		menu: Menu | null;
		user: SessionUser | null;
		collapsed: boolean;
		userSpaces?: UserSpaceRef[];
		/** Platform logo URL from appearance.logo_url (control panel). */
		/** Platform dark-theme logo URL from appearance.logo_url_dark (control panel). */
		logoUrl?: string;
		logoDarkUrl?: string;
		logoWidth?: number;
		logoHeight?: number;
		logoDarkWidth?: number;
		logoDarkHeight?: number;
		/** Feature flags from /feature-flags — used to hide nav items for
		 *  disabled features so no trace of a toggled-off feature leaks. */
		features?: Record<string, boolean>;
		/** Render the brand row at the top of the nav. Mounts that already
		 *  carry the logo elsewhere can pass showBrand={false}. */
		showBrand?: boolean;
		/** Whether the expanded state is pinned (persisted). False on
		 *  transient expansions (hover peek/expand) — the toggle shows an
		 *  outline pin that pins the nav open on click. */
		pinned?: boolean;
		onToggle: () => void;
	}

	let { menu, user, collapsed, userSpaces = [], logoUrl = '', logoDarkUrl = '', logoWidth = 0, logoHeight = 0, logoDarkWidth = 0, logoDarkHeight = 0, features = {}, showBrand = true, pinned = true, onToggle }: Props = $props();

	// Collapsed rail shows the theme-aware favicon icon (appearance.favicon_url_*),
	// falling back to the bundled favicon.
	const collapsedFavicon = $derived(
		((theme.resolved === 'dark'
			? (page.data.siteSettings as Record<string, string> | undefined)?.['appearance.favicon_url_dark']
			: (page.data.siteSettings as Record<string, string> | undefined)?.['appearance.favicon_url_light']) ??
			(page.data.siteSettings as Record<string, string> | undefined)?.['appearance.favicon_url'] ??
			'/favicon.svg')
	);

	// The API already filters by auth/permissions, but guard against visibility
	// drift so an 'authenticated' item never leaks to a guest and vice-versa.
	// Also filter by feature flags: if a nav item's route maps to a feature
	// flag that is disabled, the item is hidden entirely.
	const items = $derived(
		(menu?.items ?? []).filter((item: MenuItem) => {
			if (item.visibility === 'authenticated') return !!user;
			if (item.visibility === 'guest') return !user;
			const flag = routeFeatureFlagForPath(item.url);
			// An array maps a route to a composite surface (e.g.
			// /messages/bookings) — visible when ANY listed flag is on.
			const flags = Array.isArray(flag) ? flag : flag ? [flag] : [];
			if (flags.length > 0 && !flags.some((f) => features[f] === true)) return false;
			return true;
		})
	);

	// §25: menu items flagged is_ad_slot carry an ad_slot_config payload
	// ({ slot, format }) instead of a navigable url — they render an AdSlot
	// where the nav link would sit rather than a broken anchor.
	function adSlotProps(item: MenuItem): { slot: string; format: 'banner' | 'square' | 'inline' | 'skyscraper' } {
		const cfg = item.ad_slot_config ?? {};
		const slot = typeof cfg.slot === 'string' && cfg.slot ? cfg.slot : 'left_rail';
		const fmt = cfg.format;
		const format = fmt === 'banner' || fmt === 'square' || fmt === 'inline' || fmt === 'skyscraper' ? fmt : 'skyscraper';
		return { slot, format };
	}

	function isActive(url: string): boolean {
		if (url === '/') return page.url.pathname === '/';
		const [path, query] = url.split('?');
		const pathMatch = page.url.pathname === path || page.url.pathname.startsWith(path + '/');
		if (!pathMatch || !query) return pathMatch;
		// Query-string menu urls (e.g. /services?kind=mentorship) only stay
		// active while the page carries the same params.
		return [...new URLSearchParams(query)].every(([k, v]) => page.url.searchParams.get(k) === v);
	}

	// Deterministic hue per item (same hash convention as DirectoryCard /
	// SourceCard avatars) — tints each icon's chip so the nav reads as a
	// coloured icon list rather than a row of bare glyphs. Seeded by icon
	// name so the same icon always maps to the same colour.
	function itemHue(item: MenuItem): number {
		const seed = item.icon || item.url;
		let h = 0;
		for (let i = 0; i < seed.length; i++) h = (h * 31 + seed.charCodeAt(i)) % 360;
		return h;
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

	// Section 09 Product Tour: map seeded nav-item URLs to per-feature tour
	// targets so the walkthrough can highlight individual features. Unknown or
	// admin-renamed items return undefined — the attribute is omitted and the
	// corresponding tour step is skipped gracefully.
	function navTourKey(url: string): string | undefined {
		const map: Record<string, string> = {
			'/': 'home',
			'/notifications': 'notifications',
			'/messages': 'messages',
			'/stories': 'stories',
			'/forum': 'forum',
			'/discover': 'discover',
			'/explore': 'explore',
			'/academy': 'academy',
			'/jobs': 'jobs',
			'/achievements': 'achievements',
			'/bookmarks': 'bookmarks'
		};
		return map[url];
	}

	// Collapsed-rail label flyout — a fixed-position pill rendered outside
	// the scrollable items list (whose overflow-y-auto would clip it) that
	// slides in from the left on hover/focus of an icon-only control.
	let flyout = $state<{ label: string; top: number } | null>(null);

	function showFlyout(e: Event, label: string) {
		if (!collapsed) return;
		const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
		flyout = { label, top: rect.top + rect.height / 2 };
	}

	function hideFlyout() {
		flyout = null;
	}
</script>

{#snippet navIcon(item: MenuItem, active: boolean, badge: number)}
	{@const Icon = resolveIcon(item.icon)}
	<span
		class={clsx('nav-icon-chip', active && 'nav-icon-chip--active')}
		style="--chip-hue: {itemHue(item)}"
	>
		<Icon size={16} strokeWidth={active ? 2.2 : 1.9} />
		{#if badge > 0}
			<span
				class="absolute -end-1 -top-1 grid h-4 min-w-4 place-items-center rounded-full bg-[var(--color-primary)] px-1 text-[10px] font-semibold text-[var(--color-primary-contrast)]"
				aria-label={t('a11y.unread_count', '{count} unread', { count: badge })}
			>{badge > 99 ? '99+' : badge}</span>
		{/if}
	</span>
{/snippet}

<nav
	class="flex h-full w-full flex-col border-e border-[var(--color-border)] bg-[var(--color-surface)]"
	aria-label={t('a11y.main_nav')}
>
	<!-- Logo + collapse toggle. With showBrand=false the row shrinks to
	     just the collapse control; in the collapsed rail it's skipped
	     entirely — the standalone expand button below covers it. -->
	{#if showBrand || !collapsed}
		<div
			class={clsx(
				'flex shrink-0 items-center gap-2 px-3',
				showBrand ? 'h-[var(--nav-height)]' : 'h-10',
				collapsed && 'justify-center',
				!showBrand && 'justify-end'
			)}
		>
			{#if showBrand}
				<a href="/" class="flex items-center gap-2 font-medium text-[var(--color-text)]">
					{#if collapsed}
						<img src={collapsedFavicon} alt={t('app.name')} class="h-8 w-8 shrink-0 object-contain" />
					{:else}
						<ThemeLogo {logoUrl} {logoDarkUrl} {logoWidth} {logoHeight} {logoDarkWidth} {logoDarkHeight} alt={t('app.name')} class="h-8 w-auto max-w-[140px] shrink-0 object-contain" />
					{/if}
				</a>
			{/if}
			{#if !collapsed}
				<!-- Pin state styling: filled primary = pinned open; outline
				     muted = transient (hover) expansion, click to pin. -->
				<button
					type="button"
					onclick={onToggle}
					class={clsx(
						'grid h-8 w-8 place-items-center rounded-[var(--radius)] transition-colors hover:bg-[var(--color-surface-subtle)]',
						pinned ? 'text-[var(--color-primary)]' : 'text-[var(--color-muted)]',
						showBrand && 'ms-auto'
					)}
					aria-label={pinned ? t('nav.collapse') : t('nav.pin', 'Pin navigation open')}
					aria-pressed={pinned}
					title={pinned ? t('nav.collapse') : t('nav.pin', 'Pin navigation open')}
				>
					<Pin size={18} fill={pinned ? 'currentColor' : 'none'} />
				</button>
			{/if}
		</div>
	{/if}

	{#if collapsed}
		<button
			type="button"
			onclick={onToggle}
			onmouseenter={(e) => showFlyout(e, t('nav.expand'))}
			onmouseleave={hideFlyout}
			onfocus={(e) => showFlyout(e, t('nav.expand'))}
			onblur={hideFlyout}
			class="mx-auto mb-1 grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
			aria-label={t('nav.expand')}
		>
			<PanelLeftOpen class="rtl:-scale-x-100" size={18} />
		</button>
	{/if}

	<!-- Nav items (from GET /menus/main_sidebar) -->
	<ul class="flex-1 overflow-y-auto px-2 py-2" data-tour="primary-nav" onscroll={hideFlyout}>
		{#each items as item (item.id)}
			{#if item.is_ad_slot}
				<!-- §25: ad-slot nav entries render the campaign slot; hidden in
				     collapsed mode (LeftRail owns the rail ad there) and empty
				     when no campaign fills — no placeholder space is reserved. -->
				{#if !collapsed}
					{@const adProps = adSlotProps(item)}
					<li class="my-2 px-1">
						<AdSlot slot={adProps.slot} format={adProps.format} />
					</li>
				{/if}
			{:else}
			{@const active = isActive(item.url)}
			{@const badge = item.url === '/notifications' ? realtime.unreadNotifications : 0}
			{@const isNotifications = item.url === '/notifications' && !!user}
			<li
				data-tour={navTourKey(item.url)}
				onmouseenter={(e) => showFlyout(e, item.label)}
				onmouseleave={hideFlyout}
				onfocusin={(e) => showFlyout(e, item.label)}
				onfocusout={hideFlyout}
			>
				{#if isNotifications}
					<!-- Phase 13.01 — Notifications nav item opens the dropdown -->
					<NotificationCentre side="right" align="start" {collapsed} class={itemClass(active)}>
						{#snippet children()}
							<span
								class={clsx('flex items-center gap-3', collapsed && 'justify-center')}
								aria-current={active ? 'page' : undefined}
							>
								{@render navIcon(item, active, badge)}
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
						target={item.target === '_blank' ? '_blank' : undefined}
						rel={item.target === '_blank' ? 'noopener noreferrer' : undefined}
						aria-label={collapsed ? item.label : undefined}
						aria-current={active ? 'page' : undefined}
						class={itemClass(active)}
					>
						{@render navIcon(item, active, badge)}
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
			{/if}
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
					onmouseenter={(e) => showFlyout(e, t('nav.compose'))}
					onmouseleave={hideFlyout}
					onfocus={(e) => showFlyout(e, t('nav.compose'))}
					onblur={hideFlyout}
					class="mx-auto grid h-11 w-11 place-items-center rounded-full bg-[var(--color-primary)] text-[var(--color-primary-contrast)] shadow-sm transition-colors hover:bg-[var(--color-primary-hover)]"
					aria-label={t('nav.compose')}
				>
					<PenSquare size={20} />
				</button>
			{:else}
				<a
					href="/login?redirect=/create"
					onmouseenter={(e) => showFlyout(e, t('nav.compose'))}
					onmouseleave={hideFlyout}
					onfocus={(e) => showFlyout(e, t('nav.compose'))}
					onblur={hideFlyout}
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

	<!-- Space switcher (bottom) — ADR-058: stays in the left nav, not the
	     account menu. The account entry lives in the right-sidebar topbar
	     (user_dropdown widget). -->
	{#if user && userSpaces.length > 0}
		<div class="border-t border-[var(--color-border)] p-2" data-tour="space-switcher">
			<UiMenu side="right" align="start" contentClass="flex max-h-[min(28rem,var(--bits-floating-available-height,70vh))] min-w-52 flex-col overflow-hidden">
				{#snippet trigger(props)}
					<button
						type="button"
						{...props}
						onmouseenter={(e) => showFlyout(e, t('nav.space_switcher'))}
						onmouseleave={hideFlyout}
						onfocus={(e) => showFlyout(e, t('nav.space_switcher'))}
						onblur={hideFlyout}
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
					</button>
				{/snippet}
				<div class="min-h-0 flex-1 overflow-y-auto">
					{#each userSpaces as sp (sp.id)}
						<UiMenuItem href={`/spaces/${sp.slug}`} icon={Hash}>
							<span class="min-w-0 flex-1 truncate">{sp.name}</span>
						</UiMenuItem>
					{/each}
				</div>
				<UiMenuSeparator />
				<UiMenuItem href="/discover?tab=spaces" icon={ExternalLink} iconClass="rtl:-scale-x-100" class="font-medium text-[var(--color-primary)]">
					{t('nav.space_switcher_browse')}
				</UiMenuItem>
			</UiMenu>
		</div>
	{/if}

	<!-- Collapsed-rail label flyout (fixed — escapes the items list's
	     overflow-y-auto, which would clip an in-flow tooltip) -->
	{#if collapsed && flyout}
		<span
			class="nav-label-flyout"
			style="top: {flyout.top}px"
			aria-hidden="true"
		>{flyout.label}</span>
	{/if}
</nav>

<style>
	/* Tinted circular icon chip — hue comes from --chip-hue (deterministic
	 * per-item hash). Muted tinted disc + saturated glyph, matching the
	 * directory-avatar convention used by DirectoryCard/SourceCard. */
	.nav-icon-chip {
		position: relative;
		display: grid;
		height: 2rem;
		width: 2rem;
		flex-shrink: 0;
		place-items: center;
		border-radius: 9999px;
		background: hsl(var(--chip-hue) 60% 50% / 0.14);
		color: hsl(var(--chip-hue) 65% 42%);
		transition:
			background-color 0.15s ease,
			color 0.15s ease;
	}

	.nav-icon-chip--active {
		background: hsl(var(--chip-hue) 65% 50% / 0.22);
		color: hsl(var(--chip-hue) 75% 40%);
	}

	:global([data-theme='dark']) .nav-icon-chip {
		background: hsl(var(--chip-hue) 45% 45% / 0.22);
		color: hsl(var(--chip-hue) 85% 68%);
	}

	:global([data-theme='dark']) .nav-icon-chip--active {
		background: hsl(var(--chip-hue) 55% 55% / 0.32);
		color: hsl(var(--chip-hue) 90% 74%);
	}

	/* Collapsed-rail label flyout — a pill that slides in from the left on
	 * hover/focus of an icon-only item. Position: fixed so it escapes the
	 * items list's overflow-y-auto clipping; left tracks the rail width. */
	.nav-label-flyout {
		position: fixed;
		inset-inline-start: calc(var(--nav-width-collapsed) + 10px);
		z-index: 60;
		max-width: 16rem;
		overflow: hidden;
		padding: 0.375rem 0.75rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		box-shadow: 0 4px 12px rgb(0 0 0 / 0.12);
		color: var(--color-text);
		font-size: 0.8125rem;
		font-weight: 500;
		line-height: 1.25rem;
		text-overflow: ellipsis;
		white-space: nowrap;
		pointer-events: none;
		transform: translateY(-50%);
		animation: nav-flyout-in 0.16s ease-out 40ms backwards;
	}

	@keyframes nav-flyout-in {
		from {
			opacity: 0;
			transform: translate(-10px, -50%);
		}
		to {
			opacity: 1;
			transform: translate(0, -50%);
		}
	}
</style>
