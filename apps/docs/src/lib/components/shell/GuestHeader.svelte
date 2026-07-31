<script lang="ts">
	// Full-width top navigation bar for unauthenticated users on the docs app.
	// Replaces the DocNavTree + right sidebar shell for guests.
	// Ref: ADR-060 — docs shell for guests should show a top bar with search,
	// audience switcher, theme toggle, and auth buttons.
	import { t } from '$lib/i18n';
	import { theme } from '$lib/stores/theme.svelte';
	import { Sun, Moon, LogIn, UserPlus } from 'lucide-svelte';
	import SearchLauncher from './SearchLauncher.svelte';
	import AudienceSwitcher from './AudienceSwitcher.svelte';
	import type { DocAudience } from '$lib/types';

	interface Props {
		logoUrl?: string;
		accessibleAudiences: DocAudience[];
		activeAudience: DocAudience | null;
	}

	let { logoUrl = '', accessibleAudiences, activeAudience }: Props = $props();
</script>

<header class="guest-header">
	<div class="guest-header-inner">
		<a href="/docs" class="guest-brand" aria-label={t('app.name')}>
			{#if logoUrl}
				<img src={logoUrl} alt={t('app.name')} class="guest-logo" />
			{:else}
				<span class="guest-brand-mark">JC</span>
			{/if}
			<span class="guest-brand-name">{t('app.name')}</span>
		</a>

		<nav class="guest-nav" aria-label={t('a11y.main_nav', 'Main navigation')}>
			<a href="/docs" class="guest-nav-link">{t('nav.docs_home')}</a>
			{#if accessibleAudiences.length > 0}
				<div class="guest-audience">
					<AudienceSwitcher {accessibleAudiences} {activeAudience} />
				</div>
			{/if}
		</nav>

		<div class="guest-actions">
			<SearchLauncher />
			<button
				type="button"
				class="guest-icon-btn"
				onclick={() => theme.toggle()}
				aria-label={theme.current === 'dark' ? t('theme.toggle_light') : t('theme.toggle_dark')}
			>
				{#if theme.current === 'dark'}<Sun size={18} />{:else}<Moon size={18} />{/if}
			</button>
			<a href="/login" class="guest-login-btn">
				<LogIn size={16} />
				<span>{t('auth.login')}</span>
			</a>
			<a href="/signup" class="guest-signup-btn">
				<UserPlus size={16} />
				<span>{t('auth.signup')}</span>
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
	.guest-audience {
		display: flex;
		align-items: center;
	}
	.guest-actions {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-shrink: 0;
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
