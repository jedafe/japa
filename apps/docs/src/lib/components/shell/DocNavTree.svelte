<script lang="ts">
	import { untrack } from 'svelte';
	import { PanelLeftClose, PanelLeft, ChevronDown, BookOpen, Plus, House, ArrowRight, Compass, FileText, Rocket, Star, User, X } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { theme } from '$lib/stores/theme.svelte';
	import type { DocNavTree as DocNavTreeType, DocAudience, DocNavItem } from '$lib/types';
	import { AUDIENCE_LABEL_KEY } from '$lib/types';
	import { AUDIENCE_ICON } from '$lib/audience-meta';
	import { groupNavItems } from '$lib/docs/grouping';
	import { collectCategories, categoryHref, parsePinnedPages, resolvePinnedPages } from '$lib/docs/catalog';
	import { categoryIcon } from '$lib/category-meta';

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

	let { nav, accessibleAudiences, activeAudience, collapsed, onToggleCollapse }: Props = $props();

	// Collapsed rail shows the theme-aware favicon icon (appearance.favicon_url_*),
	// falling back to the bundled favicon.
	const collapsedFavicon = $derived(
		((theme.resolved === 'dark'
			? (page.data.siteSettings as Record<string, string> | undefined)?.['appearance.favicon_url_dark']
			: (page.data.siteSettings as Record<string, string> | undefined)?.['appearance.favicon_url_light']) ??
			(page.data.siteSettings as Record<string, string> | undefined)?.['appearance.favicon_url'] ??
			'/favicon.svg')
	);

	// Expand/collapse state per audience section (keyed by audience slug).
	// Default: the active audience is expanded, others collapsed.
	let expanded = $state<Record<string, boolean>>({});
	$effect(() => {
		if (activeAudience) expanded[activeAudience] = true;
	});

	function toggleSection(audience: string) {
		expanded[audience] = !expanded[audience];
	}

	// Expand/collapse state per category row (keyed by category slug).
	let catExpanded = $state<Record<string, boolean>>({});
	function toggleCategory(slug: string) {
		catExpanded[slug] = !catExpanded[slug];
	}

	// Canonical audience order from the backend (Rule 16).
	const audienceOrder = $derived(page.data?.audienceOrder ?? []);

	// Filter the nav tree to only audiences the session can access (ADR-060:
	// server-side gating — the sidebar never renders gated sections).
	let visibleTrees = $derived(
		nav
			.filter((tree) => accessibleAudiences.includes(tree.audience))
			.sort((a, b) => (audienceOrder.indexOf(a.audience) - audienceOrder.indexOf(b.audience)))
	);

	// BY USER GROUP lists every accessible audience — even ones with no
	// published pages yet (they still link to the audience landing, matching
	// the group cards on /docs). Sorted by canonical backend order. 'api' is
	// always accessible (ADR-060), so its section always appears.
	let groupSections = $derived.by(() => {
		const order = (a: DocAudience) => {
			const i = audienceOrder.indexOf(a);
			return i === -1 ? 99 : i;
		};
		return accessibleAudiences
			.slice()
			.sort((a, b) => order(a) - order(b))
			.map((a) => visibleTrees.find((t) => t.audience === a) ?? { audience: a, items: [] as DocNavItem[] });
	});

	// Categories aggregated across every accessible audience (portal sidebar).
	const categories = $derived(collectCategories(visibleTrees));

	// Get Started pins — docs.pinned_pages setting, resolved to nav links.
	// Unresolvable pins are skipped so the sidebar never dead-links.
	const pins = $derived(
		resolvePinnedPages(
			parsePinnedPages(page.data?.siteSettings?.['docs.pinned_pages']),
			visibleTrees,
			accessibleAudiences
		)
	);
	const quickstartHref = $derived(pins[0]?.href ?? null);

	// "New to JapaConnect?" card dismissal — per-device preference persisted
	// in a cookie (SSR-readable; localStorage kept as a fallback write).
	const NEW_CARD_DISMISS_KEY = 'jc.docs.new_card_dismissed';
	// Dismiss state is persisted in a cookie (readable SSR via +layout.server)
	// so the card never flashes for users who already dismissed it — the old
	// localStorage read happened post-mount and always showed it once first.
	let newCardDismissed = $state(untrack(() => page.data.newCardDismissed === true));
	function dismissNewCard() {
		newCardDismissed = true;
		try {
			document.cookie = `${NEW_CARD_DISMISS_KEY}=1; path=/; max-age=${60 * 60 * 24 * 365}; samesite=lax`;
			localStorage.setItem(NEW_CARD_DISMISS_KEY, '1');
		} catch {
			// Ignore — the card just reappears next visit.
		}
	}

	// GET STARTED pin icons — names come from docs.pinned_pages entries.
	const PIN_ICONS: Record<string, typeof FileText> = {
		file: FileText,
		book: BookOpen,
		rocket: Rocket,
		compass: Compass,
		user: User,
		star: Star
	};

	// On locale-prefixed URLs (/fr/docs/…) page.url retains the prefix, so
	// strip it before comparing against generated /docs/* hrefs — otherwise
	// active-page highlighting silently breaks on localized pages.
	const enabledLangs = $derived(
		((page.data?.siteSettings?.['i18n.enabled_languages'] as string | undefined) ?? 'en')
			.split(',')
			.map((l) => l.trim())
			.filter(Boolean)
	);
	let activePath = $derived.by(() => {
		const p = page.url.pathname;
		const seg = p.split('/')[1];
		return seg && enabledLangs.includes(seg) ? p.slice(seg.length + 1) || '/' : p;
	});

	function hrefFor(audience: DocAudience, slug: string): string {
		return `/docs/${audience}/${slug}`;
	}

	function isActive(item: DocNavItem, audience: DocAudience): boolean {
		return activePath === hrefFor(audience, item.slug);
	}

	// Editor affordance: docs.manage holders get a per-section "new page"
	// shortcut (§44 in-app authoring). Group-name checks are unreliable —
	// the permission node is canonical (DOC-007).
	const canManageDocs = $derived(page.data?.user?.permissions?.includes('docs.manage') ?? false);
