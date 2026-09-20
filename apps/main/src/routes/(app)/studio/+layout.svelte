<script lang="ts">
	// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md — shared studio
	// chrome. The whole /studio surface renders as an ADR-058 exception in
	// composer chrome (same visual language as CreateShell's composer-grid):
	// centered 1200px surface, composer-header bar, segmented tab control.
	import { page } from '$app/state';
	import type { LayoutData } from './$types';
	import type { Snippet } from 'svelte';
	import { t } from '$lib/i18n';
	import { Files, Clapperboard, TerminalSquare, SquarePen } from 'lucide-svelte';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	const videoEnabled = $derived(data.videoEnabled);
	const isCreateTab = $derived(page.route.id === '/(app)/studio/create');

	const tabs = $derived.by(() => [
		{ href: '/studio/contents', label: t('studio.tabs.contents', 'Contents'), icon: Files, disabled: false },
		{
			href: '/studio/videos',
			label: t('studio.tabs.videos', 'Videos'),
			icon: Clapperboard,
			disabled: !videoEnabled
		},
		{ href: '/studio/prompts', label: t('studio.tabs.prompts', 'Prompts'), icon: TerminalSquare, disabled: false },
		{ href: '/studio/create', label: t('studio.tabs.create', 'Create'), icon: SquarePen, disabled: false }
	]);

	function isActive(href: string): boolean {
		return page.url.pathname === href || page.url.pathname.startsWith(href + '/');
	}
</script>

<div class="studio-surface" class:is-composer={isCreateTab}>
	<header class="studio-header">
		<h1 class="studio-title">{t('studio.heading', 'Studio')}</h1>
		<nav class="studio-tabs" aria-label={t('studio.tabs.label', 'Studio sections')}>
			{#each tabs as tab (tab.href)}
				{#if tab.disabled}
					<span
						class="studio-tab disabled"
						title={tab.href === '/studio/videos' && !videoEnabled
							? t('studio.tabs.videos_disabled', 'Video is not enabled on this site')
							: t('studio.tabs.coming_soon', 'Coming soon')}
					>
						<tab.icon size={14} />
						{tab.label}
					</span>
				{:else}
					<a
						href={tab.href}
						class="studio-tab"
						class:active={isActive(tab.href)}
						aria-current={isActive(tab.href) ? 'page' : undefined}
					>
						<tab.icon size={14} />
						{tab.label}
					</a>
				{/if}
			{/each}
		</nav>
	</header>

	<div class="studio-body">
		{@render children()}
	</div>
</div>

<style>
	.studio-surface {
		max-width: 1200px;
		margin: 0 auto;
		padding: 1.5rem;
	}

	.studio-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		flex-wrap: wrap;
	}

	.studio-title {
		margin: 0;
		font-size: 1.125rem;
		font-weight: 600;
		color: var(--color-text);
	}

	/* Segmented control in the composer-type-select idiom. */
	.studio-tabs {
		display: flex;
		height: 2.375rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		overflow: hidden;
	}

	.studio-tab {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		padding: 0 12px;
		font-size: 0.875rem;
		color: var(--color-muted);
		text-decoration: none;
		transition: background 0.15s, color 0.15s;
	}

	.studio-tab + .studio-tab {
		border-left: 1px solid var(--color-border);
	}

	.studio-tab:hover:not(.disabled):not(.active) {
		background: var(--color-surface-elevated);
	}

	.studio-tab.active {
		background: color-mix(in srgb, var(--color-primary) 12%, transparent);
		color: var(--color-text);
		font-weight: 500;
	}

	.studio-tab.disabled {
		opacity: 0.45;
		cursor: not-allowed;
	}

	.studio-body {
		margin-top: 1.5rem;
		min-width: 0;
	}

	/* The create tab renders CreateShell — de-pad its composer-grid so the
	 * composer sits flush inside this surface (its own padding would double
	 * up with .studio-surface). */
	.studio-surface.is-composer :global(.composer-grid) {
		padding: 0;
		margin: 0;
		max-width: none;
	}
</style>
