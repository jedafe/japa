<script lang="ts">
	// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md — shared studio
	// chrome. The whole /studio surface renders as an ADR-058 exception in
	// composer chrome (same visual language as CreateShell's composer-grid):
	// centered 1200px surface, composer-header bar, segmented tab control.
	import { page } from '$app/state';
	import type { LayoutData } from './$types';
	import type { Snippet } from 'svelte';
	import { t } from '#lib/i18n/index.js';
	import { Modal } from '#lib/components/ui/index.js';
	import { studioMobile } from '#lib/studio/mobileView.svelte.js';
	import { Files, Clapperboard, TerminalSquare, SquarePen, Shield, SlidersHorizontal, Shapes, Check } from 'lucide-svelte';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	const videoEnabled = $derived(data.videoEnabled);
	const moderationEnabled = $derived(
		!!data.moderationScope &&
			(data.moderationScope.platform_moderator ||
				data.moderationScope.moderated_spaces.length > 0 ||
				data.moderationScope.moderated_tags.length > 0 ||
				data.moderationScope.assigned_count > 0)
	);
	const isCreateTab = $derived(page.route.id === '/(app)/studio/create');

	const tabs = $derived.by(() => {
		const list = [
			{ href: '/studio/contents', label: t('studio.tabs.contents', 'Contents'), icon: Files, disabled: false },
			{
				href: '/studio/videos',
				label: t('studio.tabs.videos', 'Videos'),
				icon: Clapperboard,
				disabled: !videoEnabled
			},
			{ href: '/studio/prompts', label: t('studio.tabs.prompts', 'Prompts'), icon: TerminalSquare, disabled: false },
			{ href: '/studio/create', label: t('studio.tabs.create', 'Create'), icon: SquarePen, disabled: false }
		];
		if (moderationEnabled) {
			list.splice(1, 0, {
				href: '/studio/moderation',
				label: t('studio.tabs.moderation', 'Moderation'),
				icon: Shield,
				disabled: false
			});
		}
		return list;
	});

	function isActive(href: string): boolean {
		return page.url.pathname === href || page.url.pathname.startsWith(href + '/');
	}

	// Mobile composer FABs — only the two composer-style tabs (Prompts,
	// Create) get the editor/config swapper and the type-picker modal, and
	// only once the page registered its type bridge (studioMobile.typeItems
	// stays empty on load errors).
	const isComposerTab = $derived(
		page.route.id === '/(app)/studio/prompts' || page.route.id === '/(app)/studio/create'
	);
	const showMobileFabs = $derived(isComposerTab && studioMobile.typeItems.length > 0);
	let typeModalOpen = $state(false);
</script>

<svelte:head>
	<!-- Creator studio is private — not indexed. -->
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

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

<!-- Mobile: the segmented header control is replaced by a fixed bottom
     tab bar (same idiom as the shell's MobileBottomTabBar, which is
     suppressed on this exception surface). -->
