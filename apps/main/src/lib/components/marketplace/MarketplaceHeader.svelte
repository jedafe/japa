<script lang="ts">
	// Marketplace chrome header — sticky top nav for the public /services
	// surface (ADR-058 shell exception). Mirrors the GuestHeader auth-state
	// pattern (bell + avatar dropdown for authed users, login/signup for
	// guests) with marketplace-scoped nav links and a directory search that
	// submits to /services?q=.
	// Ref: docs/superpowers/plans/2026-10-07-services-marketplace-frontend.md (Task 2)
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t } from '$lib/i18n';
	import { theme } from '$lib/stores/theme.svelte';
	import { realtime } from '$lib/realtime/realtime.svelte';
	import ThemeLogo from '$lib/components/branding/ThemeLogo.svelte';
	import NotificationCentre from '$lib/components/shell/NotificationCentre.svelte';
	import UserDropdown from '$lib/components/shell/UserDropdown.svelte';
	import {
		Sun,
		Moon,
		LogIn,
		UserPlus,
		Menu as MenuIcon,
		X,
		Search,
		Briefcase,
		Package,
		BadgePercent
	} from 'lucide-svelte';
	import type { SessionUser } from '$lib/types';

	interface Props {
		user: SessionUser | null;
		/** SSR unread-notification count — seeds the realtime store so the
		    NotificationCentre bell badge renders before the socket connects. */
		unreadCount?: number;
		logoUrl?: string;
		logoDarkUrl?: string;
	}

	let { user, unreadCount = 0, logoUrl = '', logoDarkUrl = '' }: Props = $props();

	let mobileMenuOpen = $state(false);
	let searchTerm = $state('');

	// Seed the realtime unread counter from SSR (same pattern as
	// (app)/+layout.svelte) so the NotificationCentre badge is correct, and
	// connect the socket so live notifications reach the bell.
	$effect(() => {
		realtime.setUnreadCount(unreadCount);
	});
	onMount(() => {
		if (user) realtime.connect();
		return () => realtime.disconnect();
	});
	$effect(() => {
		if (user) realtime.connect();
		else realtime.disconnect();
	});

	// §111 — Packages nav link is gated on the service_packages flag, same
	// as the kind tab on the directory page.
	const packagesEnabled = $derived(
		!!(page.data.features as Record<string, boolean> | undefined)?.['service_packages.enabled']
	);

	const pathname = $derived(page.url.pathname);
	const kindParam = $derived(page.url.searchParams.get('kind'));

	function navLinkClass(active: boolean): string {
		return active
			? 'inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)]'
			: 'inline-flex items-center gap-1.5 rounded-[var(--radius)] px-3 py-1.5 text-sm font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]';
	}

	const servicesActive = $derived(pathname.startsWith('/services') && kindParam !== 'package');
	const packagesActive = $derived(pathname.startsWith('/services') && kindParam === 'package');
	const dealsActive = $derived(pathname.startsWith('/deals'));

	// Post-login return target — the auth flow reads `?redirect=` (see
	// (auth)/login/+page.svelte), so preserve the full current URL.
	const loginHref = $derived(
		`/login?redirect=${encodeURIComponent(page.url.pathname + page.url.search)}`
	);

	function submitSearch(e: SubmitEvent) {
		e.preventDefault();
		const q = searchTerm.trim();
		goto(q ? `/services?q=${encodeURIComponent(q)}` : '/services');
	}
</script>

