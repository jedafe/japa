<script lang="ts">
	// Full-width top navigation bar for unauthenticated users.
	// Replaces the LeftNav + RightSidebar shell for guests on public pages.
	// Ref: ADR-058 §5 Exception #4 — showing the full app shell (Compose,
	// Notifications, left nav) to a logged-out user is incoherent.
	import { t } from '$lib/i18n';
	import { theme } from '$lib/stores/theme.svelte';
	import { Search, Sun, Moon, LogIn, UserPlus, Menu as MenuIcon, X } from 'lucide-svelte';
	import SearchAutocomplete from './SearchAutocomplete.svelte';
	import type { Menu, MenuItem } from '$lib/types';

	let mobileMenuOpen = $state(false);

	interface Props {
		/** Platform logo URL from appearance.logo_url (control panel). */
		logoUrl?: string;
		/** Navigation menu from GET /menus/main_sidebar — filtered to guest-visible items. */
		menu: Menu | null;
	}

	let { logoUrl = '', menu }: Props = $props();

	// Filter to guest-visible items only (visibility === 'guest' or 'all').
	const guestItems = $derived(
		(menu?.items ?? []).filter((item: MenuItem) => {
			if (item.visibility === 'authenticated') return false;
			return true;
		})
	);
</script>

<header class="guest-header">
	<div class="guest-header-inner">
		<!-- Logo + brand -->
		<a href="/" class="guest-brand" aria-label={t('app.name')}>
			{#if logoUrl}
				<img src={logoUrl} alt={t('app.name')} class="guest-logo" />
			{:else}
				<span class="guest-brand-mark">JC</span>
			{/if}
			<span class="guest-brand-name hidden">{t('app.name')}</span>
		</a>

		<!-- Primary nav links -->
		<nav class="guest-nav" aria-label={t('nav.main')}>
			{#each guestItems as item (item.id)}
				<a href={item.url} class="guest-nav-link">{item.label}</a>
			{/each}
		</nav>

		<!-- Right cluster: search, theme toggle, auth buttons, mobile hamburger -->
		<div class="guest-actions">
			<div class="guest-search">
				<SearchAutocomplete />
			</div>
			<button
				type="button"
				class="guest-icon-btn"
				onclick={() => theme.toggle()}
				aria-label={theme.current === 'dark' ? t('theme.toggle_light', 'Switch to light') : t('theme.toggle_dark', 'Switch to dark')}
			>
				{#if theme.current === 'dark'}<Sun size={18} />{:else}<Moon size={18} />{/if}
			</button>
			<a href="/login" class="guest-login-btn guest-auth-link">
				<LogIn size={16} />
				<span>{t('landing.login')}</span>
			</a>
			<a href="/signup" class="guest-signup-btn guest-auth-link">
				<UserPlus size={16} />
				<span>{t('landing.signup')}</span>
			</a>
			<button
				type="button"
				class="guest-icon-btn md:hidden"
				onclick={() => (mobileMenuOpen = true)}
				aria-label={t('a11y.open_menu', 'Open menu')}
			>
				<MenuIcon size={20} />
			</button>
		</div>
	</div>
</header>

{#if mobileMenuOpen}
	<button
		type="button"
		class="fixed inset-0 z-40 bg-black/50"
		aria-label={t('a11y.close_menu', 'Close menu')}
		onclick={() => (mobileMenuOpen = false)}
	></button>
	<div class="fixed left-0 top-0 z-50 h-screen w-[min(80vw,18rem)] border-r border-[var(--color-border)] bg-[var(--color-surface)] p-4 shadow-xl">
		<div class="flex items-center justify-between gap-3 border-b border-[var(--color-border)] pb-4">
			<span class="font-semibold text-[var(--color-text)]">{t('nav.menu', 'Menu')}</span>
			<button
				type="button"
				class="guest-icon-btn"
				onclick={() => (mobileMenuOpen = false)}
				aria-label={t('a11y.close_menu', 'Close menu')}
			>
				<X size={18} />
			</button>
		</div>
		<nav class="mt-4 flex flex-col gap-2">
			{#each guestItems as item (item.id)}
				<a
					href={item.url}
					class="rounded-[var(--radius)] px-3 py-2 text-sm font-medium text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
					onclick={() => (mobileMenuOpen = false)}
				>
					{item.label}
				</a>
			{/each}
		</nav>
		<div class="mt-6 flex flex-col gap-2 border-t border-[var(--color-border)] pt-4">
			<a href="/login" class="guest-login-btn guest-drawer-btn w-full justify-center">
				<LogIn size={16} />
				<span>{t('landing.login')}</span>
			</a>
			<a href="/signup" class="guest-signup-btn guest-drawer-btn w-full justify-center">
				<UserPlus size={16} />
				<span>{t('landing.signup')}</span>
			</a>
		</div>
	</div>
{/if}

<style>
	.guest-header {
		position: sticky;
		top: 0;
		z-index: 50;
		border-bottom: 1px solid var(--color-border);
		background: var(--color-surface);
	}
	.guest-header-inner {
		display: flex;
		align-items: center;
		gap: 1rem;
		height: var(--nav-height);
		padding: 0 1rem;
		max-width: 1400px;
		margin: 0 auto;
	}

	/* ---- Brand ---- */
	.guest-brand {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-weight: 600;
		color: var(--color-text);
		white-space: nowrap;
	}
	.guest-logo {
		height: 1.75rem;
		width: auto;
		max-width: 140px;
		object-fit: contain;
	}
	:global([data-theme='dark']) .guest-logo {
		filter: invert(1) hue-rotate(180deg);
	}
	.guest-brand-mark {
		display: inline-grid;
		place-items: center;
		height: 2rem;
		width: 2rem;
		border-radius: var(--radius);
		background: var(--color-primary);
		color: var(--color-primary-contrast);
		font-size: 0.8125rem;
		font-weight: 700;
	}
	.guest-brand-name {
		font-size: 1rem;
	}

	/* ---- Nav ---- */
	.guest-nav {
		display: flex;
		align-items: center;
		gap: 0.25rem;
		flex: 1;
		min-width: 0;
		overflow-x: auto;
		scrollbar-width: none;
	}
	.guest-nav::-webkit-scrollbar {
		display: none;
	}
	.guest-nav-link {
		padding: 0.375rem 0.75rem;
		border-radius: var(--radius);
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--color-text-subtle);
		white-space: nowrap;
		transition: color 0.15s, background 0.15s;
	}
	.guest-nav-link:hover {
		color: var(--color-text);
		background: var(--color-surface-subtle);
	}

	/* ---- Actions ---- */
	.guest-actions {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-shrink: 0;
	}
	.guest-search {
		width: 200px;
	}
	.guest-icon-btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		height: 2rem;
		width: 2rem;
		border: none;
		border-radius: var(--radius);
		background: transparent;
		color: var(--color-text-subtle);
		cursor: pointer;
		transition: color 0.15s, background 0.15s;
	}
	.guest-icon-btn:hover {
		color: var(--color-text);
		background: var(--color-surface-subtle);
	}
	.guest-login-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		padding: 0.375rem 0.875rem;
		border-radius: var(--radius);
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--color-text);
		transition: background 0.15s;
	}
	.guest-login-btn:hover {
		background: var(--color-surface-subtle);
	}
	.guest-signup-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		padding: 0.375rem 0.875rem;
		border-radius: var(--radius);
		background: var(--color-primary);
		color: var(--color-primary-contrast);
		font-size: 0.875rem;
		font-weight: 600;
		transition: background 0.15s;
	}
	.guest-signup-btn:hover {
		background: var(--color-primary-hover);
	}

	.guest-drawer-btn span {
		display: inline !important;
	}

	/* ---- Responsive ---- */
	@media (max-width: 1023px) {
		.guest-search {
			display: none;
		}
	}
	@media (max-width: 767px) {
		.guest-brand-name {
			display: none;
		}
		.guest-nav {
			display: none;
		}
		.guest-login-btn span,
		.guest-signup-btn span {
			display: none;
		}
	}
	@media (max-width: 639px) {
		.guest-actions .guest-auth-link {
			display: none;
		}
	}
</style>
