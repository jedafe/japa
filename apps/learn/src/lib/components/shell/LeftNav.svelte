<script lang="ts">
	import clsx from 'clsx';
	import { page } from '$app/state';
	import { PanelLeftClose, PanelLeftOpen, LogIn, GraduationCap } from 'lucide-svelte';
	import { resolveIcon } from '$lib/components/ui/icons';
	import { t } from '$lib/i18n';
	import ThemeLogo from '$lib/components/branding/ThemeLogo.svelte';
	import { loginUrl, profileUrl, resolveMenuUrl } from '$lib/appOrigins';
	import type { Menu, MenuItem, SessionUser } from '$lib/types';

	interface Props {
		menu: Menu | null;
		user: SessionUser | null;
		collapsed: boolean;
		logoUrl?: string;
		logoDarkUrl?: string;
		onToggle: () => void;
	}

	let { menu, user, collapsed, logoUrl = '', logoDarkUrl = '', onToggle }: Props = $props();

	// The API already filters by auth/permissions, but guard against visibility
	// drift so an 'authenticated' item never leaks to a guest and vice-versa.
	// Menu URLs that have no route on Learn (/login, /docs, …) are rewritten to
	// absolute Main-app URLs so they don't 404 (resolveMenuUrl).

	const items = $derived(
		(menu?.items ?? []).filter((item: MenuItem) => {
			if (item.visibility === 'authenticated') return !!user;
			if (item.visibility === 'guest') return !user;
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

	// Section 09 Product Tour: map seeded nav-item URLs to per-feature tour
	// targets so the walkthrough can highlight individual features. Unknown or
	// admin-renamed items return undefined — the attribute is omitted and the
	// corresponding tour step is skipped gracefully.
	function navTourKey(url: string): string | undefined {
		const map: Record<string, string> = {
			'/': 'learn-browse',
			'/my-learning': 'my-learning',
			'/certificates': 'certificates',
			'/bookmarks': 'learn-bookmarks',
			'/dashboard/instructor': 'instructor'
		};
		return map[url];
	}

	const brandLabel = $derived(t('university.name'));
</script>

<nav
	class="flex h-full w-full flex-col border-r border-[var(--color-border)] bg-[var(--color-surface)]"
	aria-label={t('a11y.main_nav')}
>
	<!-- Logo + collapse toggle -->
	<div class={clsx('flex h-[var(--nav-height)] shrink-0 items-center gap-2 px-3', collapsed && 'justify-center')}>
		<a href="/" class="flex items-center gap-2 font-medium text-[var(--color-text)]">
			{#if logoUrl}
				<ThemeLogo {logoUrl} {logoDarkUrl} alt={brandLabel} class="h-8 w-auto max-w-[140px] shrink-0 object-contain" />
			{:else}
				<span
					class="grid h-8 w-8 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-primary)] text-[var(--color-primary-contrast)]"
				>
					<GraduationCap size={18} />
				</span>
			{/if}
			{#if !collapsed && !logoUrl}<span>{brandLabel}</span>{/if}
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

	<!-- Nav items (from GET /menus/learn_sidebar — ADR-054, migration 000213) -->
	<ul class="flex-1 overflow-y-auto px-2 py-2" data-tour="primary-nav">
		{#each items as item (item.id)}
			{@const Icon = resolveIcon(item.icon)}
			{@const active = isActive(item.url)}
			<li data-tour={navTourKey(item.url)}>
				<a
					href={resolveMenuUrl(item.url)}
					target={item.target === '_blank' ? '_blank' : undefined}
					rel={item.target === '_blank' ? 'noopener noreferrer' : undefined}
					title={collapsed ? item.label : undefined}
					aria-current={active ? 'page' : undefined}
					class={itemClass(active)}
				>
					<Icon size={20} strokeWidth={active ? 2.2 : 1.8} />
					{#if !collapsed}
						<span class="min-w-0 flex-1">{item.label}</span>
					{/if}
				</a>
			</li>
		{/each}
	</ul>

	<!-- Account (bottom) — no space switcher or compose button on Learn -->
	<div class="border-t border-[var(--color-border)] p-2" data-tour="learn-account">
		{#if user}
			<a
				href={profileUrl(user.username)}
				title={collapsed ? user.displayName : undefined}
				class={clsx('flex items-center gap-3 rounded-[var(--radius)] p-2 hover:bg-[var(--color-surface-subtle)]', collapsed && 'justify-center')}
			>
				{#if user.avatar_url}
					<img src={user.avatar_url} alt="" class="h-8 w-8 shrink-0 rounded-full object-cover" />
				{:else}
					<span
						class="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-xs font-medium text-[var(--color-text-subtle)]"
						>{user.displayName?.charAt(0)?.toUpperCase() ?? '?'}</span
					>
				{/if}
				{#if !collapsed}
					<span class="min-w-0 flex-1">
						<span class="block truncate text-sm font-medium text-[var(--color-text)]">{user.displayName}</span>
						<span class="block truncate text-xs text-[var(--color-muted)]">@{user.username}</span>
					</span>
				{/if}
			</a>
		{:else}
			<a
				href={loginUrl()}
				class="flex items-center justify-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
			>
				<LogIn size={18} />
				{#if !collapsed}<span>{t('nav.account')}</span>{/if}
			</a>
		{/if}
	</div>
</nav>
