<script lang="ts">
	import { PanelLeftClose, PanelLeft, ChevronDown, BookOpen } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import type { DocNavTree as DocNavTreeType, DocAudience, DocNavItem } from '$lib/types';
	import { AUDIENCE_LABEL_KEY } from '$lib/types';
	import ThemeLogo from '$lib/components/branding/ThemeLogo.svelte';
	import AudienceSwitcher from './AudienceSwitcher.svelte';

	interface Props {
		nav: DocNavTreeType[];
		accessibleAudiences: DocAudience[];
		activeAudience: DocAudience | null;
		collapsed: boolean;
		onToggleCollapse: () => void;
		/** Platform logo URL from appearance.logo_url (control panel). */
		logoUrl?: string;
		/** Platform dark-theme logo URL from appearance.logo_url_dark (control panel). */
		logoDarkUrl?: string;
	}

	let { nav, accessibleAudiences, activeAudience, collapsed, onToggleCollapse, logoUrl = '', logoDarkUrl = '' }: Props = $props();

	// Expand/collapse state per audience section (keyed by audience slug).
	// Default: the active audience is expanded, others collapsed.
	let expanded = $state<Record<string, boolean>>({});
	$effect(() => {
		if (activeAudience) expanded[activeAudience] = true;
	});

	function toggleSection(audience: string) {
		expanded[audience] = !expanded[audience];
	}

	// Filter the nav tree to only audiences the session can access (ADR-060:
	// server-side gating — the switcher only offers accessibleAudiences, and
	// we never render gated section groups).
	let visibleTrees = $derived(
		nav.filter((tree) => accessibleAudiences.includes(tree.audience))
	);

	// Ensure the API audience is always visible in the nav tree (ADR-060:
	// /docs/api is always public). The backend only returns audiences with
	// published pages, so we add a fallback entry for 'api' when missing.
	let treesWithApi = $derived(
		visibleTrees.some((t) => t.audience === 'api')
			? visibleTrees
			: [...visibleTrees, { audience: 'api' as const, items: [] }]
	);

	let activePath = $derived(page.url.pathname);

	function hrefFor(audience: DocAudience, slug: string): string {
		return `/docs/${audience}/${slug}`;
	}

	function isActive(item: DocNavItem, audience: DocAudience): boolean {
		return activePath === hrefFor(audience, item.slug);
	}
</script>