</script>

<aside class="nav" class:collapsed data-tour="docs-nav">
	<div class="nav-toolbar" class:centered={collapsed}>
		<button
			type="button"
			class="collapse-btn"
			onclick={onToggleCollapse}
			aria-label={collapsed ? t('a11y.expand_nav') : t('a11y.collapse_nav')}
			title={collapsed ? t('a11y.expand_nav') : t('a11y.collapse_nav')}
		>
			{#if collapsed}
				<PanelLeft class="rtl:-scale-x-100" size={18} />
			{:else}
				<PanelLeftClose class="rtl:-scale-x-100" size={18} />
			{/if}
		</button>
	</div>

	<nav class="nav-body" aria-label={t('a11y.main_nav')}>
		{#if collapsed}
			<!-- Collapsed: brand icon + home + audience initials as icons. -->
			<ul class="rail">
				<li>
					<a href="/" class="rail-item" title={t('nav.home')}>
						<img src={collapsedFavicon} alt="" class="rail-item-img" />
					</a>
				</li>
				<li>
					<a href="/docs" class="rail-item" class:active={activePath === '/docs'} title={t('nav.home')}>
						<House size={18} />
					</a>
				</li>
				{#each groupSections as tree (tree.audience)}
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
		{:else}
			<a href="/docs" class="home-link" class:active={activePath === '/docs'}>
				<House size={16} />
				<span>{t('nav.home')}</span>
			</a>

			{#if pins.length > 0}
				<div class="group">
					<span class="group-label">{t('nav.get_started')}</span>
					<ul class="page-list">
						{#each pins as pin (pin.href)}
							{@const PinIcon = PIN_ICONS[pin.icon ?? 'file'] ?? FileText}
							<li>
								<a href={pin.href} class="page-link pin-link" class:active={activePath === pin.href}>
									<PinIcon size={14} class="pin-icon" />
									<span class="pin-label">{pin.label}</span>
								</a>
							</li>
						{/each}
					</ul>
				</div>
			{/if}

			{#if groupSections.length === 0}
				<p class="nav-empty">{t('nav.empty')}</p>
			{:else}
				<div class="group">
					<span class="group-label">{t('nav.by_user_group')}</span>
					{#each groupSections as tree (tree.audience)}
						{@const AudienceIcon = AUDIENCE_ICON[tree.audience]}
						<div class="section">
							<div class="section-header-row">
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
									<span class="section-title">
										<AudienceIcon size={15} class="section-icon" />
										<span>{t(AUDIENCE_LABEL_KEY[tree.audience], tree.audience)}</span>
									</span>
									{#if tree.items.length > 0}
										<ChevronDown size={14} class={`chevron ${expanded[tree.audience] ?? false ? 'rot' : ''}`} />
									{/if}
								</a>
								{#if canManageDocs}
									<a
										href="/docs/{tree.audience}/new"
										class="new-page-btn"
										title={t('nav.new_page', 'New page')}
										aria-label={t('nav.new_page', 'New page')}
									>
										<Plus size={14} />
									</a>
								{/if}
							</div>
						{#if expanded[tree.audience] ?? false}
							{#if tree.items.length > 0}
								{@const grouped = groupNavItems(tree.items)}
								<ul class="page-list">
									{#each grouped.ungrouped as item (item.id)}
										{@render navItem(item, tree.audience)}
									{/each}
									{#each grouped.categories as cat (cat.name)}
										<li class="cat-group">
											<span class="cat-header">{cat.name}</span>
											<ul class="page-list">
												{#each cat.subcategories as sub (sub.name)}
													{#if sub.name}
														<li class="subcat-group">
															<span class="subcat-header">{sub.name}</span>
															<ul class="page-list">
																{#each sub.items as item (item.id)}
																	{@render navItem(item, tree.audience)}
																{/each}
															</ul>
														</li>
													{:else}
														{#each sub.items as item (item.id)}
															{@render navItem(item, tree.audience)}
														{/each}
													{/if}
												{/each}
											</ul>
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
				</div>
			{/if}

			{#if categories.length > 0}
				<div class="group">
					<span class="group-label">{t('nav.categories')}</span>
					{#each categories as cat (cat.slug)}
						{@const CatIcon = categoryIcon(cat.name)}
						<div class="section">
							<div class="section-header-row">
								<a
									href={categoryHref(cat.name)}
									class="section-header"
									class:active={activePath === categoryHref(cat.name)}
									onclick={(e) => {
										// Toggle expand when already on this category's page.
										if (activePath === categoryHref(cat.name)) {
											e.preventDefault();
											toggleCategory(cat.slug);
										}
									}}
								>
									<span class="section-title">
										<CatIcon size={15} class="section-icon" />
										<span>{cat.name}</span>
									</span>
									<ChevronDown size={14} class={`chevron ${catExpanded[cat.slug] ?? false ? 'rot' : ''}`} />
								</a>
							</div>
						{#if catExpanded[cat.slug] ?? false}
							<ul class="page-list">
								{#each cat.items.slice(0, 8) as item (`${item.audience}/${item.slug}`)}
									<li>
										<a
											href="/docs/{item.audience}/{item.slug}"
											class="page-link"
											class:active={activePath === `/docs/${item.audience}/${item.slug}`}
										>
											{item.title}
										</a>
									</li>
								{/each}
								<li>
									<a href={categoryHref(cat.name)} class="page-link view-all">
										{t('landing.view_all_count', 'View all ({count})', { count: String(cat.items.length) })}
									</a>
								</li>
							</ul>
						{/if}
						</div>
					{/each}
				</div>
			{/if}

		{/if}
	</nav>

	{#if !collapsed && quickstartHref && !newCardDismissed}
		<div class="nav-bottom">
			<div class="new-card">
				<button
					type="button"
					class="new-card-dismiss"
					onclick={dismissNewCard}
					aria-label={t('nav.new_to_dismiss', 'Dismiss')}
				>
					<X size={14} />
				</button>
				<span class="new-card-icon"><Compass size={18} /></span>
				<span class="new-card-title">{t('nav.new_to_title')}</span>
				<span class="new-card-body">{t('nav.new_to_body')}</span>
				<a href={quickstartHref} class="new-card-cta">
					{t('nav.new_to_cta')}
					<ArrowRight class="rtl:-scale-x-100" size={14} />
				</a>
			</div>
		</div>
	{/if}
</aside>

{#snippet navItem(item: DocNavItem, audience: DocAudience)}
	<li>
		<a
			href={hrefFor(audience, item.slug)}
			class="page-link"
			class:active={isActive(item, audience)}
		>
			{item.title}
		</a>
		{#if item.children && item.children.length > 0}
			<ul class="sub-list">
				{#each item.children as child (child.id)}
					<li>
						<a
							href={hrefFor(audience, child.slug)}
							class="page-link sub"
							class:active={isActive(child, audience)}
						>
							{child.title}
						</a>
					</li>
				{/each}
			</ul>
		{/if}
	</li>
{/snippet}

<style>
	.nav {
		display: flex;
		flex-direction: column;
		height: 100%;
		background: var(--color-surface);
		border-inline-end: 1px solid var(--color-border);
		overflow: hidden;
	}
	.nav-toolbar {
		display: flex;
		align-items: center;
		justify-content: flex-end;
		height: 2.75rem;
		padding: 0 0.5rem;
		flex-shrink: 0;
	}
	.nav-toolbar.centered {
		justify-content: center;
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

	.nav-body {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
		padding: 0 0 1rem;
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

	/* Home link */
	.home-link {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		padding: 0.5rem 0.75rem;
		margin: 0 0 0.5rem;
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--color-text-subtle);
		text-decoration: none;
		border-radius: 0;
	}
	.home-link:hover { color: var(--color-text); background: var(--color-surface-subtle); }
	.home-link.active {
		color: var(--color-primary);
		border-inline-start: 2px solid var(--color-primary);
		background: color-mix(in srgb, var(--color-primary) 6%, transparent);
	}

	/* Group section labels (GET STARTED / BY USER GROUP / CATEGORIES) */
	.group { margin-bottom: 0.25rem; }
	.group-label {
		display: block;
		padding: 0.75rem 0.75rem 0.25rem;
		font-size: 0.6875rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.07em;
		color: var(--color-muted);
	}

	/* Collapsed rail */
	.nav.collapsed .nav-body { padding-top: 0.25rem; }
	.rail {
		list-style: none;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.25rem;
		padding: 0;
		margin: 0;
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
	.rail-item-img { width: 18px; height: 18px; object-fit: contain; }
	.rail-item.active {
		background: color-mix(in srgb, var(--color-primary) 12%, transparent);
		color: var(--color-primary);
	}

	/* Audience / category section rows */
	.section { margin-bottom: 0.125rem; }
	.section-header-row {
		display: flex;
		align-items: center;
		gap: 0.25rem;
	}
	.section-header-row .section-header { flex: 1; min-width: 0; }
	.new-page-btn {
		display: grid;
		place-items: center;
		height: 1.5rem;
		width: 1.5rem;
		margin-inline-end: 0.5rem;
		border-radius: var(--radius);
		color: var(--color-muted);
		flex-shrink: 0;
	}
	.new-page-btn:hover { background: var(--color-surface-subtle); color: var(--color-primary); }

	.section-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		padding: 0.5rem 0.75rem;
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--color-text-subtle);
		text-decoration: none;
		border-radius: 0;
	}
	.section-title {
		display: inline-flex;
		align-items: center;
		gap: 0.625rem;
		min-width: 0;
	}
	.section-title :global(.section-icon) {
		color: var(--color-muted);
		flex-shrink: 0;
	}
	.section-header:hover { color: var(--color-text); background: var(--color-surface-subtle); }
	.section-header:hover :global(.section-icon) { color: var(--color-primary); }
	.section-header.active { color: var(--color-primary); }
	.section-header.active :global(.section-icon) { color: var(--color-primary); }
	:global(.chevron) { transition: transform 0.15s; flex-shrink: 0; }
	:global(.chevron.rot) { transform: rotate(180deg); }

	/* Category/subcategory group headers (§44 taxonomy) */
	.cat-group { margin-top: 0.25rem; }
	.cat-header {
		display: block;
		padding: 0.375rem 0.75rem 0.125rem 1rem;
		font-size: 0.6875rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--color-muted);
	}
	.subcat-group { margin-top: 0.125rem; }
	.subcat-header {
		display: block;
		padding: 0.25rem 0.75rem 0.125rem 1.75rem;
		font-size: 0.6875rem;
		font-weight: 500;
		color: var(--color-text-subtle);
	}
	.cat-group > .page-list > li > .page-link { padding-inline-start: 1.75rem; }
	.subcat-group .page-list .page-link { padding-inline-start: 2.5rem; }
	.subcat-group .page-list .page-link.sub { padding-inline-start: 3rem; }

	.page-list, .sub-list {
		list-style: none;
		padding: 0 0 0.25rem;
		margin: 0;
	}
	.sub-list { padding-inline-start: 0.75rem; }
	.page-link {
		display: block;
		padding: 0.375rem 0.75rem 0.375rem 1.75rem;
		font-size: 0.8125rem;
		color: var(--color-text-subtle);
		text-decoration: none;
		border-inline-start: 2px solid transparent;
		line-height: 1.4;
	}
	.page-link:hover { color: var(--color-text); background: var(--color-surface-subtle); }
	.page-link.active {
		color: var(--color-primary);
		border-inline-start-color: var(--color-primary);
		background: color-mix(in srgb, var(--color-primary) 6%, transparent);
		font-weight: 500;
	}
	.page-link.sub { padding-inline-start: 2rem; }
	.page-link.view-all {
		color: var(--color-primary);
		font-weight: 500;
	}
	/* Get Started pins — icon rows, slightly tighter than tree links. */
	.pin-link {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		padding-inline-start: 0.75rem;
	}
	.pin-link :global(.pin-icon) { color: var(--color-muted); flex-shrink: 0; }
	.pin-link:hover :global(.pin-icon), .pin-link.active :global(.pin-icon) {
		color: var(--color-primary);
	}
	.pin-label {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	/* "New to JapaConnect?" card — pinned below the scrollable nav while the
	   sidebar is expanded. */
	.nav-bottom {
		flex-shrink: 0;
		padding: 0.75rem;
		border-top: 1px solid var(--color-border);
	}
	.new-card {
		position: relative;
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		padding: 1rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		background: var(--color-surface-subtle);
	}
	.new-card-dismiss {
		position: absolute;
		top: 0.5rem;
		inset-inline-end: 0.5rem;
		display: grid;
		place-items: center;
		height: 1.5rem;
		width: 1.5rem;
		border: none;
		border-radius: var(--radius);
		background: transparent;
		color: var(--color-muted);
		cursor: pointer;
	}
	.new-card-dismiss:hover {
		background: var(--color-surface);
		color: var(--color-text);
	}
	.new-card-icon {
		display: grid;
		place-items: center;
		height: 2rem;
		width: 2rem;
		border-radius: var(--radius);
		background: color-mix(in srgb, var(--color-primary) 12%, transparent);
		color: var(--color-primary);
	}
	.new-card-title {
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--color-text);
	}
	.new-card-body {
		font-size: 0.75rem;
		color: var(--color-text-subtle);
		line-height: 1.5;
	}
	.new-card-cta {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		margin-top: 0.375rem;
		font-size: 0.8125rem;
		font-weight: 600;
		color: var(--color-primary);
		text-decoration: none;
	}
	.new-card-cta:hover { text-decoration: underline; }
</style>
