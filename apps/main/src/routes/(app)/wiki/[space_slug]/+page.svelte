<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md
	// Guide-style destination landing: breadcrumb + save/share, flag + title +
	// tagline, category tab strip, hero image, intro, and section cards — with
	// a wiki-specific right sidebar (quick facts, related, popular cities)
	// registered on the shared rail via SetSidebar (ADR-058).
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { i18n, t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { requireAuthOrRedirect } from '$lib/auth/guest';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import ShareButton from '$lib/components/ui/ShareButton.svelte';
	import SetSidebar from '$lib/components/shell/SetSidebar.svelte';
	import EntityFieldsPanel from '$lib/components/wiki/EntityFieldsPanel.svelte';
	import WikiTabs from '$lib/components/wiki/WikiTabs.svelte';
	import WikiSidebar from '$lib/components/wiki/WikiSidebar.svelte';
	import { categoryIconResolved, categoryTint } from '$lib/components/wiki/icons';
	import { decodeBase64Text } from '$lib/tiptap/serialize';
	import {
		BookOpen,
		Bookmark,
		BookmarkCheck,
		ChevronRight,
		CircleHelp,
		Lock,
		Plus,
		Star,
		Table2
	} from 'lucide-svelte';
	import type { WikiPageIndex } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// The canonical category list is scoped to the space type server-side:
	// university spaces get the university set (§80 amendment).
	type ApiCategory = {
		slug: string;
		label_key: string;
		label: string;
		/** §80 lazy template pages — declared sibling page links under the
		 *  category ({name} substituted server-side). Present only on
		 *  ?space= requests. Nested topic templates carry parent_template_id
		 *  and stay off top-level cards (§80 Wiki Topic Pages). */
		page_templates?: { slug: string; label: string; position: number; parent_template_id?: string }[];
		/** Stub page slug for /wiki/{space}/{page_slug} links. */
		page_slug?: string;
		/** §80 directories — the stub page is the child-space listing. */
		directory?: boolean;
		/** §80 directories — true when the space has no matching
		 *  descendants; the card/tab is hidden. */
		empty?: boolean;
		/** §80 category icon — admin-configured lucide name. */
		icon?: string;
	};
	// svelte-ignore state_referenced_locally
	const apiCategories: ApiCategory[] = data.apiCategories ?? [];
	// svelte-ignore state_referenced_locally
	const isUniversityWiki = data.wikiScope === 'university';

	function categoryDesc(cat: string): string {
		return t(`wiki.category_desc_${cat}`, t('wiki.category_desc_other', 'Guides and resources.'), { name: spaceName });
	}

	function categoryLabel(slug: string): string {
		const c = apiCategories.find((x) => x.slug === slug);
		return t(c?.label_key ?? `wiki.category_${slug}`, c?.label ?? slug.replace(/_/g, ' '));
	}

	const space = $derived(data.spaceDetail?.space ?? null);
	const spaceName = $derived(
		data.spaceDetail?.space?.name ?? data.index[0]?.space_name ?? data.spaceSlug
	);
	const spaceLevel = $derived(data.spaceDetail?.space?.level ?? '');
	const spaceId = $derived(data.spaceDetail?.space?.id ?? data.index[0]?.space_id ?? '');
	const logoUrl = $derived(data.spaceDetail?.logo_url ?? data.spaceDetail?.space?.logo_url ?? '');

	// Wiki-content JSON (base64-encoded in the API response) carries the
	// auto-populated {title, extract, description, thumbnail, content_url}.
	const wikiData = $derived.by(() => {
		const raw = space?.wiki_content;
		if (!raw) return null;
		try {
			return typeof raw === 'string' ? JSON.parse(decodeBase64Text(raw)) : raw;
		} catch {
			return null;
		}
	});

	// Only the platform-stored header is rendered here — the auto-populated
	// thumbnail URL points at Wikimedia and would hotlink on every view.
	const heroUrl = $derived(data.spaceDetail?.header_url || '');
	// §80 attractions — the `kind` select value humanized for the hero
	// chip ("historic-site" → "Historic Site"); empty when unset.
	const kindLabel = $derived(
		(data.spaceKind ?? '')
			.replace(/[-_]+/g, ' ')
			.replace(/\b\w/g, (c) => c.toUpperCase())
	);
	const intro = $derived.by(() => {
		const desc = space?.description?.trim();
		if (desc) return desc;
		const raw = wikiData?.extract;
		const extract = typeof raw === 'string' ? raw.trim() : '';
		if (!extract) return '';
		const cut = extract.slice(0, 400);
		const stop = cut.lastIndexOf('. ');
		return (stop > 80 ? cut.slice(0, stop + 1) : cut) + (extract.length > cut.length ? '…' : '');
	});
	const tagline = $derived(
		isUniversityWiki
			? t('wiki.guide_tagline_university', 'Your complete guide to {name} — admissions, programs, tuition, and campus life.', { name: spaceName })
			: t('wiki.guide_tagline', 'Your complete guide to living, studying, and building a successful life in {name}.', { name: spaceName })
	);
	const crumbKind = $derived(
		isUniversityWiki
			? t('wiki.breadcrumb_university_guides', 'University Guides')
			: spaceLevel === 'city'
				? t('wiki.breadcrumb_city_guides', 'City Guides')
				: spaceLevel === 'state'
					? t('wiki.breadcrumb_state_guides', 'State Guides')
					: t('wiki.breadcrumb_country_guides', 'Country Guides')
	);

	// Section cards — one card per category (matching the guide design),
	// ordered by the canonical category order; sibling pages within a
	// category surface as sub-links so every page stays reachable.
	interface CardGroup {
		category: string;
		label: string;
		pages: WikiPageIndex[];
		/** §80 lazy template pages — declared slugs with no materialized
		 *  page yet, rendered as discoverable sub-links. */
		templates: { slug: string; label: string }[];
		/** §80 lazy categories — the stub page slug, when the canonical
		 *  category declares one. */
		pageSlug?: string;
		/** §80 directories — the stub is the child listing; the card links
		 *  it rather than a sibling page. */
		directory: boolean;
		/** §80 category icon — admin-configured lucide name override. */
		icon?: string;
	}
	// §80 directories: a directory-mode category with zero matching
	// descendants (empty:true) earns no card — the stub lists nothing.
	const hiddenDirCats = new Set(
		apiCategories.filter((c) => c.directory && c.empty).map((c) => c.slug)
	);
	const cardGroups = $derived.by<CardGroup[]>(() => {
		const order = new Map(apiCategories.map((c, i) => [c.slug, i]));
		const groups = new Map<string, WikiPageIndex[]>();
		for (const p of data.index) {
			if (hiddenDirCats.has(p.category)) continue;
			const arr = groups.get(p.category);
			if (arr) arr.push(p);
			else groups.set(p.category, [p]);
		}
		// Every declared category is lazy-resolvable now — a stub-only
		// category still gets a card so its listing/links stay reachable.
		const cats = new Set(groups.keys());
		for (const c of apiCategories) {
			if (c.page_slug && !hiddenDirCats.has(c.slug)) cats.add(c.slug);
		}
		return [...cats]
			.map((category) => {
				const pages = [...(groups.get(category) ?? [])].sort(
					(a, b) => Number(b.is_featured) - Number(a.is_featured)
				);
				const materialized = new Set(pages.map((p) => p.slug));
				const apiCat = apiCategories.find((c) => c.slug === category);
				const templates = (apiCat?.page_templates ?? [])
					// §80 Wiki Topic Pages: nested topics are reached through
					// their parent page's composite sections, not the index.
					.filter((tm) => !materialized.has(tm.slug) && !tm.parent_template_id)
					.map((tm) => ({ slug: tm.slug, label: tm.label }));
				return {
					category,
					label: categoryLabel(category),
					pages,
					templates,
					pageSlug: apiCat?.page_slug,
					directory: apiCat?.directory ?? false,
					icon: apiCat?.icon
				};
			})
			.sort((a, b) => {
				const ca = order.get(a.category) ?? Number.MAX_SAFE_INTEGER;
				const cb = order.get(b.category) ?? Number.MAX_SAFE_INTEGER;
				return ca !== cb ? ca - cb : a.label.localeCompare(b.label);
			});
	});

	const lastUpdated = $derived.by(() => {
		const ts = data.index
			.map((p) => Date.parse(p.updated_at))
			.filter((v) => Number.isFinite(v));
		if (ts.length === 0) return '';
		// i18n.locale is request-scoped (page.data.locale) so the SSR and
		// hydrated renders agree — `undefined` would use server vs browser
		// default locales and trigger a hydration text mismatch.
		return new Date(Math.max(...ts)).toLocaleDateString(i18n.locale, {
			year: 'numeric',
			month: 'long',
			day: 'numeric'
		});
	});

	const title = $derived(`${spaceName} ${t('wiki.heading')} — JapaConnect`);
	const description = $derived(intro || t('wiki.index_description'));
	const ogImage = $derived(
		heroUrl ? (heroUrl.startsWith('http') ? heroUrl : page.url.origin + heroUrl) : ''
	);
	const canonicalUrl = $derived(page.url.origin + page.url.pathname);
	const wikiEnabled = $derived(data.features?.['wiki.enabled'] !== false);
	const canCreate = $derived(data.authed && data.userPermissions?.includes('wiki.edit'));

	// §80 community field ops — the space itself is a community-editable
	// entity; its field groups render as a data panel below the guide cards.
	const structuredEditsOn = $derived(
		wikiEnabled && data.features?.['wiki.structured_edits'] === true
	);

	// "Save" follows the destination space (POST/DELETE /spaces/{slug}/follow)
	// with optimistic UI + rollback — same toggle as the space page header.
	let saved = $state(untrack(() => data.spaceDetail?.is_following ?? false));
	let savePending = $state(false);

	async function toggleSave() {
		if (savePending || !space) return;
		if (!(await requireAuthOrRedirect())) return;
		const was = saved;
		saved = !was;
		savePending = true;
		const res = was
			? await api.delete<{ following: boolean }>(`/spaces/${encodeURIComponent(space.slug)}/follow`)
			: await api.post<{ following: boolean }>(`/spaces/${encodeURIComponent(space.slug)}/follow`);
		if (res.error) {
			saved = was;
		} else if (typeof res.data?.following === 'boolean') {
			saved = res.data.following;
		}
		savePending = false;
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={canonicalUrl} />
	{#if ogImage}
		<meta property="og:image" content={ogImage} />
		<meta name="twitter:image" content={ogImage} />
		<meta name="twitter:card" content="summary_large_image" />
	{/if}
	<link rel="canonical" href={canonicalUrl} />
</svelte:head>

<SetSidebar>
	<WikiSidebar
		spaceSlug={data.spaceSlug}
		{spaceName}
		{spaceId}
		{spaceLevel}
		pages={data.index}
		wikiOnly={data.spaceDetail?.wiki_only ?? false}
	/>
</SetSidebar>

<div class="mx-auto w-full max-w-full">
	<!-- Breadcrumb + actions -->
	<div class="mx-4 flex items-center justify-between gap-3 border-b border-[var(--color-border)] px-4 py-2.5 sm:mx-6 sm:px-6">
		<nav aria-label={t('a11y.breadcrumb', 'Breadcrumb')} class="min-w-0 text-xs text-[var(--color-muted)]">
			<ol class="flex items-center gap-1.5">
				<li>
					<a href="/wiki" class="transition-colors hover:text-[var(--color-text)]">
						{t('wiki.heading', 'Wiki')}
					</a>
				</li>
				{#if data.ancestors?.length}
					{#each data.ancestors as anc (anc.slug)}
						<li aria-hidden="true"><ChevronRight size={12} /></li>
						<li>
							<a
								href="/wiki/{encodeURIComponent(anc.slug)}"
								class="truncate transition-colors hover:text-[var(--color-text)]"
							>
								{anc.name}
							</a>
						</li>
					{/each}
				{:else}
					<li aria-hidden="true"><ChevronRight size={12} /></li>
					<li>{crumbKind}</li>
				{/if}
				<li aria-hidden="true"><ChevronRight size={12} /></li>
				<li class="truncate font-medium text-[var(--color-text)]">{spaceName}</li>
			</ol>
		</nav>
		{#if data.ancestorsAlt?.length}
			<!-- §80 multi-parent spaces: the alternate parent links not picked
			     into the breadcrumb chain ("also under X"). -->
			<p class="min-w-0 text-xs text-[var(--color-muted)]">
				{t('wiki.also_under', 'Also under:')}
				{#each data.ancestorsAlt as alt, i (alt.slug)}
					{#if i > 0},
					{/if}<a
						href="/wiki/{encodeURIComponent(alt.slug)}"
						class="transition-colors hover:text-[var(--color-text)]"
					>{alt.name}</a>
				{/each}
			</p>
		{/if}
		<div class="flex shrink-0 items-center gap-2">
			{#if canCreate && data.index.length > 0}
				<a
					href="/wiki/{encodeURIComponent(data.spaceSlug)}/create"
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					<Plus size={14} /> {t('wiki.create_page', 'New page')}
				</a>
			{/if}
			<button
				type="button"
				onclick={toggleSave}
				disabled={savePending || !space}
				aria-pressed={saved}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
			>
				{#if saved}
					<BookmarkCheck size={14} class="text-[var(--color-primary)]" />
					{t('wiki.saved', 'Saved')}
				{:else}
					<Bookmark size={14} />
					{t('wiki.save', 'Save')}
				{/if}
			</button>
			<ShareButton
				url={page.url.href}
				title={spaceName}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
				size={14}
			/>
		</div>
	</div>

	<!-- Title block: flag/logo + name + tagline -->
	<div class="flex items-start gap-3 px-4 pt-5 sm:px-6">
		{#if logoUrl}
			<img
				src={logoUrl}
				alt=""
				class="h-11 w-11 shrink-0 rounded-[var(--radius)] border border-[var(--color-border)] object-cover"
			/>
		{/if}
		<div class="min-w-0">
			<div class="flex flex-wrap items-center gap-2">
				<h1 class="text-2xl font-semibold text-[var(--color-text)]">{spaceName}</h1>
				{#if kindLabel}
					<span
						class="inline-flex items-center rounded-full border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-2 py-0.5 text-[11px] font-medium text-[var(--color-muted)]"
					>
						{kindLabel}
					</span>
				{/if}
			</div>
			<p class="mt-0.5 text-sm text-[var(--color-muted)]">{tagline}</p>
		</div>
	</div>

	<!-- Category tab strip -->
	<div class="px-1 sm:px-3">
		<WikiTabs spaceSlug={data.spaceSlug} categories={apiCategories} pages={data.index} active="overview" />
	</div>

	<div class="px-4 pb-8 sm:px-6">
		{#if heroUrl}
			<img
				src={heroUrl}
				alt={spaceName}
				class="mt-4 max-h-72 w-full rounded-[12px] border border-[var(--color-border)] object-cover"
			/>
		{/if}

		{#if !wikiEnabled && data.loadError}
			<div class="mt-6">
				<EmptyState
					icon={BookOpen}
					title={t('wiki.coming_soon', 'Wiki is not enabled for this destination')}
					body={t('wiki.coming_soon_body', 'Wiki is currently disabled. Please check back later.')}
					action={{ label: t('wiki.back_to_index', 'Back'), href: '/wiki' }}
				/>
			</div>
		{:else if data.loadError}
			<div class="mt-6 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-4 py-3 text-sm text-[var(--color-text-subtle)]">
				{t('wiki.load_error', 'Wiki could not be loaded.')}
			</div>
		{:else if cardGroups.length === 0}
			<div class="mt-6">
				<EmptyState
					icon={BookOpen}
					title={t('wiki.empty_title', 'No wiki pages yet')}
					body={t('wiki.empty_body', "This space doesn't have any wiki pages yet.")}
					action={canCreate ? { label: t('wiki.create_first_page', 'Create the first page'), href: `/wiki/${encodeURIComponent(data.spaceSlug)}/create` } : undefined}
				/>
			</div>
		{:else}
			{#if intro}
				<p class="mt-4 max-w-3xl text-sm leading-relaxed text-[var(--color-text)]">{intro}</p>
			{/if}

			<!-- Section cards — one per category; sibling pages as sub-links -->
			<div class="mt-5 divide-y divide-[var(--color-border)] rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)]">
				{#each cardGroups as group (group.category)}
					{@const Icon = categoryIconResolved(group.category, group.icon)}
					{@const tint = categoryTint(group.category)}
					{@const first = group.pages[0]}
					<!-- Directory cards link the stub (the child listing); others
					     prefer a materialized page, then the first lazy template. -->
					{@const firstSlug = group.directory && group.pageSlug
						? group.pageSlug
						: first?.slug ?? group.templates[0]?.slug ?? group.pageSlug ?? ''}
					<div>
						<a
							href="/wiki/{encodeURIComponent(data.spaceSlug)}/{encodeURIComponent(firstSlug)}"
							class="flex items-center gap-3.5 px-4 py-4 transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							<span
								class="grid h-10 w-10 shrink-0 place-items-center rounded-full"
								style="background: {tint.bg}; color: {tint.fg}"
							>
								<Icon size={18} aria-hidden="true" />
							</span>
							<span class="min-w-0 flex-1">
								<span class="flex items-center gap-2 text-sm font-semibold text-[var(--color-text)]">
									{group.label}
									{#if first?.is_featured}
										<Star size={12} class="shrink-0 text-[var(--color-warning)]" role="img" aria-label={t('wiki.featured', 'Featured')} />
									{/if}
									{#if first?.is_locked}
										<Lock size={12} class="shrink-0 text-[var(--color-muted)]" role="img" aria-label={t('wiki.locked', 'Locked')} />
									{/if}
									{#if structuredEditsOn && group.pages.some((p) => p.data_entity_type)}
										<span
											class="inline-flex items-center gap-1 rounded bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10px] font-medium uppercase text-[var(--color-muted)]"
											title={t('wiki.data_badge_hint', 'Includes community-editable data')}
										>
											<Table2 size={10} /> {t('wiki.data_badge', 'Data')}
										</span>
									{/if}
								</span>
								<span class="mt-0.5 line-clamp-2 block text-xs text-[var(--color-muted)]">
									{categoryDesc(group.category)}
								</span>
							</span>
							<ChevronRight size={18} class="shrink-0 text-[var(--color-muted)]" aria-hidden="true" />
						</a>
						{#if group.pages.length > 1 || group.templates.length > 0}
							<div class="flex flex-wrap items-center gap-1.5 border-t border-[var(--color-border)] px-4 py-2.5 pl-[4.25rem]">
								{#each group.pages.slice(1) as p (p.id)}
									<a
										href="/wiki/{encodeURIComponent(data.spaceSlug)}/{encodeURIComponent(p.slug)}"
										class="inline-flex items-center gap-1 rounded-full bg-[var(--color-surface-subtle)] px-2.5 py-1 text-xs text-[var(--color-text)] transition-colors hover:text-[var(--color-primary)]"
									>
										{p.title}
										{#if p.is_locked}
											<Lock size={10} class="shrink-0 text-[var(--color-muted)]" role="img" aria-label={t('wiki.locked', 'Locked')} />
										{/if}
									</a>
								{/each}
								{#each group.templates as tm (tm.slug)}
									<a
										href="/wiki/{encodeURIComponent(data.spaceSlug)}/{encodeURIComponent(tm.slug)}"
										class="inline-flex items-center gap-1 rounded-full border border-dashed border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-muted)] transition-colors hover:text-[var(--color-primary)]"
									>
										{tm.label}
									</a>
								{/each}
							</div>
						{/if}
					</div>
				{/each}
			</div>

			<!-- Community CTA — hidden on wiki-only spaces (attractions):
			     "Ask a Question" links /spaces/{slug} which redirects back
			     here. -->
			{#if !data.spaceDetail?.wiki_only}
			<div class="mt-6 flex flex-wrap items-center gap-3 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<span class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)]">
					<CircleHelp size={18} class="text-[var(--color-primary)]" aria-hidden="true" />
				</span>
				<span class="min-w-0 flex-1">
					<span class="block text-sm font-semibold text-[var(--color-text)]">
						{t('wiki.still_have_questions', 'Still have questions about {name}?', { name: spaceName })}
					</span>
					<span class="block text-xs text-[var(--color-muted)]">
						{t('wiki.community_help', 'Our community and expert advisors are here to help.')}
					</span>
				</span>
				<a
					href="/spaces/{encodeURIComponent(data.spaceSlug)}"
					class="inline-flex shrink-0 items-center rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					{t('wiki.ask_a_question', 'Ask a Question')}
				</a>
			</div>
			{/if}
		{/if}

		{#if lastUpdated}
			<p class="mt-4 text-xs text-[var(--color-muted)]">
				{t('wiki.last_updated', 'Last updated: {date}', { date: lastUpdated })}
			</p>
		{/if}

		{#if structuredEditsOn && spaceId && !data.loadError}
			<EntityFieldsPanel
				entityType="space"
				entityId={spaceId}
				title={t('wiki.space_data_title', '{name} data', { name: spaceName })}
			/>
		{/if}
	</div>
</div>
