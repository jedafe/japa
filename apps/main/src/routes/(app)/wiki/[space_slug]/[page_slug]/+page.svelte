<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md
	import { page } from '$app/state';
	import { beforeNavigate } from '$app/navigation';
	import { t, siteName} from '$lib/i18n';
	import { api } from '$lib/api/client';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import PostEditor from '$lib/components/create/editors/PostEditor.svelte';
	import SuggestEdit from '$lib/components/wiki/SuggestEdit.svelte';
	import WikiItemTables from '$lib/components/wiki/WikiItemTables.svelte';
	import WikiHelpfulWidget from '$lib/components/wiki/WikiHelpfulWidget.svelte';
	import WikiRollupPanel from '$lib/components/wiki/WikiRollupPanel.svelte';
	import WikiDirectoryPanel from '$lib/components/wiki/WikiDirectoryPanel.svelte';
	import InArticleAd from '$lib/components/ads/InArticleAd.svelte';
	import SetSidebar from '$lib/components/shell/SetSidebar.svelte';
	import WikiTabs from '$lib/components/wiki/WikiTabs.svelte';
	import WikiSidebar from '$lib/components/wiki/WikiSidebar.svelte';
	import { searchEntities } from '$lib/api/jce';
	import type { ResolvedItem } from '$lib/types/jce';
	import { BookOpen, Calculator, ChevronLeft, ChevronRight, Lock, Pencil, History, Send, Trash2 } from 'lucide-svelte';
	import type { WikiCategory, WikiPage } from '$lib/types';
	import type { PageData } from './$types';
	import { sanitizeContentHtml, safeJsonLdScript } from '$lib/utils/safeHtml';
	import { breadcrumbJsonLd } from '$lib/seo/breadcrumbs';
	import { tiptapToHtml, tiptapToText, type TipTapDoc } from '$lib/tiptap/serialize';

	let { data }: { data: PageData } = $props();

	const spaceName = $derived(
		data.page?.space_name ?? data.spaceDetail?.space?.name ?? data.spaceSlug
	);
	const spaceId = $derived(data.page?.space_id ?? data.spaceDetail?.space?.id ?? '');
	const spaceLevel = $derived(data.spaceDetail?.space?.level ?? '');
	const crumbKind = $derived(
		data.wikiScope === 'university'
			? t('wiki.breadcrumb_university_guides', 'University Guides')
			: spaceLevel === 'city'
				? t('wiki.breadcrumb_city_guides', 'City Guides')
				: spaceLevel === 'state'
					? t('wiki.breadcrumb_state_guides', 'State Guides')
					: t('wiki.breadcrumb_country_guides', 'Country Guides')
	);

	function categoryLabel(cat: WikiCategory): string {
		// §80 Wiki Types: prefer the admin-set template label when the API
		// supplies it; fall back to the i18n key for unmanaged categories.
		return data.page?.category_label || t(`wiki.category_${cat}`, cat.replace(/_/g, ' '));
	}

	// §80 Reader UX — estimated read time from the prose word count.
	const readMinutes = $derived.by(() => {
		const c = data.page?.current_content;
		if (c == null) return 0;
		const text = typeof c === 'string' ? c.replace(/<[^>]*>/g, ' ') : tiptapToText(c as TipTapDoc);
		const words = text.split(/\s+/).filter(Boolean).length;
		return words > 0 ? Math.max(1, Math.ceil(words / 200)) : 0;
	});

	// Render the wiki content to safe HTML. TipTap JSON is serialized to HTML;
	// legacy HTML strings are sanitized directly (Rule 16 / §5.1).
	const htmlContent = $derived.by(() => {
		const c = data.page?.current_content;
		if (c == null) return '';
		if (typeof c === 'string') return sanitizeContentHtml(c);
		if (typeof c === 'object') {
			return sanitizeContentHtml(tiptapToHtml(c as unknown as TipTapDoc));
		}
		return '';
	});

	// Edit form state.
	let editing = $state(false);
	let editDirty = $state(false);
	let editBodyHtml = $state('');
	let editBodyJson = $state<Record<string, unknown>>({ type: 'doc', content: [] });
	let summary = $state('');
	let loading = $state(false);
	let submitted = $state(false);
	let autoApproved = $state(false);
	let errorKey = $state('');

	// The tab strip and sibling links stay live while the revision form is
	// open — warn before a navigation silently drops an edited draft.
	beforeNavigate((nav) => {
		if (
			editing &&
			(editDirty || summary.trim()) &&
			!confirm(t('settings.unsaved_warning', 'You have unsaved changes. Leave anyway?'))
		) {
			nav.cancel();
		}
	});

	async function startEdit() {
		summary = '';
		submitted = false;
		autoApproved = false;
		errorKey = '';
		editDirty = false;
		editing = true;
		// Fetch the raw (unresolved) revision so authored {{...}} literals
		// survive a re-save — the display payload has them resolved.
		const res = await api.get<WikiPage>(
			`/wiki/${encodeURIComponent(data.spaceSlug)}/${encodeURIComponent(data.pageSlug)}?raw=1`
		);
		const c = res.data?.current_content ?? data.page?.current_content;
		if (!editing) return;
		if (typeof c === 'string') {
			editBodyHtml = c;
			editBodyJson = { type: 'doc', content: [] };
		} else if (c && typeof c === 'object') {
			editBodyHtml = tiptapToHtml(c as unknown as TipTapDoc);
			editBodyJson = c as Record<string, unknown>;
		} else {
			editBodyHtml = '';
			editBodyJson = { type: 'doc', content: [] };
		}
	}

	function cancelEdit() {
		editing = false;
	}

	async function submitRevision(e: SubmitEvent) {
		e.preventDefault();
		errorKey = '';
		if (!data.page) return;
		const plainText = tiptapToText(editBodyJson as unknown as TipTapDoc).trim();
		if (!plainText) {
			errorKey = 'wiki.content_required';
			return;
		}
		loading = true;
		try {
			const res = await api.post<{ id: string; status: string; auto_approved?: boolean }>(
				`/wiki/${encodeURIComponent(data.spaceSlug)}/${encodeURIComponent(data.pageSlug)}/revisions`,
				{ content: editBodyJson, summary: summary.trim() || null }
			);
			if (res.data) {
				submitted = true;
				autoApproved = res.data.auto_approved === true;
				editing = false;
			} else if (res.error) {
				// Surface the backend's structured error code (e.g. wiki.insufficient_reputation).
				errorKey = `wiki.${res.error.code}`;
			} else {
				errorKey = 'wiki.submit_failed';
			}
		} catch {
			errorKey = 'wiki.submit_failed';
		} finally {
			loading = false;
		}
	}

	const title = $derived(
		data.page ? `${data.page.title} — ${siteName()}` : t('wiki.page_title_suffix')
	);
	const description = $derived.by(() => {
		const c = data.page?.current_content;
		if (c == null) return t('wiki.page_description');
		// Legacy pages store raw HTML strings; current pages store TipTap JSON.
		const text = (typeof c === 'string'
			? c.replace(/<[^>]*>/g, ' ')
			: tiptapToText(c as unknown as TipTapDoc)
		)
			.replace(/\s+/g, ' ')
			.trim();
		if (!text) return t('wiki.page_description');
		return text.length > 160 ? text.slice(0, 157).trimEnd() + '…' : text;
	});
	const ogImage = $derived.by(() => {
		const u = data.spaceDetail?.header_url || data.spaceDetail?.logo_url || '';
		return u && !u.startsWith('http') ? page.url.origin + u : u;
	});
	const canonicalUrl = $derived(page.url.origin + page.url.pathname);
	// Article + BreadcrumbList JSON-LD (Section 26 — wiki pages are
	// hierarchical articles under their destination space index).
	const wikiArticleJsonLd = $derived(
		data.page
			? JSON.stringify({
					'@context': 'https://schema.org',
					'@type': 'Article',
					headline: data.page.title,
					description,
					url: canonicalUrl,
					about: { '@type': 'Place', name: spaceName }
				})
			: ''
	);
	const wikiBreadcrumbJsonLd = $derived(
		JSON.stringify(
			breadcrumbJsonLd([
				{ name: 'Home', path: page.url.origin },
				{ name: spaceName, path: `${page.url.origin}/wiki/${data.spaceSlug}` },
				{ name: data.page?.title ?? data.pageSlug, path: canonicalUrl }
			])
		)
	);
	// §80 Super-Admin Direct Publish — super_admin carries the `*` wildcard
	// node (the resolved list holds no per-node keys), bypasses lock state,
	// and publishes directly without review.
	const isSuperAdmin = $derived(data.userPermissions?.includes('*') === true);
	const canEdit = $derived(
		!!data.page &&
			data.authed &&
			(isSuperAdmin || (!data.page.is_locked && data.userPermissions?.includes('wiki.edit') === true))
	);
	const wikiEnabled = $derived(data.features?.['wiki.enabled'] !== false);
	const minReputation = $derived(Number(data.siteSettings?.['wiki.min_reputation_to_edit'] ?? 100));

	// §80 community field ops — the data panel + per-entity "suggest an edit"
	// affordances only render when the structured_edits flag is on.
	const structuredEditsOn = $derived(
		wikiEnabled && data.features?.['wiki.structured_edits'] === true
	);
	// The page's structured-data target is its host space (entity_type 'space').
	const panelSpaceId = $derived(data.page?.space_id ?? data.spaceDetail?.space?.id ?? '');
	// When the page is bound to a child entity class (data_entity_type, e.g.
	// 'visa_route'), list that space's entities with per-row suggest buttons.
	const dataEntityType = $derived(data.page?.data_entity_type ?? '');

	let childEntities = $state<ResolvedItem[]>([]);
	let childEntitiesLoading = $state(false);
	// Per-row "suggest an edit" — one keyed dialog instance, retargeted to
	// the clicked entity (keyed so a new target refetches its schema).
	let suggestTarget = $state<ResolvedItem | null>(null);
	let suggestOpen = $state(false);

	// §80 Hierarchical UX — Prev/Next derives from the page's ordered
	// sibling set (position-ordered, current flagged by the backend).
	const siblings = $derived(data.page?.siblings ?? []);
	const prevSibling = $derived.by(() => {
		const i = siblings.findIndex((s) => s.current);
		return i > 0 ? siblings[i - 1] : null;
	});
	const nextSibling = $derived.by(() => {
		const i = siblings.findIndex((s) => s.current);
		return i >= 0 && i < siblings.length - 1 ? siblings[i + 1] : null;
	});
	// Sibling pages beyond Prev/Next (e.g. "Prices in Toronto" from the
	// Cost of Living page) — the "View more …" links in the mockups.
	const otherSiblings = $derived(siblings.filter((s) => !s.current));

	// §80 Wiki Topic Pages — "More in this section" nav: same-parentage
	// siblings (already including lazy templates) plus the category's other
	// declared templates. Child topic templates nest under their parent's
	// chip; children of the current page — or whose parent isn't shown —
	// render as flat dashed chips.
	const moreNav = $derived.by(() => {
		const templates = data.page?.page_templates ?? [];
		const items: {
			slug: string;
			label: string;
			dashed: boolean;
			children: typeof templates;
		}[] = [];
		const shownParentIdx = new Map<string, number>();
		for (const sib of otherSiblings) {
			const tmpl = templates.find((tm) => tm.slug === sib.slug);
			if (tmpl?.template_id) shownParentIdx.set(tmpl.template_id, items.length);
			items.push({ slug: sib.slug, label: sib.title, dashed: false, children: [] });
		}
		const shownSlugs = new Set(items.map((i) => i.slug));
		for (const tm of templates) {
			if (tm.slug === data.pageSlug || shownSlugs.has(tm.slug)) continue;
			const pIdx = tm.parent_template_id ? shownParentIdx.get(tm.parent_template_id) : undefined;
			if (pIdx !== undefined) {
				items[pIdx].children.push(tm);
				continue;
			}
			items.push({ slug: tm.slug, label: tm.label, dashed: true, children: [] });
		}
		return items;
	});

	// §80 Wiki Topic Pages — the page-level ancestor chain (category stub →
	// template parents) for the breadcrumb's in-page segments and the
	// "Back to {parent}" link.
	const pageParents = $derived(data.page?.parents ?? []);
	const parentPage = $derived(
		pageParents.length > 0 ? pageParents[pageParents.length - 1] : null
	);

	// §80 Wiki Topic Pages — ON THIS PAGE anchors: the rendered field-table
	// sections (named, non-quick_facts, non-empty or composite) plus the
	// rollup/directory panels. Shown when the page has ≥2 anchors.
	const toc = $derived.by(() => {
		const items: { id: string; label: string }[] = [];
		for (const s of data.page?.fields?.sections ?? []) {
			if (!s.key || s.key === 'quick_facts') continue;
			if (s.fields.filter((f) => f.visible).length === 0 && !s.page_slug) continue;
			const label =
				s.label ||
				t(
					`wiki.section_${s.key}`,
					s.key.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())
				);
			items.push({ id: `section-${s.key}`, label });
		}
		if (data.page?.rollup) items.push({ id: 'wiki-rollup', label: t('wiki.rollup_title', 'City-by-city comparison') });
		if (data.page?.directory) items.push({ id: 'wiki-directory', label: t('wiki.directory_title', 'Places') });
		return items.length >= 2 ? items : [];
	});

	$effect(() => {
		const type = dataEntityType;
		const spaceId = panelSpaceId;
		if (!structuredEditsOn || !type || !spaceId) {
			childEntities = [];
			return;
		}
		childEntitiesLoading = true;
		searchEntities(type, '', { geo_space_id: spaceId }, { pageSize: 50 })
			.then((items) => (childEntities = items))
			.catch(() => (childEntities = []))
			.finally(() => (childEntitiesLoading = false));
	});

	// This component instance is reused across /wiki/{space}/{page}
	// navigations — clear the edit/submission state when a different page
	// loads so a stale draft can't be submitted to the new page.
	let lastRouteKey = $state('');
	$effect(() => {
		const key = `${data.spaceSlug}/${data.pageSlug}`;
		if (lastRouteKey && lastRouteKey !== key) {
			editing = false;
			editDirty = false;
			submitted = false;
			autoApproved = false;
			errorKey = '';
			summary = '';
			suggestTarget = null;
			suggestOpen = false;
		}
		lastRouteKey = key;
	});
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="article" />
	<meta property="og:url" content={canonicalUrl} />
	{#if ogImage}
		<meta property="og:image" content={ogImage} />
		<meta name="twitter:image" content={ogImage} />
		<meta name="twitter:card" content="summary_large_image" />
	{/if}
	<link rel="canonical" href={canonicalUrl} />
	{#if wikiArticleJsonLd}
		{@html safeJsonLdScript(wikiArticleJsonLd)}
	{/if}
	{@html safeJsonLdScript(wikiBreadcrumbJsonLd)}
</svelte:head>

<SetSidebar>
	<WikiSidebar
		spaceSlug={data.spaceSlug}
		{spaceName}
		{spaceId}
		{spaceLevel}
		pages={data.index ?? []}
		currentPageSlug={data.pageSlug}
		{toc}
		wikiOnly={data.spaceDetail?.wiki_only ?? false}
	/>
</SetSidebar>

<div class="mx-auto w-full max-w-full">
	<!-- Breadcrumb: Wiki / {ancestors…} / {space} / {page} — the chain
	     resolves from actual space links arbitrated by the declared type
	     edges (§80 Hierarchical UX); falls back to the static kind label
	     when no ancestors resolve. -->
	<div class="border-b border-[var(--color-border)] px-4 py-2.5 sm:px-6" style="margin-inline: max(1rem, calc((100% - 48rem)/2))">
		<nav aria-label={t('a11y.breadcrumb', 'Breadcrumb')} class="min-w-0 text-xs text-[var(--color-muted)]">
			<ol class="flex items-center gap-1.5">
				<li>
					<a href="/wiki" class="transition-colors hover:text-[var(--color-text)]">
						{t('wiki.heading', 'Wiki')}
					</a>
				</li>
				{#if data.page?.ancestors?.length}
					{#each data.page.ancestors as anc (anc.slug)}
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
				<li>
					<a
						href="/wiki/{encodeURIComponent(data.spaceSlug)}"
						class="truncate transition-colors hover:text-[var(--color-text)]"
					>
						{spaceName}
					</a>
				</li>
				<!-- §80 Wiki Topic Pages — in-page ancestors: category stub +
				     template chain, each a real /wiki/{space}/{slug} target. -->
				{#each pageParents as par (par.slug)}
					<li aria-hidden="true"><ChevronRight size={12} /></li>
					<li>
						<a
							href="/wiki/{encodeURIComponent(data.spaceSlug)}/{encodeURIComponent(par.slug)}"
							class="truncate transition-colors hover:text-[var(--color-text)]"
						>
							{par.title}
						</a>
					</li>
				{/each}
				{#if data.page}
					<li aria-hidden="true"><ChevronRight size={12} /></li>
					<li class="truncate font-medium text-[var(--color-text)]">{data.page.title}</li>
				{/if}
			</ol>
		</nav>
	</div>

	<!-- Guide tab strip — active tab = this page's category -->
	<div class="mx-auto w-full max-w-3xl px-1 sm:px-3">
		<WikiTabs
			spaceSlug={data.spaceSlug}
			categories={data.apiCategories ?? []}
			pages={data.index ?? []}
			active={data.page?.category ?? ''}
			currentPageSlug={data.pageSlug}
		/>
	</div>

	<div class="p-4">
		{#if !wikiEnabled && (data.loadError || !data.page)}
			<EmptyState
				icon={BookOpen}
				title={t('wiki.coming_soon', 'Wiki is not enabled for this destination')}
				body={t('wiki.coming_soon_body', 'Wiki is currently disabled. Please check back later.')}
				action={{ label: t('wiki.back_to_index', 'Back to wiki'), href: `/wiki/${data.spaceSlug}` }}
			/>
		{:else if data.removed}
			<EmptyState
				icon={Trash2}
				title={t('wiki.page_removed', 'This wiki page was removed')}
				body={t('wiki.page_removed_body', 'This page was removed by moderators. Its history is preserved for transparency.')}
				action={{ label: t('wiki.back_to_index', 'Back to wiki'), href: `/wiki/${data.spaceSlug}` }}
			/>
		{:else if data.loadError || !data.page}
			<EmptyState
				icon={BookOpen}
				title={t('wiki.page_not_found', 'Wiki page not found')}
				body={t('wiki.page_not_found_body', "The wiki page you're looking for doesn't exist or has been removed.")}
				action={{ label: t('wiki.back_to_index', 'Back to wiki'), href: `/wiki/${data.spaceSlug}` }}
			/>
		{:else if submitted}
			<div class="rounded-[12px] border border-green-500/30 bg-green-500/10 p-6 text-center">
				<p class="text-lg font-medium text-green-600">
					{autoApproved
						? t('wiki.suggest_approved', 'Your changes were applied.')
						: t('wiki.revision_submitted', 'Your revision has been submitted for review.')}
				</p>
				<div class="mt-4 flex justify-center gap-3">
					<a
						href="/wiki/{data.spaceSlug}/{data.pageSlug}"
						class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
					>
						{t('wiki.back_to_index', 'Back to page')}
					</a>
					<button
						type="button"
						onclick={() => (submitted = false)}
						class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
					>
						{t('wiki.edit', 'Edit again')}
					</button>
				</div>
			</div>
		{:else}
			<article class="mx-auto w-full max-w-3xl">
				<!-- §80 Wiki Topic Pages — "Back to {parent}" above the article
				     when the page has a parent segment. -->
				{#if parentPage}
					<div class="mb-3">
						<a
							href="/wiki/{encodeURIComponent(data.spaceSlug)}/{encodeURIComponent(parentPage.slug)}"
							class="inline-flex items-center gap-1 text-xs font-medium text-[var(--color-muted)] transition-colors hover:text-[var(--color-primary)]"
						>
							<ChevronLeft size={13} class="shrink-0" />
							{t('wiki.back_to_parent', 'Back to {title}', { title: parentPage.title })}
						</a>
					</div>
				{/if}
				<header class="mb-4 flex flex-wrap items-center gap-2">
					<h1 class="text-2xl font-semibold text-[var(--color-text)]">{data.page.title}</h1>
					<span class="rounded-full border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-2.5 py-0.5 text-xs font-medium text-[var(--color-text-subtle)]">
						{categoryLabel(data.page.category as WikiCategory)}
					</span>
					{#if data.page.is_locked}
						<span class="inline-flex items-center gap-1 rounded-full bg-amber-500/10 px-2.5 py-0.5 text-xs font-medium text-amber-600">
							<Lock size={12} /> {t('wiki.locked', 'Locked')}
						</span>
					{/if}
				</header>

				<!-- §80 Reader UX byline: approved submitter + avatar, last-updated
				     date, and a prose-derived read time. -->
				{#if data.page.submitted_by || data.page.updated_at || readMinutes > 0}
					<div class="mb-4 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-[var(--color-muted)]">
						{#if data.page.submitted_by_avatar_url}
							<img
								src={data.page.submitted_by_avatar_url}
								alt=""
								class="h-5 w-5 rounded-full object-cover"
								loading="lazy"
							/>
						{/if}
						{#if data.page.submitted_by}
							<span>
								{t('wiki.byline', 'By {name}').replace('{name}', data.page.submitted_by)}
							</span>
						{/if}
						{#if data.page.updated_at}
							<span aria-hidden="true">·</span>
							<span>
								{t('wiki.updated_on', 'Updated {date}').replace(
									'{date}',
									new Date(data.page.updated_at).toLocaleDateString(undefined, {
										year: 'numeric',
										month: 'short',
										day: 'numeric'
									})
								)}
							</span>
						{/if}
						{#if readMinutes > 0}
							<span aria-hidden="true">·</span>
							<span>{t('wiki.read_time', '{min} min read').replace('{min}', String(readMinutes))}</span>
						{/if}
					</div>
				{/if}

				<div class="mb-6 flex flex-wrap items-center gap-3 text-sm">
					{#if canEdit}
						<button
							type="button"
							onclick={startEdit}
							class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
						>
							<Pencil size={14} /> {t('wiki.edit', 'Edit')}
						</button>
					{:else if data.page.is_locked}
						<span class="text-xs text-[var(--color-muted)]">{t('wiki.locked_body', 'This page is locked and cannot be edited.')}</span>
					{/if}
					<a
						href="/wiki/{data.spaceSlug}/{data.pageSlug}/history"
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						<History size={14} /> {t('wiki.history', 'History')}
					</a>
					<!-- §72 ↔ §80 cross-link: city cost-of-living wiki pages feed the
					     calculator via source_field_id bindings; surface the tool. -->
					{#if data.page.category === 'cost_of_living' && spaceLevel === 'city'}
						<a
							href="/tools/cost-of-living"
							class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							<Calculator size={14} /> {t('wiki.open_calculator', 'Compare in calculator')}
						</a>
					{/if}
				</div>

				{#if editing}
					<form onsubmit={submitRevision} class="space-y-4 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-5" novalidate>
						<div class="flex flex-col gap-1.5">
							<span class="text-sm font-medium text-[var(--color-text)]">
								{t('wiki.content_label', 'Content')}
							</span>
							<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] focus-within:border-[var(--color-primary)]">
								<PostEditor
									html={editBodyHtml}
									onedit={() => (editDirty = true)}
									onhtml={(v) => (editBodyHtml = v)}
									onjson={(v) => (editBodyJson = v)}
								/>
							</div>
						</div>
						<div class="flex flex-col gap-1.5">
							<label for="wiki-summary" class="text-sm font-medium text-[var(--color-text)]">
								{t('wiki.edit_summary_label', 'Edit summary')}
							</label>
							<input
								id="wiki-summary"
								type="text"
								value={summary}
								placeholder={t('wiki.edit_summary_placeholder', 'Briefly describe your changes')}
								oninput={(e) => (summary = e.currentTarget.value)}
								class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
							/>
						</div>

						<p class="text-xs text-[var(--color-muted)]">
							{t('wiki.reputation_required', 'Minimum reputation required to edit: {min}')
								.replace('{min}', String(minReputation))}
						</p>

						{#if errorKey}
							<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
								{t(errorKey, t('wiki.submit_failed', 'Failed to submit revision. Please try again.'))}
							</p>
						{/if}

						<div class="flex justify-end gap-2 pt-1">
							<button
								type="button"
								onclick={cancelEdit}
								class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
							>
								{t('wiki.cancel', 'Cancel')}
							</button>
							<button
								type="submit"
								disabled={loading}
								class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition hover:opacity-90 disabled:opacity-50"
							>
								<Send size={14} />
								{loading ? '...' : t('wiki.submit_revision', 'Submit revision')}
							</button>
						</div>
					</form>
				{:else}
					{#if htmlContent}
						<div class="prose max-w-none text-sm leading-relaxed text-[var(--color-text)]">
							<InArticleAd html={htmlContent} {spaceId} />
						</div>
					{:else if !data.page.directory}
						<!-- Directory pages have no prose by design — the child
						     listing below IS the content; the empty-state box
						     would read as a missing page. -->
						<div class="rounded-[12px] border border-dashed border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-6 text-center">
							<p class="text-sm text-[var(--color-muted)]">
								{t('wiki.empty_content', 'This page has no content yet.')}
							</p>
							{#if canEdit}
								<button
									type="button"
									onclick={startEdit}
									class="mt-3 inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition hover:opacity-90"
								>
									<Pencil size={14} /> {t('wiki.contribute_cta', 'Be the first to contribute')}
								</button>
							{/if}
						</div>
					{/if}

					<!-- §80 Hierarchical UX — the read-facing structured render: the
					     descendant rollup (country/state category pages), the
					     child-space directory (directory-mode categories), and the
					     bound group's itemized sections (city prices pages). -->
					{#if data.page.rollup}
						<WikiRollupPanel
							rollup={data.page.rollup}
							spaceName={spaceName}
							contextCurrency={data.page.space_currency_code ?? ''}
							contextFx={data.page.space_currency_fx}
						/>
					{/if}
					{#if data.page.directory}
						<WikiDirectoryPanel directory={data.page.directory} spaceSlug={data.spaceSlug} />
					{/if}
					{#if data.page.fields}
						<div class="mt-6">
							<WikiItemTables
								group={data.page.fields}
								contextCurrency={data.page.space_currency_code ?? ''}
								contextFx={data.page.space_currency_fx}
								activeSection={data.section || undefined}
								allHref="/wiki/{encodeURIComponent(data.spaceSlug)}/{encodeURIComponent(
									data.pageSlug
								)}"
								moreHref={(sec) =>
									sec.page_slug
										? `/wiki/${encodeURIComponent(data.spaceSlug)}/${encodeURIComponent(
												sec.page_slug
											)}`
										: `/wiki/${encodeURIComponent(data.spaceSlug)}/${encodeURIComponent(
												data.pageSlug
											)}?section=${encodeURIComponent(sec.key)}`}
							/>
						</div>
					{/if}

					{#if moreNav.length > 0}
						<nav class="mt-6 flex flex-wrap items-center gap-1.5" aria-label={t('wiki.more_pages', 'More in this section')}>
							{#each moreNav as item (item.slug)}
								<a
									href="/wiki/{encodeURIComponent(data.spaceSlug)}/{encodeURIComponent(item.slug)}"
									class={item.dashed
										? 'inline-flex items-center rounded-full border border-dashed border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-muted)] transition-colors hover:text-[var(--color-primary)]'
										: 'inline-flex items-center rounded-full bg-[var(--color-surface-subtle)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:text-[var(--color-primary)]'}
								>
									{item.label}
								</a>
								{#if item.children.length > 0}
									<!-- §80 Wiki Topic Pages — child topic templates nest
									     under their parent's chip. -->
									<span class="inline-flex flex-wrap items-center gap-1.5 border-l border-[var(--color-border)] pl-1.5">
										{#each item.children as child (child.slug)}
											<a
												href="/wiki/{encodeURIComponent(data.spaceSlug)}/{encodeURIComponent(child.slug)}"
												class="inline-flex items-center rounded-full border border-dashed border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-muted)] transition-colors hover:text-[var(--color-primary)]"
											>
												{child.label}
											</a>
										{/each}
									</span>
								{/if}
							{/each}
						</nav>
					{/if}

					<!-- §80 Reader UX + Super-Admin Direct Publish — the bottom
					     Details panel is retired: the bound group's item tables
					     above render the same fields/values, so the panel only
					     duplicated them. The Suggest-an-edit trigger sits inside the
					     "Was this page helpful?" card's trailing slot; on lazy pages
					     (no wiki_pages row to vote on) the card is absent and the
					     affordance renders standalone. -->
					{#if data.page.helpful !== undefined || (structuredEditsOn && panelSpaceId)}
						<div class="mt-8">
							{#if data.page.helpful !== undefined}
								<WikiHelpfulWidget
									spaceSlug={data.spaceSlug}
									pageSlug={data.pageSlug}
									helpful={data.page.helpful}
									authed={data.authed}
								>
									{#if structuredEditsOn && panelSpaceId}
										<SuggestEdit
											entityType="space"
											entityId={panelSpaceId}
											groupId={data.page?.field_group_id ?? undefined}
											proposeGroupSlug={data.page?.field_group_slug ?? undefined}
										/>
									{/if}
								</WikiHelpfulWidget>
							{:else if structuredEditsOn && panelSpaceId}
								<SuggestEdit
									entityType="space"
									entityId={panelSpaceId}
									groupId={data.page?.field_group_id ?? undefined}
									proposeGroupSlug={data.page?.field_group_slug ?? undefined}
								/>
							{/if}
						</div>
					{/if}

					{#if prevSibling || nextSibling}
						<nav
							class="mt-8 flex items-stretch justify-between gap-3 border-t border-[var(--color-border)] pt-4"
							aria-label={t('wiki.page_nav', 'Page navigation')}
						>
							{#if prevSibling}
								<a
									href="/wiki/{encodeURIComponent(data.spaceSlug)}/{encodeURIComponent(prevSibling.slug)}"
									class="inline-flex min-w-0 items-center gap-1.5 text-sm text-[var(--color-muted)] transition-colors hover:text-[var(--color-primary)]"
								>
									<ChevronLeft size={14} class="shrink-0" />
									<span class="truncate">{prevSibling.title}</span>
								</a>
							{:else}
								<span></span>
							{/if}
							{#if nextSibling}
								<a
									href="/wiki/{encodeURIComponent(data.spaceSlug)}/{encodeURIComponent(nextSibling.slug)}"
									class="inline-flex min-w-0 items-center justify-end gap-1.5 text-right text-sm text-[var(--color-muted)] transition-colors hover:text-[var(--color-primary)]"
								>
									<span class="truncate">{nextSibling.title}</span>
									<ChevronRight size={14} class="shrink-0" />
								</a>
							{:else}
								<span></span>
							{/if}
						</nav>
					{/if}

					{#if structuredEditsOn && dataEntityType}
						<section
							class="mt-6 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
						>
							<h2 class="mb-3 text-sm font-semibold text-[var(--color-text)]">
								{t('wiki.data_entities_title', 'Listed entries')}
							</h2>
							{#if childEntitiesLoading}
								<p class="text-sm text-[var(--color-muted)]">
									{t('common.loading', 'Loading')}…
								</p>
							{:else if childEntities.length === 0}
								<p class="text-sm text-[var(--color-muted)]">
									{t('wiki.data_entities_empty', 'No entries yet.')}
								</p>
							{:else}
								<ul class="divide-y divide-[var(--color-border)]">
									{#each childEntities as item (item.id)}
										<li class="flex items-center justify-between gap-3 py-2 text-sm text-[var(--color-text)]">
											<span class="min-w-0 truncate">{item.title}</span>
											<button
												type="button"
												onclick={() => {
													suggestTarget = item;
													suggestOpen = true;
												}}
												class="shrink-0 text-[var(--color-muted)] transition-colors hover:text-[var(--color-primary)]"
												aria-label={t('wiki.suggest_edit', 'Suggest an edit')}
												title={t('wiki.suggest_edit', 'Suggest an edit')}
											>
												<Pencil size={13} />
											</button>
										</li>
									{/each}
								</ul>
							{/if}
							{#if suggestTarget}
								{#key suggestTarget.id}
									<SuggestEdit
										entityType={dataEntityType}
										entityId={suggestTarget.id}
										label={suggestTarget.title}
										proposeGroupSlug={data.page?.field_group_slug ?? undefined}
										showTrigger={false}
										bind:open={suggestOpen}
									/>
								{/key}
							{/if}
						</section>
					{/if}
				{/if}
			</article>
		{/if}
	</div>
</div>
