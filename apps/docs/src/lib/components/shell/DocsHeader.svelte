<script lang="ts">
	// Global docs-portal header (ADR-060 drift): brand · search · version ·
	// language · theme · account. Rendered once above
	// the three-column shell on every docs surface.
	import { page } from '$app/state';
	import { Menu as MenuIcon, Search, ChevronDown, Sun, Moon, Languages, Check } from 'lucide-svelte';
	import { Menu, MenuItem } from '@japaconnect/ui';
	import ThemeLogo from '$lib/components/branding/ThemeLogo.svelte';
	import SearchPalette from '$lib/components/docs/SearchPalette.svelte';
	import UserDropdown from './UserDropdown.svelte';
	import GuestAuthButtons from './GuestAuthButtons.svelte';
	import SidebarWidgetRenderer from './SidebarWidgetRenderer.svelte';
	import { t, i18n } from '$lib/i18n';
	import { theme } from '$lib/stores/theme.svelte';
	import { enabledLanguages, localeLabel, switchLanguage } from '$lib/locale';
	import { parseVersions } from '$lib/docs/catalog';

	import type { SessionUser } from '$lib/types';

	interface HeaderWidget {
		id: string;
		widget_type: string;
		label: string;
		props: Record<string, unknown>;
		sort_order: number;
		slot?: string;
	}

	interface Props {
		user: SessionUser | null;
		logoUrl?: string;
		logoDarkUrl?: string;
		/** Admin-managed topbar widgets — types the header already provides
		 *  built-ins for (search/theme/language/user) are deduplicated. */
		topbarWidgets?: HeaderWidget[];
		/** Show the mobile drawer button (hidden for the full-width variant). */
		showMenuButton?: boolean;
		onOpenMenu?: () => void;
	}

	let {
		user,
		logoUrl = '',
		logoDarkUrl = '',
		topbarWidgets = [],
		showMenuButton = true,
		onOpenMenu
	}: Props = $props();


	// Widget types the header renders natively — skip admin widgets of these
	// types so the canonical chrome never duplicates.
	const builtinTypes = new Set(['search', 'search_launcher', 'theme_toggle', 'language_toggle', 'user_dropdown']);
	const extraWidgets = $derived(topbarWidgets.filter((w) => !builtinTypes.has(w.widget_type)));

	const versions = $derived(parseVersions(page.data?.siteSettings?.['docs.versions']));
	const langList = $derived(enabledLanguages());

	let searchOpen = $state(false);

	function onKeydown(e: KeyboardEvent) {
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
			e.preventDefault();
			searchOpen = true;
		}
	}

	$effect(() => {
		window.addEventListener('keydown', onKeydown);
		return () => window.removeEventListener('keydown', onKeydown);
	});
</script>