<nav class="studio-mobile-tabs" aria-label={t('studio.tabs.label', 'Studio sections')}>
	{#each tabs as tab (tab.href)}
		{#if tab.disabled}
			<span
				class="studio-mobile-tab disabled"
				title={tab.href === '/studio/videos' && !videoEnabled
					? t('studio.tabs.videos_disabled', 'Video is not enabled on this site')
					: t('studio.tabs.coming_soon', 'Coming soon')}
			>
				<tab.icon size={20} />
				<span class="studio-mobile-tab-label">{tab.label}</span>
			</span>
		{:else}
			<a
				href={tab.href}
				class="studio-mobile-tab"
				class:active={isActive(tab.href)}
				aria-current={isActive(tab.href) ? 'page' : undefined}
			>
				<tab.icon size={20} />
				<span class="studio-mobile-tab-label">{tab.label}</span>
			</a>
		{/if}
	{/each}
</nav>

{#if showMobileFabs}
	<!-- Mobile composer actions — stacked above the bottom tab bar. The
	     type picker only appears while the editor column is active. -->
	{#if studioMobile.section === 'editor'}
		<button
			type="button"
			class="studio-fab studio-fab-type"
			onclick={() => (typeModalOpen = true)}
			aria-label={t('studio.mobile.pick_type', 'Choose content type')}
			title={t('studio.mobile.pick_type', 'Choose content type')}
		>
			<Shapes size={20} />
		</button>
	{/if}
	<button
		type="button"
		class="studio-fab studio-fab-toggle"
		onclick={() => (studioMobile.section = studioMobile.section === 'editor' ? 'config' : 'editor')}
		aria-label={studioMobile.section === 'editor'
			? t('studio.mobile.show_config', 'Show settings')
			: t('studio.mobile.show_editor', 'Show editor')}
		title={studioMobile.section === 'editor'
			? t('studio.mobile.show_config', 'Show settings')
			: t('studio.mobile.show_editor', 'Show editor')}
	>
		{#if studioMobile.section === 'editor'}
			<SlidersHorizontal size={20} />
		{:else}
			<SquarePen size={20} />
		{/if}
	</button>
{/if}

<Modal
	bind:open={typeModalOpen}
	title={t('create.type_placeholder', 'Content type')}
	icon={Shapes}
	size="sm"
>
	{#snippet children(close)}
		<div class="type-modal-list">
			{#each studioMobile.typeItems as item (item.id)}
				<button
					type="button"
					class="type-modal-item"
					class:selected={item.id === studioMobile.selectedTypeId}
					disabled={studioMobile.typeLocked}
					onclick={() => {
						studioMobile.selectType?.(item.id);
						close();
					}}
				>
					<span>{item.label}</span>
					{#if item.id === studioMobile.selectedTypeId}
						<Check size={16} />
					{/if}
				</button>
			{/each}
		</div>
		{#if studioMobile.typeLocked}
			<p class="type-modal-hint">
				{t('create.type_locked', 'Content type cannot be changed after creation')}
			</p>
		{/if}
	{/snippet}
</Modal>

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

	/* Segmented control in the composer-type-select idiom. On narrow
	 * viewports the four tabs don't all fit — scroll horizontally instead
	 * of clipping the last tab. */
	.studio-tabs {
		display: flex;
		height: 2.375rem;
		max-width: 100%;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		overflow-x: auto;
		overflow-y: hidden;
		scrollbar-width: none;
	}
	.studio-tabs::-webkit-scrollbar {
		display: none;
	}

	.studio-tab {
		display: inline-flex;
		align-items: center;
		flex-shrink: 0;
		gap: 6px;
		padding: 0 12px;
		font-size: 0.875rem;
		white-space: nowrap;
		color: var(--color-muted);
		text-decoration: none;
		transition: background 0.15s, color 0.15s;
	}

	.studio-tab + .studio-tab {
		border-inline-start: 1px solid var(--color-border);
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

	/* Mobile bottom tab bar — desktop-only above 640px where the segmented
	 * header control takes over. */
	.studio-mobile-tabs {
		position: fixed;
		inset-inline: 0;
		bottom: 0;
		z-index: 40;
		display: flex;
		align-items: stretch;
		border-top: 1px solid var(--color-border);
		background: color-mix(in srgb, var(--color-surface) 95%, transparent);
		backdrop-filter: blur(8px);
		padding-bottom: calc(0.5rem + env(safe-area-inset-bottom));
	}

	.studio-mobile-tab {
		flex: 1;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 2px;
		padding: 0.5rem 0;
		font-size: 10px;
		color: var(--color-muted);
		text-decoration: none;
		transition: color 0.15s;
	}

	.studio-mobile-tab.active {
		color: var(--color-primary-text);
	}

	.studio-mobile-tab.disabled {
		opacity: 0.45;
	}

	.studio-mobile-tab-label {
		line-height: 1;
	}

	/* Mobile composer FABs — stacked above the bottom tab bar, lower-right. */
	.studio-fab {
		position: fixed;
		inset-inline-end: 1rem;
		z-index: 40;
		display: grid;
		place-items: center;
		width: 3rem;
		height: 3rem;
		border: none;
		border-radius: 9999px;
		background: var(--color-primary);
		color: var(--color-primary-contrast);
		box-shadow: 0 4px 12px rgb(0 0 0 / 0.25);
		cursor: pointer;
	}

	.studio-fab-toggle {
		bottom: calc(4.5rem + env(safe-area-inset-bottom));
	}

	.studio-fab-type {
		bottom: calc(8.25rem + env(safe-area-inset-bottom));
		background: var(--color-surface);
		color: var(--color-text);
		border: 1px solid var(--color-border);
	}

	/* Type-picker modal option list. */
	.type-modal-list {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.type-modal-item {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		padding: 0.625rem 0.75rem;
		border: 1px solid transparent;
		border-radius: var(--radius);
		background: transparent;
		color: var(--color-text);
		font-size: 0.875rem;
		text-align: start;
		cursor: pointer;
		transition: background 0.15s;
	}

	.type-modal-item:hover:not(:disabled) {
		background: var(--color-surface-elevated);
	}

	.type-modal-item.selected {
		border-color: var(--color-primary);
		background: color-mix(in srgb, var(--color-primary) 10%, transparent);
		color: var(--color-primary-text);
		font-weight: 500;
	}

	.type-modal-item:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.type-modal-hint {
		margin: 0.75rem 0 0;
		font-size: 0.75rem;
		color: var(--color-muted);
	}

	@media (max-width: 639px) {
		/* The whole header (title + segmented control) gives way to the
		 * fixed bottom tab bar — the active tab already labels the page. */
		.studio-header {
			display: none;
		}

		.studio-body {
			margin-top: 0;
		}

		.studio-surface {
			padding: 1rem;
		}

		/* Scroll clearance for the fixed bottom bar. */
		.studio-body {
			padding-bottom: calc(4rem + env(safe-area-inset-bottom));
		}
	}

	@media (min-width: 640px) {
		.studio-mobile-tabs,
		.studio-fab {
			display: none;
		}
	}
</style>