<aside class="nav" class:collapsed>
	<div class="nav-header">
		<a href="/docs" class="nav-brand" class:hidden={collapsed}>
			{#if logoUrl}
				<ThemeLogo {logoUrl} {logoDarkUrl} alt={t('app.name')} class="h-8 w-auto max-w-[160px] object-contain shrink-0" />
			{:else}
				<span class="brand-mark">JC</span>
			{/if}
			<span class="brand-name">{t('app.name')}</span>
		</a>
		<button
			type="button"
			class="collapse-btn"
			onclick={onToggleCollapse}
			aria-label={collapsed ? t('a11y.expand_nav') : t('a11y.collapse_nav')}
			title={collapsed ? t('a11y.expand_nav') : t('a11y.collapse_nav')}
		>
			{#if collapsed}
				<PanelLeft size={18} />
			{:else}
				<PanelLeftClose size={18} />
			{/if}
		</button>
	</div>

	{#if !collapsed}
		<div class="nav-audience">
			<AudienceSwitcher {accessibleAudiences} {activeAudience} />
		</div>
	{/if}

	<nav class="nav-body" aria-label={t('a11y.main_nav')}>
		{#if collapsed}
			<!-- Collapsed: show audience initials as icons. -->
			<ul class="rail">
				{#each treesWithApi as tree (tree.audience)}
					<li>
						<a
							href="/docs/{tree.audience}"
							class="rail-item"
							class:active={activeAudience === tree.audience}
							title={t(AUDIENCE_LABEL_KEY[tree.audience], tree.audience)}
						>
							<BookOpen size={18} />
						</a>
					</li>
				{/each}
			</ul>
		{:else if treesWithApi.length === 0}
			<p class="nav-empty">{t('nav.empty')}</p>
		{:else}
			{#each treesWithApi as tree (tree.audience)}
				<div class="section">
					<a
						href="/docs/{tree.audience}"
						class="section-header"
						class:active={activeAudience === tree.audience && !activePath.includes(`/${tree.audience}/`)}
						onclick={(e) => {
							// Toggle expand when clicking the header if it's the active audience.
							if (activeAudience === tree.audience) {
								e.preventDefault();
								toggleSection(tree.audience);
							}
						}}
					>
						<span>{t(AUDIENCE_LABEL_KEY[tree.audience], tree.audience)}</span>
						{#if tree.items.length > 0}
							<ChevronDown size={14} class={`chevron ${expanded[tree.audience] ?? true ? 'rot' : ''}`} />
						{/if}
					</a>
				{#if expanded[tree.audience] ?? true}
					{#if tree.items.length > 0}
						<ul class="page-list">
							{#each tree.items as item (item.id)}
								<li>
									<a
										href={hrefFor(tree.audience, item.slug)}
										class="page-link"
										class:active={isActive(item, tree.audience)}
									>
										{item.title}
									</a>
									{#if item.children && item.children.length > 0}
										<ul class="sub-list">
											{#each item.children as child (child.id)}
												<li>
													<a
														href={hrefFor(tree.audience, child.slug)}
														class="page-link sub"
														class:active={isActive(child, tree.audience)}
													>
														{child.title}
													</a>
												</li>
											{/each}
										</ul>
									{/if}
								</li>
							{/each}
						</ul>
					{:else if tree.audience === 'api'}
						<!-- API audience with no published pages: show a helpful hint -->
						<p class="nav-empty">
							<a href="/docs/api" class="api-hint">{t('api.browse_endpoints', 'Browse API endpoints →')}</a>
						</p>
					{/if}
				{/if}
				</div>
			{/each}
		{/if}
	</nav>
</aside>

<style>
	.nav {
		display: flex;
		flex-direction: column;
		height: 100vh;
		background: var(--color-surface);
		border-right: 1px solid var(--color-border);
		overflow: hidden;
	}
	.nav-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		height: var(--nav-height);
		padding: 0 0.75rem;
		border-bottom: 1px solid var(--color-border);
		flex-shrink: 0;
	}
	.nav-brand {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-weight: 600;
		color: var(--color-text);
		text-decoration: none;
		min-width: 0;
	}
	.nav-brand.hidden { display: none; }
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
		flex-shrink: 0;
	}
	.brand-name {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.collapse-btn {
		display: grid;
		place-items: center;
		height: 2rem;
		width: 2rem;
		border-radius: var(--radius);
		color: var(--color-muted);
		background: transparent;
		border: none;
		cursor: pointer;
		flex-shrink: 0;
	}
	.collapse-btn:hover { background: var(--color-surface-subtle); color: var(--color-text); }

	.nav-audience {
		padding: 0.5rem 0.75rem;
		border-bottom: 1px solid var(--color-border);
		flex-shrink: 0;
	}

	.nav-body {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
		padding: 0.5rem 0 2rem;
	}

	.nav-empty {
		padding: 1rem 0.75rem;
		font-size: 0.8125rem;
		color: var(--color-muted);
	}
	.api-hint {
		font-size: 0.8125rem;
		color: var(--color-primary);
		text-decoration: none;
	}
	.api-hint:hover {
		text-decoration: underline;
	}

	/* Collapsed rail */
	.nav.collapsed .nav-body { padding: 0.5rem 0; }
	.rail {
		list-style: none;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.25rem;
		padding: 0;
	}
	.rail-item {
		display: grid;
		place-items: center;
		height: 2.25rem;
		width: 2.25rem;
		border-radius: var(--radius);
		color: var(--color-muted);
	}
	.rail-item:hover { background: var(--color-surface-subtle); color: var(--color-text); }
	.rail-item.active {
		background: color-mix(in srgb, var(--color-primary) 12%, transparent);
		color: var(--color-primary);
	}

	/* Expanded sections (Stripe-docs style) */
	.section { margin-bottom: 0.25rem; }
	.section-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		padding: 0.5rem 0.75rem;
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-text-subtle);
		text-decoration: none;
	}
	.section-header:hover { color: var(--color-text); }
	.section-header.active { color: var(--color-primary); }
	:global(.chevron) { transition: transform 0.15s; flex-shrink: 0; }
	:global(.chevron.rot) { transform: rotate(180deg); }

	.page-list, .sub-list {
		list-style: none;
		padding: 0 0 0.25rem;
		margin: 0;
	}
	.sub-list { padding-left: 0.75rem; }
	.page-link {
		display: block;
		padding: 0.375rem 0.75rem 0.375rem 1rem;
		font-size: 0.875rem;
		color: var(--color-text-subtle);
		text-decoration: none;
		border-left: 2px solid transparent;
		line-height: 1.4;
	}
	.page-link:hover { color: var(--color-text); background: var(--color-surface-subtle); }
	.page-link.active {
		color: var(--color-primary);
		border-left-color: var(--color-primary);
		background: color-mix(in srgb, var(--color-primary) 6%, transparent);
		font-weight: 500;
	}
	.page-link.sub { padding-left: 1.5rem; font-size: 0.8125rem; }
</style>
