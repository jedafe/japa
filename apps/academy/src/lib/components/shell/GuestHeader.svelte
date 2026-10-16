<script lang="ts">
	// Full-width top navigation bar for unauthenticated users on the Academy app.
	// Replaces the LeftNav + RightSidebar shell for guests.
	// Ref: ADR-060 — Academy shell for guests should show a top bar with search,
	// theme toggle, language toggle, and auth buttons.
	import { t, i18n } from '$lib/i18n';
	import { theme } from '$lib/stores/theme.svelte';
	import { loginUrl, MAIN_ORIGIN } from '$lib/appOrigins';
	import { page } from '$app/state';
	import { Menu as UiMenu, MenuItem as UiMenuItem } from '@japaconnect/ui';
	import { Sun, Moon, LogIn, UserPlus, Languages, GraduationCap, Check } from 'lucide-svelte';
	import SearchAutocomplete from './SearchAutocomplete.svelte';
	import type { Menu, MenuItem } from '$lib/types';

	interface Props {
		logoUrl?: string;
		menu: Menu | null;
	}

	let { logoUrl = '', menu }: Props = $props();

	const guestItems = $derived(
		(menu?.items ?? []).filter((item: MenuItem) => {
			if (item.visibility === 'authenticated') return false;
			return true;
		})
	);

	const brandLabel = $derived(t('academy.name'));

	// Enabled languages + default from site settings (§18, ADR-013).
	const enabledLanguages = $derived(
		((page.data?.siteSettings?.['i18n.enabled_languages'] as string | undefined) ?? 'en,fr')
			.split(',').map((l) => l.trim()).filter(Boolean)
	);
	const defaultLanguage = $derived(
		(page.data?.siteSettings?.['i18n.default_language'] as string | undefined) ?? 'en'
	);
	const showLanguageToggle = $derived(enabledLanguages.length > 1);
	const currentPath = $derived(page.url.pathname.replace(/^\//, ''));

	// Localize in the ACTIVE app locale — `undefined` would resolve the
	// runtime's default locale, so SSR (server locale) and hydration
	// (browser locale) can produce different labels → hydration mismatch.
	let displayNames: Intl.DisplayNames | null = null;
	let displayNamesLocale = '';
	function localeLabel(code: string): string {
		// Recreate when the active locale changes so labels follow a live
		// locale switch instead of the locale captured at component init.
		if (displayNamesLocale !== i18n.locale) {
			displayNamesLocale = i18n.locale;
			try { displayNames = new Intl.DisplayNames([i18n.locale], { type: 'language' }); }
			catch { displayNames = null; }
		}
		return displayNames?.of(code) ?? code;
	}
	function localeHref(lang: string): string {
		const path = currentPath || '';
		return lang === defaultLanguage ? `/${path}` : `/${lang}/${path}`;
	}
</script>

<header class="guest-header">
	<div class="guest-header-inner">
		<a href="/" class="guest-brand" aria-label={brandLabel}>
			{#if logoUrl}
				<img src={logoUrl} alt={brandLabel} class="guest-logo" />
			{:else}
				<span class="guest-brand-mark">
					<GraduationCap size={18} />
				</span>
			{/if}
			<span class="guest-brand-name">{brandLabel}</span>
		</a>

		<nav class="guest-nav" aria-label={t('a11y.main_nav')}>
			{#each guestItems as item (item.id)}
				<a href={item.url} class="guest-nav-link">{item.label}</a>
			{/each}
		</nav>

		<div class="guest-actions">
			<div class="guest-search">
				<SearchAutocomplete />
			</div>
			{#if showLanguageToggle}
				<UiMenu contentClass="max-h-60 overflow-y-auto">
					{#snippet trigger(props)}
						<button
							type="button"
							{...props}
							class="guest-icon-btn"
							aria-label={t('locale.switch_language', 'Switch language')}
							title={localeLabel(i18n.locale)}
						>
							<Languages size={18} />
						</button>
					{/snippet}
					{#each enabledLanguages as lang (lang)}
						<UiMenuItem
							onSelect={() => { window.location.href = localeHref(lang); }}
							class="justify-between"
						>
							<span>{localeLabel(lang)}</span>
							{#if lang === i18n.locale}<Check size={14} class="text-[var(--color-primary)]" />{/if}
						</UiMenuItem>
					{/each}
				</UiMenu>
			{/if}
			<button
				type="button"
				class="guest-icon-btn"
				onclick={() => theme.toggle()}
				aria-label={theme.current === 'dark' ? t('theme.toggle_light') : t('theme.toggle_dark')}
			>
				{#if theme.current === 'dark'}<Sun size={18} />{:else}<Moon size={18} />{/if}
			</button>
			<a href={loginUrl()} class="guest-login-btn">
				<LogIn size={16} />
				<span>{t('nav.login')}</span>
			</a>
			<a href={`${MAIN_ORIGIN}/signup`} class="guest-signup-btn">
				<UserPlus size={16} />
				<span>{t('nav.signup')}</span>
			</a>
		</div>
	</div>
</header>

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
</style>