<header class="header">
	<div class="header-left">
		{#if showMenuButton}
			<button
				type="button"
				class="icon-btn menu-btn"
				onclick={onOpenMenu}
				aria-label={t('a11y.open_menu')}
				data-tour="docs-nav"
			>
				<MenuIcon size={20} />
			</button>
		{/if}
		<a href="/docs" class="brand">
			{#if logoUrl}
				<ThemeLogo {logoUrl} {logoDarkUrl} alt={t('app.name')} class="h-7 w-auto max-w-[140px] object-contain" />
			{:else}
				<span class="brand-mark">JC</span>
				<span class="brand-name">{t('app.name')}</span>
			{/if}
		</a>
	</div>

	<div class="header-center">
		<button type="button" class="search-field" onclick={() => (searchOpen = true)} data-tour="docs-search">
			<Search size={16} />
			<span class="search-label">{t('search.placeholder')}</span>
			<kbd class="search-kbd">⌘K</kbd>
		</button>
	</div>

	<div class="header-right" data-tour="docs-topbar">
		<button
			type="button"
			class="icon-btn search-icon-btn"
			onclick={() => (searchOpen = true)}
			aria-label={t('a11y.open_search')}
		>
			<Search size={18} />
		</button>

		{#if versions.length > 0}
			<Menu contentClass="max-h-60 overflow-y-auto">
				{#snippet trigger(props)}
					<button type="button" {...props} class="select-btn" aria-label={t('header.version')}>
						<span class="select-label">{versions[0].label}</span>
						<ChevronDown size={14} />
					</button>
				{/snippet}
				{#each versions as v (v.label)}
					{#if v.url}
						<MenuItem href={v.url}>{v.label}</MenuItem>
					{:else}
						<MenuItem>{v.label}</MenuItem>
					{/if}
				{/each}
			</Menu>
		{/if}

		{#if langList.length > 1}
			<Menu contentClass="max-h-60 overflow-y-auto">
				{#snippet trigger(props)}
					<button
						type="button"
						{...props}
						class="select-btn"
						aria-label={t('locale.switch_language', 'Switch language')}
					>
						<Languages size={16} />
						<span class="select-label">{localeLabel(i18n.locale)}</span>
						<ChevronDown size={14} />
					</button>
				{/snippet}
				{#each langList as lang (lang)}
					<MenuItem onSelect={() => switchLanguage(lang)}>
						<span class="flex-1">{localeLabel(lang)}</span>
						{#if lang === i18n.locale}<Check size={14} class="text-[var(--color-primary)]" />{/if}
					</MenuItem>
				{/each}
			</Menu>
		{/if}

		<button
			type="button"
			class="icon-btn"
			onclick={() => theme.toggle()}
			aria-label={theme.resolved === 'dark' ? t('theme.toggle_light') : t('theme.toggle_dark')}
		>
			{#if theme.resolved === 'dark'}<Sun size={18} />{:else}<Moon size={18} />{/if}
		</button>

		{#each extraWidgets as widget (widget.id)}
			<SidebarWidgetRenderer {widget} {user} />
		{/each}

		{#if user}
			<UserDropdown {user} />
		{:else}
			<GuestAuthButtons compact />
		{/if}
	</div>
</header>

{#if searchOpen}
	<SearchPalette onClose={() => (searchOpen = false)} />
{/if}

<style>
	.header {
		grid-column: 1 / -1;
		position: sticky;
		top: 0;
		z-index: 30;
		display: flex;
		align-items: center;
		gap: 1rem;
		height: var(--nav-height);
		padding: 0 0.875rem;
		border-bottom: 1px solid var(--color-border);
		background: var(--color-surface);
	}
	.header-left {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		min-width: 0;
		flex-shrink: 0;
	}
	.brand {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-weight: 600;
		color: var(--color-text);
		text-decoration: none;
		white-space: nowrap;
	}
	.brand-mark {
		display: inline-grid;
		place-items: center;
		height: 1.5rem;
		width: 1.5rem;
		border-radius: var(--radius);
		background: var(--color-primary);
		color: var(--color-primary-contrast);
		font-size: 0.6875rem;
		font-weight: 700;
	}
	.brand-name {
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.header-center {
		flex: 1;
		display: flex;
		justify-content: center;
		min-width: 0;
	}
	.search-field {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		width: 100%;
		max-width: 26rem;
		height: 2.25rem;
		padding: 0 0.75rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface-subtle);
		color: var(--color-muted);
		font-size: 0.8125rem;
		cursor: pointer;
		text-align: left;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.search-field:hover {
		border-color: var(--color-primary);
	}
	.search-label {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.search-kbd {
		font-family: var(--font-mono);
		font-size: 0.6875rem;
		padding: 0.0625rem 0.375rem;
		border: 1px solid var(--color-border);
		border-radius: 4px;
		background: var(--color-surface);
		color: var(--color-muted);
		flex-shrink: 0;
	}
	.header-right {
		display: flex;
		align-items: center;
		gap: 0.375rem;
		margin-left: auto;
		flex-shrink: 0;
	}
	.icon-btn {
		display: grid;
		place-items: center;
		height: 2.25rem;
		width: 2.25rem;
		border: none;
		border-radius: var(--radius);
		background: transparent;
		color: var(--color-muted);
		cursor: pointer;
		flex-shrink: 0;
	}
	.icon-btn:hover {
		background: var(--color-surface-subtle);
		color: var(--color-text);
	}
	.menu-btn { display: none; }
	.search-icon-btn { display: none; }
	.select-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		height: 2.25rem;
		padding: 0 0.625rem;
		border: 1px solid transparent;
		border-radius: var(--radius);
		background: transparent;
		color: var(--color-text-subtle);
		font-size: 0.8125rem;
		cursor: pointer;
		white-space: nowrap;
	}
	.select-btn:hover {
		background: var(--color-surface-subtle);
		color: var(--color-text);
	}
	.select-label {
		max-width: 7rem;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	@media (max-width: 767px) {
		.header { gap: 0.5rem; }
		.header-center { display: none; }
		.search-icon-btn { display: grid; }
		.menu-btn { display: grid; }
		.select-label { display: none; }
		/* Wordmark + full control cluster exceeds the row anywhere below
		   ~502px — keep the mark, drop the text for the whole mobile band. */
		.brand-name { display: none; }
	}
</style>