<header class="sticky top-0 z-40 border-b border-[var(--color-border)] bg-[var(--color-surface)]">
	<div class="mx-auto flex h-[var(--nav-height)] max-w-[1400px] items-center gap-4 px-4">
		<!-- Logo + brand -->
		<a href="/" class="flex shrink-0 items-center" aria-label={t('app.name')}>
			<ThemeLogo {logoUrl} {logoDarkUrl} alt={t('app.name')} class="h-7 w-auto max-w-[140px] object-contain" />
		</a>

		<!-- Primary nav links -->
		<nav class="hidden min-w-0 flex-1 items-center gap-1 overflow-x-auto md:flex" aria-label={t('marketplace.nav_aria', 'Marketplace navigation')}>
			<a href="/services" class={navLinkClass(servicesActive)} aria-current={servicesActive ? 'page' : undefined}>
				<Briefcase size={15} />
				{t('marketplace.nav_services', 'Services')}
			</a>
			{#if packagesEnabled}
				<a href="/services?kind=package" class={navLinkClass(packagesActive)} aria-current={packagesActive ? 'page' : undefined}>
					<Package size={15} />
					{t('marketplace.nav_packages', 'Packages')}
				</a>
			{/if}
			<a href="/deals" class={navLinkClass(dealsActive)} aria-current={dealsActive ? 'page' : undefined}>
				<BadgePercent size={15} />
				{t('marketplace.nav_deals', 'Deals')}
			</a>
		</nav>

		<!-- Directory search — compact, hidden below md (mobile uses the drawer) -->
		<form class="hidden w-56 shrink-0 lg:block" role="search" onsubmit={submitSearch}>
			<label for="marketplace-search" class="sr-only">{t('marketplace.search_placeholder', 'Search services…')}</label>
			<div class="relative">
				<Search size={15} class="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--color-muted)]" />
				<input
					id="marketplace-search"
					type="search"
					bind:value={searchTerm}
					placeholder={t('marketplace.search_placeholder', 'Search services…')}
					class="h-9 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] pl-8 pr-3 text-sm text-[var(--color-text)] outline-none transition placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
				/>
			</div>
		</form>

		<!-- Right cluster -->
		<div class="ml-auto flex shrink-0 items-center gap-1.5">
			<button
				type="button"
				class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
				onclick={() => theme.toggle()}
				aria-label={theme.current === 'dark' ? t('theme.toggle_light', 'Switch to light') : t('theme.toggle_dark', 'Switch to dark')}
			>
				{#if theme.current === 'dark'}<Sun size={18} />{:else}<Moon size={18} />{/if}
			</button>

			{#if user}
				<!-- Same bell + avatar menu the AppShell uses (LeftNav /
				     SidebarWidgetRenderer) so notification + account behavior is
				     identical inside and outside the marketplace chrome. -->
				<NotificationCentre side="bottom" align="end" />
				<UserDropdown {user} />
			{:else}
				<a
					href={loginHref}
					class="hidden items-center gap-1.5 rounded-[var(--radius)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] sm:inline-flex"
				>
					<LogIn size={16} />
					<span>{t('landing.login', 'Log in')}</span>
				</a>
				<a
					href="/signup"
					class="hidden items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-semibold text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] sm:inline-flex"
				>
					<UserPlus size={16} />
					<span>{t('landing.signup', 'Get started')}</span>
				</a>
			{/if}

			<button
				type="button"
				class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)] md:hidden"
				onclick={() => (mobileMenuOpen = !mobileMenuOpen)}
				aria-label={mobileMenuOpen ? t('a11y.close_menu', 'Close menu') : t('a11y.open_menu', 'Open menu')}
				aria-expanded={mobileMenuOpen}
			>
				{#if mobileMenuOpen}<X size={20} />{:else}<MenuIcon size={20} />{/if}
			</button>
		</div>
	</div>

	<!-- Mobile dropdown nav — same links as the desktop bar plus the
	     directory search (the header input hides below lg). -->
	{#if mobileMenuOpen}
		<div class="border-t border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3 md:hidden">
			<form class="mb-3" role="search" onsubmit={(e) => { submitSearch(e); mobileMenuOpen = false; }}>
				<label for="marketplace-search-mobile" class="sr-only">{t('marketplace.search_placeholder', 'Search services…')}</label>
				<div class="relative">
					<Search size={15} class="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--color-muted)]" />
					<input
						id="marketplace-search-mobile"
						type="search"
						bind:value={searchTerm}
						placeholder={t('marketplace.search_placeholder', 'Search services…')}
						class="h-9 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] pl-8 pr-3 text-sm text-[var(--color-text)] outline-none transition placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
					/>
				</div>
			</form>
			<nav class="flex flex-col gap-1" aria-label={t('marketplace.nav_aria', 'Marketplace navigation')}>
				<a
					href="/services"
					class={navLinkClass(servicesActive)}
					aria-current={servicesActive ? 'page' : undefined}
					onclick={() => (mobileMenuOpen = false)}
				>
					<Briefcase size={15} />
					{t('marketplace.nav_services', 'Services')}
				</a>
				{#if packagesEnabled}
					<a
						href="/services?kind=package"
						class={navLinkClass(packagesActive)}
						aria-current={packagesActive ? 'page' : undefined}
						onclick={() => (mobileMenuOpen = false)}
					>
						<Package size={15} />
						{t('marketplace.nav_packages', 'Packages')}
					</a>
				{/if}
				<a
					href="/deals"
					class={navLinkClass(dealsActive)}
					aria-current={dealsActive ? 'page' : undefined}
					onclick={() => (mobileMenuOpen = false)}
				>
					<BadgePercent size={15} />
					{t('marketplace.nav_deals', 'Deals')}
				</a>
			</nav>
			{#if !user}
				<div class="mt-3 flex flex-col gap-2 border-t border-[var(--color-border)] pt-3">
					<a
						href={loginHref}
						class="inline-flex items-center justify-center gap-1.5 rounded-[var(--radius)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
						onclick={() => (mobileMenuOpen = false)}
					>
						<LogIn size={16} />
						{t('landing.login', 'Log in')}
					</a>
					<a
						href="/signup"
						class="inline-flex items-center justify-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-2 text-sm font-semibold text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
						onclick={() => (mobileMenuOpen = false)}
					>
						<UserPlus size={16} />
						{t('landing.signup', 'Get started')}
					</a>
				</div>
			{/if}
		</div>
	{/if}
</header>
