<script lang="ts">
	// Learn mobile bottom bar — learning-oriented tabs (no compose button).
	// Ref: ADR-058 (mobile single-column + bottom bar), ADR-060 (Learn nav).
	// Nav items come from the API-driven bottom bar menu
	// (GET /menus/learn_bottom_bar) so the list is never hardcoded (Rule 16).
	import { page } from '$app/state';
	import { resolveIcon } from '$lib/components/ui/icons';
	import { t } from '$lib/i18n';
	import { loginUrl, profileUrl } from '$lib/appOrigins';
	import type { Menu, MenuItem, SessionUser } from '$lib/types';

	interface Props {
		user: SessionUser | null;
		menu: Menu | null;
	}
	let { user, menu }: Props = $props();

	function isActive(url: string): boolean {
		if (url === '/') return page.url.pathname === '/';
		return page.url.pathname === url || page.url.pathname.startsWith(url + '/');
	}

	// Cross-app routes (e.g. /login, /profile) are rewritten to absolute Main-app
	// URLs or the user's profile URL so they don't 404 on the Learn app.
	function resolveItemUrl(url: string): string {
		if (url === '/login') return loginUrl();
		if (url === '/profile') return user ? profileUrl(user.username) : loginUrl();
		return url;
	}

	const menuItems = $derived(
		(menu?.items ?? []).filter((item: MenuItem) => {
			if (item.visibility === 'authenticated') return !!user;
			if (item.visibility === 'guest') return !user;
			return true;
		})
	);
</script>

<nav
	class="fixed inset-x-0 bottom-0 z-40 flex items-stretch border-t border-[var(--color-border)] bg-[var(--color-surface)]/95 backdrop-blur sm:hidden"
	aria-label={t('a11y.main_nav')}
>
	{#each menuItems as item (item.id)}
		{@const Icon = resolveIcon(item.icon)}
		{@const active = isActive(item.url)}
		<a
			href={resolveItemUrl(item.url)}
			target={item.target}
			class="relative flex flex-1 flex-col items-center justify-center gap-0.5 py-2 text-[10px] {active
				? 'text-[var(--color-primary)]'
				: 'text-[var(--color-muted)]'}"
			aria-current={active ? 'page' : undefined}
		>
			<Icon size={22} />
			<span class="leading-none">{item.label}</span>
		</a>
	{/each}
</nav>
