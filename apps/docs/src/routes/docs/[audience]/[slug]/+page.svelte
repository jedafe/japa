<script lang="ts">
	import { untrack } from 'svelte';
	import { onMount } from 'svelte';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import TableOfContents from '$lib/components/shell/TableOfContents.svelte';
	import DocContent from '$lib/components/docs/DocContent.svelte';
	import DocEditForm from '$lib/components/docs/DocEditForm.svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { persistNavCollapse } from '$lib/nav-collapse';
	import {
		ArrowLeft,
		ArrowRight,
		CalendarClock,
		ChevronRight,
		Clock,
		Eye,
		ListTree,
		Pencil
	} from 'lucide-svelte';
	import SeoHead from '$lib/seo/SeoHead.svelte';
	import { parseManagedCategories } from '$lib/docs/catalog';
	import type { PageData, ActionData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();

	let collapsed = $state(untrack(() => data.navCollapsed));

	async function toggleCollapse() {
		collapsed = !collapsed;
		await persistNavCollapse(collapsed);
	}

	// Inline editing (§44 in-app authoring): docs.manage only, ?edit=1 in the
	// URL so edit mode is deep-linkable and Cancel is just a link back.
	let canEdit = $derived(data.canEditDocs === true);
	let editing = $derived(canEdit && page.url.searchParams.get('edit') === '1');
	const viewPath = $derived(`/docs/${data.audience}/${data.page.slug}`);

	// One-shot notices (?created=…/?saved=…) — show once, then strip the query
	// param so a refresh doesn't resurrect a stale banner.
	let notice = $state<string | null>(null);
	onMount(() => {
		const status = page.url.searchParams.get('saved') ?? page.url.searchParams.get('created');
		if (status) {
			notice = status;
			const url = new URL(page.url);
			url.searchParams.delete('saved');
			url.searchParams.delete('created');
			history.replaceState(history.state, '', url);
		}
	});

	// The backend DocPage response has no breadcrumb/prev/next fields — derive
	// them from the nav tree (GET /docs/nav), which carries parent_id nesting
	// and position ordering for the current audience (Rule 16).
	const audienceTree = $derived(data.nav.find((tree) => tree.audience === data.audience));
	const flatNav = $derived(
		(audienceTree?.items ?? []).flatMap((item) => [item, ...(item.children ?? [])])
	);

	// Category/subcategory suggestions — case-folded so "Guides"/"guides"
	// don't surface as two near-duplicate groups (§44 amendment 2). The
	// admin-managed catalog (docs.categories) merges in so curated names
	// are suggested even before any page uses them.
	const managedCatalog = $derived(
		parseManagedCategories(page.data?.siteSettings?.['docs.categories'])
	);
	const existingCategories = $derived(
		[
			...new Map(
				[
					...managedCatalog.map((c) => c.name),
					...data.existingPages.map((p) => p.category?.trim())
				]
					.filter((c): c is string => !!c)
					.map((c) => [c.toLowerCase(), c])
			).values()
		]
	);
	const existingSubcategories = $derived(
		[
			...new Map(
				[
					...managedCatalog.flatMap((c) => c.subcategories),
					...data.existingPages.map((p) => p.subcategory?.trim())
				]
					.filter((c): c is string => !!c)
					.map((c) => [c.toLowerCase(), c])
			).values()
		]
	);


	const navIndex = $derived(flatNav.findIndex((i) => i.slug === data.page.slug));
	const breadcrumb = $derived.by(() => {
		if (data.page.breadcrumb?.length) return data.page.breadcrumb;
		const self = navIndex >= 0 ? flatNav[navIndex] : null;
		const parent = self?.parent_id
			? (audienceTree?.items ?? []).find((i) => i.id === self.parent_id)
			: null;
		return parent ? [{ title: parent.title, slug: parent.slug, audience: data.audience }] : [];
	});
	const prev = $derived(data.page.prev ?? (navIndex > 0 ? flatNav[navIndex - 1] : null));
	const next = $derived(
		data.page.next ?? (navIndex >= 0 && navIndex < flatNav.length - 1 ? flatNav[navIndex + 1] : null)
	);

	// Reading-time estimate from the rendered HTML (~200 wpm, min 1) —
	// standard doc-page meta next to the updated date.
	const readMinutes = $derived.by(() => {
		const words = data.html.replace(/<[^>]+>/g, ' ').trim().split(/\s+/).filter(Boolean).length;
		return Math.max(1, Math.round(words / 200));
	});
	const updatedLabel = $derived(
		t('page.updated', 'Updated {date}', {
			date: new Date(data.page.updated_at).toLocaleDateString(data.locale, {
				year: 'numeric',
				month: 'short',
				day: 'numeric'
			})
		})
	);

	const noticeText = $derived.by(() => {
		if (!notice) return null;
		switch (notice) {
			case 'published':
				return t('notice.saved_published', 'Published — this page is now live.');
			case 'approved':
				return t('notice.saved_approved', 'Approved — ready to publish.');
			case 'in_review':
				return t('notice.saved_in_review', 'Submitted for review. Only editors can see this page.');
			case 'draft':
				return t('notice.saved_draft', 'Saved as draft. Only editors can see this page.');
			default:
				return t('notice.saved', 'Changes saved.');
		}
	});
</script>

<svelte:head>
	<SeoHead meta={data.meta} gated={data.isGated} />
</svelte:head>

<AppShell
	nav={data.nav}
	accessibleAudiences={data.accessibleAudiences}
	logoUrl={(data.siteSettings?.["appearance.logo_url"] as string | undefined) ?? ""}
	footerMenu={data.footerMenu}
	rightSidebarWidgets={data.rightSidebarWidgets}
	leftSidebarWidgets={data.leftSidebarWidgets}
	activeAudience={data.audience}
	user={data.user}
	collapsed={collapsed}
	onToggleCollapse={toggleCollapse}
>
	{#if !editing && data.toc.length > 0}
		{#snippet sidebar()}
			<TableOfContents headings={data.toc} />
		{/snippet}
	{/if}

	<article class="doc-article">
		<header class="doc-header">
			<!-- Breadcrumb (Ref: §44 ROADMAP 02.03) — Docs / Audience / parents -->
			<nav class="breadcrumb" aria-label={t('a11y.breadcrumb', 'Breadcrumb')}>
				<a href="/docs" class="crumb">{t('audience.overview')}</a>
				<ChevronRight size={12} class="crumb-sep rtl:-scale-x-100" />
				<a href="/docs/{data.audience}" class="crumb">
					{t(data.audienceLabelKey, data.audience)}
				</a>
				{#each breadcrumb as crumb (crumb.slug)}
					<ChevronRight size={12} class="crumb-sep rtl:-scale-x-100" />
					<a href="/docs/{data.audience}/{crumb.slug}" class="crumb">{crumb.title}</a>
				{/each}
			</nav>
			<h1>{data.page.title}</h1>
			<div class="doc-meta">
				<span class="meta-item"><CalendarClock size={13} />{updatedLabel}</span>
				<span class="meta-item"><Clock size={13} />{t('page.read_time', '{min} min read', { min: String(readMinutes) })}</span>
				{#if data.page.review_status !== 'published'}
					<span class="status-badge status-{data.page.review_status}">
						{t(`new_page.status.${data.page.review_status}`, data.page.review_status)}
					</span>
				{/if}
				{#if data.page.ai_generated}
					<span class="ai-flag">{t('page.ai_assisted_draft')}</span>
				{/if}
				{#if canEdit}
					<a href="?edit=1" class="meta-edit-link">
						<Pencil size={13} />
						{t('page.edit_this_page', 'Edit this page')}
					</a>
				{/if}
			</div>
			{#if data.preview}
				<div class="preview-banner" role="status">
					<Eye size={15} />
					<span>
						{t(
							'page.preview_banner',
							'Preview — this page is {status} and only visible to editors.',
							{ status: t(`new_page.status.${data.page.review_status}`, data.page.review_status) }
						)}
					</span>
				</div>
			{/if}
			{#if noticeText}
				<div class="save-notice" role="status">{noticeText}</div>
			{/if}
			{#if data.locale !== 'en' && data.page.content_lang === 'en'}
				<div class="translation-notice">
					{t('page.not_translated', 'This page is not yet translated. Showing the English version.')}
					{#if editing}
						<strong>{t('page.edit_source_warning', 'Editing changes the English source content.')}</strong>
					{/if}
				</div>
			{/if}
		</header>

		{#if editing}
			<!-- Inline edit mode (§44 in-app authoring — docs.manage). -->
			<DocEditForm
				page={data.page}
				statuses={data.statuses}
				existingPages={data.existingPages}
				editSeed={data.editSeed}
				categories={existingCategories}
				subcategories={existingSubcategories}
				managed={managedCatalog}
				{form}
				{viewPath}
			/>
		{:else}
			<!-- Mobile TOC — the right sidebar hides below 1280px, so the in-page
			     anchors get a collapsible panel at the top of the article. -->
			{#if data.toc.length > 0}
				<details class="mobile-toc">
					<summary><ListTree size={15} />{t('page.table_of_contents')}</summary>
					<ul>
						{#each data.toc as h (h.id)}
							<li class:level-3={h.level === 3}>
								<a href="#{h.id}">{h.text}</a>
							</li>
						{/each}
					</ul>
				</details>
			{/if}

			<div class="prose">
				<DocContent
					pageId={data.page.id}
					audience={data.audience}
					slug={data.page.slug}
					html={data.html}
					lastReviewedAt={data.page.last_reviewed_at}
					lastReviewedBy={data.page.last_reviewed_by}
					published={data.page.review_status === 'published'}
					feedback={data.feedback}
				/>
			</div>

			<!-- Prev/Next navigation (Ref: §44 ROADMAP 02.05) -->
			{#if prev || next}
				<nav class="prev-next" aria-label={t('a11y.page_nav', 'Page navigation')}>
					{#if prev}
						<a href="/docs/{data.audience}/{prev.slug}" class="pn-card pn-prev">
							<span class="pn-label"><ArrowLeft class="rtl:-scale-x-100" size={14} /> {t('page.previous', 'Previous')}</span>
							<span class="pn-title">{prev.title}</span>
						</a>
					{:else}
						<span class="pn-card pn-placeholder"></span>
					{/if}
					{#if next}
						<a href="/docs/{data.audience}/{next.slug}" class="pn-card pn-next">
							<span class="pn-label">{t('page.next', 'Next')} <ArrowRight class="rtl:-scale-x-100" size={14} /></span>
							<span class="pn-title">{next.title}</span>
						</a>
					{:else}
						<span class="pn-card pn-placeholder"></span>
					{/if}
				</nav>
			{/if}
		{/if}
	</article>
</AppShell>

<style>
	.doc-article {
		max-width: 820px;
		margin: 0 auto;
		padding: 2.5rem 1.5rem 4rem;
	}
	.doc-header {
		margin-bottom: 2rem;
		padding-bottom: 1.5rem;
		border-bottom: 1px solid var(--color-border);
	}
	.doc-header h1 {
		font-family: var(--font-heading, var(--font-sans));
		font-size: 2rem;
		font-weight: 700;
		color: var(--color-text);
		margin: 0.5rem 0 0;
		letter-spacing: -0.02em;
		line-height: 1.2;
	}
	.doc-meta {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 0.5rem 1rem;
		margin-top: 0.875rem;
	}
	.meta-item {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		font-size: 0.8125rem;
		color: var(--color-muted);
	}
	.meta-edit-link {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		margin-inline-start: auto;
		font-size: 0.8125rem;
		font-weight: 500;
		color: var(--color-primary);
		text-decoration: none;
		padding: 0.25rem 0.625rem;
		border-radius: var(--radius);
		border: 1px solid color-mix(in srgb, var(--color-primary) 35%, transparent);
	}
	.meta-edit-link:hover {
		background: color-mix(in srgb, var(--color-primary) 8%, transparent);
	}
	.status-badge {
		display: inline-flex;
		align-items: center;
		font-size: 0.6875rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		font-weight: 600;
		padding: 0.125rem 0.5rem;
		border-radius: 4px;
		background: var(--color-surface-subtle);
		color: var(--color-text-subtle);
		border: 1px solid var(--color-border);
	}
	.status-badge.status-approved {
		color: var(--method-patch);
		background: var(--method-patch-bg);
		border-color: transparent;
	}
	.ai-flag {
		display: inline-flex;
		align-items: center;
		font-size: 0.6875rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--method-patch);
		background: var(--method-patch-bg);
		padding: 0.125rem 0.5rem;
		border-radius: 4px;
		font-weight: 600;
	}
	.preview-banner {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		margin-top: 0.875rem;
		padding: 0.625rem 0.875rem;
		border: 1px solid color-mix(in srgb, var(--method-patch) 45%, var(--color-border));
		background: var(--method-patch-bg);
		color: var(--method-patch);
		border-radius: var(--radius);
		font-size: 0.8125rem;
		font-weight: 500;
	}
	.save-notice {
		margin-top: 0.875rem;
		padding: 0.5rem 0.75rem;
		border: 1px solid color-mix(in srgb, var(--method-get, #16a34a) 40%, var(--color-border));
		background: color-mix(in srgb, var(--method-get, #16a34a) 8%, transparent);
		color: var(--method-get, #16a34a);
		border-radius: var(--radius);
		font-size: 0.8125rem;
		font-weight: 500;
	}
	.translation-notice {
		margin-top: 0.875rem;
		padding: 0.5rem 0.75rem;
		border: 1px solid var(--method-patch-bg);
		background: var(--method-patch-bg);
		color: var(--method-patch);
		border-radius: var(--radius);
		font-size: 0.8125rem;
	}
	.translation-notice strong { display: block; margin-top: 0.125rem; }
	.breadcrumb {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 0.375rem;
		font-size: 0.8125rem;
		margin-bottom: 0.5rem;
	}
	.crumb {
		color: var(--color-muted);
		text-decoration: none;
	}
	.crumb:hover { color: var(--color-primary); }
	.breadcrumb :global(.crumb-sep) { color: var(--color-muted); flex-shrink: 0; }
	/* Mobile TOC — hidden on desktop where the right sidebar shows it. */
	.mobile-toc { display: none; }
	@media (max-width: 1279px) {
		.mobile-toc {
			display: block;
			margin-bottom: 1.75rem;
			border: 1px solid var(--color-border);
			border-radius: var(--radius-card);
			background: var(--color-surface);
			padding: 0.75rem 1rem;
		}
		.mobile-toc summary {
			display: flex;
			align-items: center;
			gap: 0.5rem;
			font-size: 0.8125rem;
			font-weight: 600;
			text-transform: uppercase;
			letter-spacing: 0.05em;
			color: var(--color-muted);
			cursor: pointer;
			list-style: none;
		}
		.mobile-toc summary::-webkit-details-marker { display: none; }
		.mobile-toc ul {
			list-style: none;
			padding: 0;
			margin: 0.75rem 0 0;
			display: flex;
			flex-direction: column;
			gap: 0.25rem;
		}
		.mobile-toc a {
			font-size: 0.875rem;
			color: var(--color-text-subtle);
			text-decoration: none;
		}
		.mobile-toc a:hover { color: var(--color-primary); }
		.mobile-toc li.level-3 a { padding-inline-start: 1rem; font-size: 0.8125rem; }
	}
	.prev-next {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 1rem;
		margin-top: 2.5rem;
	}
	.pn-card {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
		padding: 1rem 1.25rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		background: var(--color-surface);
		text-decoration: none;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.pn-card:hover {
		border-color: var(--color-primary);
		box-shadow: 0 2px 10px color-mix(in srgb, var(--color-text) 5%, transparent);
	}
	.pn-placeholder { visibility: hidden; }
	.pn-next { text-align: end; }
	.pn-label {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		font-size: 0.75rem;
		color: var(--color-muted);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}
	.pn-card:hover .pn-label { color: var(--color-primary); }
	.pn-next .pn-label { justify-content: flex-end; }
	.pn-title {
		font-size: 0.9375rem;
		font-weight: 600;
		color: var(--color-text);
	}
	@media (max-width: 560px) {
		.prev-next { grid-template-columns: 1fr; }
		.pn-next { text-align: start; }
		.pn-next .pn-label { justify-content: flex-start; }
		.meta-edit-link { margin-inline-start: 0; }
	}
</style>
