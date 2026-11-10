<script lang="ts">
	import { t, siteName } from '#lib/i18n/index.js';
	import { page as pageStore } from '$app/state';
	import { api } from '#lib/api/client.js';
	import { ChevronRight, Lock, Pencil } from 'lucide-svelte';
	import SuggestEdit from '#lib/components/wiki/SuggestEdit.svelte';
	import WikiHelpfulWidget from '#lib/components/wiki/WikiHelpfulWidget.svelte';
	import PostEditor from '#lib/components/create/editors/PostEditor.svelte';
	import { tiptapToText, type TipTapDoc as TipDoc } from '#lib/tiptap/serialize.js';
	import { tiptapToHtml, type TipTapDoc } from '#lib/tiptap/serialize.js';
	import { sanitizeContentHtml } from '#lib/utils/safeHtml.js';
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import { breadcrumbJsonLd } from '#lib/seo/breadcrumbs.js';
	import { entityPageHref } from '#lib/utils/wikiEntityLinks.js';
	import type { PageData } from './$types';
	import type { EntityWikiPage } from './+page.server';

	let { data }: { data: PageData } = $props();
	let page = $derived(data.page);

	// Render the wiki content to safe HTML — same pipeline as the
	// space-bound wiki page (TipTap JSON → HTML → sanitize, Rule 16/§5.1).
	const htmlContent = $derived.by(() => {
		const c = page?.content;
		if (c == null) return '';
		if (typeof c === 'string') return sanitizeContentHtml(c);
		if (typeof c === 'object') {
			return sanitizeContentHtml(tiptapToHtml(c as unknown as TipTapDoc));
		}
		return '';
	});

	// §wiki-links — mode:"related" grid heading: an offering page lists
	// other universities with the same catalog course; a program page
	// lists every university offering it.
	const relatedHeading = $derived(
		page?.kind === 'course'
			? t('wiki.also_offered_at', 'Also offered at')
			: t('wiki.universities_offering', 'Universities offering this program')
	);

	const authed = $derived(pageStore.data.user != null);
	// §80 Super-Admin Direct Publish — `*` wildcard counts as wiki.edit and
	// bypasses lock state, same contract as the space-bound page.
	const isSuperAdmin = $derived(data.userPermissions?.includes('*') === true);
	const canEdit = $derived(
		authed && (isSuperAdmin || (!page.is_locked && data.userPermissions?.includes('wiki.edit') === true))
	);
	// Category label for the meta line — resolve through the page's tab
	// strip (label is server-English + i18n-able), not the raw token.
	const categoryLabel = $derived(
		page?.categories?.find((c) => c.category === page.category)?.label ??
			page?.category?.replaceAll('_', ' ') ??
			''
	);
	const entityEndpoint = $derived(
		`/wiki/e/${encodeURIComponent(page.type_slug)}/${encodeURIComponent(page.entity_slug)}/${encodeURIComponent(page.slug)}`
	);
	// §43 SEO head — mirrors the space page: description from the approved
	// prose, canonical + og, and noindex,follow on lazy/thin stubs.
	// Canonical targets the page's canonical type key — several type slugs
	// can resolve the same entity page (e.g. a student route is served by
	// both 'visa_route' and 'student_visa'), so the URL path alone would
	// mint duplicate self-canonicals. canonical_type_slug comes from the
	// API; absent it, fall back to the request path.
	const canonicalUrl = $derived(
		page
			? pageStore.url.origin +
					entityPageHref(
						page.canonical_type_slug ?? page.type_slug,
						page.entity_slug,
						page.slug
					)
			: pageStore.url.origin + pageStore.url.pathname
	);
	const metaDescription = $derived.by(() => {
		const c = page?.content;
		const text = typeof c === 'string' ? c : c ? tiptapToText(c as unknown as TipTapDoc) : '';
		const clean = text.replace(/\s+/g, ' ').trim();
		return clean
			? clean.slice(0, 160)
			: t('wiki.entity_description', '{title} — community-contributed info for {name}.', {
					title: page.title,
					name: page.entity_name
				});
	});
	const entityArticleJsonLd = $derived(
		page
			? {
					'@context': 'https://schema.org',
					'@type': 'Article',
					headline: page.title,
					description: metaDescription,
					url: canonicalUrl,
					about: { '@type': 'Thing', name: page.entity_name }
				}
			: null
	);
	const entityBreadcrumbJsonLd = $derived(
		page
			? breadcrumbJsonLd([
					{ name: 'Home', path: pageStore.url.origin },
					{ name: t('nav.wiki', 'Wiki'), path: `${pageStore.url.origin}/wiki` },
					...(page.ancestors ?? []).map((anc) => ({
						name: anc.name,
						path: `${pageStore.url.origin}/wiki/${encodeURIComponent(anc.slug)}`
					})),
					// The entity itself has no standalone index URL —
					// /wiki/e/{type}/{entity} is not a route — so the crumb
					// carries a name only (schema.org ListItem without item).
					{ name: page.entity_name, path: '' },
					{ name: page.title, path: canonicalUrl }
				])
			: null
	);
	const siblings = $derived(page?.siblings ?? []);
	const prevSibling = $derived.by(() => {
		const i = siblings.findIndex((s) => s.current);
		return i > 0 ? siblings[i - 1] : null;
	});
	const nextSibling = $derived.by(() => {
		const i = siblings.findIndex((s) => s.current);
		return i >= 0 && i < siblings.length - 1 ? siblings[i + 1] : null;
	});

	// Body edit — same flow as space-bound pages: TipTap editor posts a
	// revision to /wiki/e/.../revisions (community_edits + reputation gates
	// apply server-side, identical to the space route).
	let editing = $state(false);
	let editDirty = $state(false);
	let editBodyHtml = $state('');
	let editBodyJson = $state<Record<string, unknown>>({ type: 'doc', content: [] });
	let summary = $state('');
	let loading = $state(false);
	let submitted = $state(false);
	let autoApproved = $state(false);
	let errorKey = $state('');

	async function startEdit() {
		submitted = false;
		autoApproved = false;
		errorKey = '';
		editing = true;
		editDirty = false;
		// Fetch the raw (unresolved) revision so authored {{...}} literals
		// survive a re-save — the display payload has them resolved (same
		// contract as the space-bound page's edit flow).
		const res = await api.get<EntityWikiPage>(`${entityEndpoint}?raw=1`);
		const c = res.data?.content ?? page?.content;
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

	async function submitRevision(e: SubmitEvent) {
		e.preventDefault();
		errorKey = '';
		if (!page) return;
		const plainText = tiptapToText(editBodyJson as unknown as TipDoc).trim();
		if (!plainText) {
			errorKey = 'wiki.content_required';
			return;
		}
		loading = true;
		try {
			const res = await api.post<{ id: string; status: string; auto_approved?: boolean }>(
				`${entityEndpoint}/revisions`,
				{ content: editBodyJson, summary: summary.trim() || null }
			);
			if (res.data) {
				submitted = true;
				autoApproved = res.data.auto_approved === true;
				editing = false;
			} else if (res.error) {
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
</script>

<SeoHead
	title="{page.title} — {page.entity_name} | {siteName()} Wiki"
	description={metaDescription}
	ogTitle="{page.title} — {page.entity_name}"
	canonical={canonicalUrl}
	ogType="article"
	robots={page.indexable !== true ? 'noindex,follow' : ''}
	jsonLd={[entityArticleJsonLd, entityBreadcrumbJsonLd].filter(Boolean)}
/>

<article class="mx-auto max-w-3xl space-y-4 px-4 py-6">
	<header class="space-y-1">
		<nav aria-label={t('a11y.breadcrumb', 'Breadcrumb')} class="text-xs text-[var(--color-muted)]">
			<ol class="flex items-center gap-1.5">
				<li><a href="/wiki" class="transition-colors hover:text-[var(--color-text)]">{t('nav.wiki', 'Wiki')}</a></li>
				{#each page.ancestors ?? [] as anc (anc.slug)}
					<li aria-hidden="true"><ChevronRight class="rtl:-scale-x-100" size={12} /></li>
					<li>
						<a href="/wiki/{encodeURIComponent(anc.slug)}" class="truncate transition-colors hover:text-[var(--color-text)]">{anc.name}</a>
					</li>
				{/each}
				<li aria-hidden="true"><ChevronRight class="rtl:-scale-x-100" size={12} /></li>
				<li class="truncate">{page.entity_name}</li>
				<li aria-hidden="true"><ChevronRight class="rtl:-scale-x-100" size={12} /></li>
				<li class="truncate text-[var(--color-text)]">{page.title}</li>
			</ol>
		</nav>
		<h1 class="text-2xl font-bold text-[var(--color-text)]">{page.title}</h1>
		<div class="flex flex-wrap items-center justify-between gap-2">
			<p class="text-sm text-[var(--color-muted)]">
				{page.entity_name} · <span>{categoryLabel}</span>
				{#if page.is_locked}
					<span class="ms-2 inline-flex items-center gap-1 rounded bg-amber-500/10 px-2 py-0.5 text-xs text-amber-600 dark:text-amber-400"><Lock size={12} />{t('wiki.locked', 'Locked')}</span>
				{/if}
				{#if page.lazy}
					<span class="ms-2 rounded bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs">{t('wiki.not_yet_written', 'not yet written')}</span>
				{/if}
				{#if page.revision_summary}
					<span class="ms-2 text-xs">{page.revision_summary}</span>
				{/if}
				{#if page.submitted_by}
					<span class="ms-2 text-xs">· {t('wiki.submitted_by', 'by {name}', { name: page.submitted_by })}</span>
				{/if}
				{#if page.id}
					<a href={entityEndpoint + '/history'} class="ms-2 text-xs text-[var(--color-primary-text)] hover:underline">{t('wiki.history', 'History')}</a>
				{/if}
			</p>
			<!-- Same affordance as space-bound pages — field-op suggestions on the
			     bound entity (the dialog self-gates on wiki.structured_edits +
			     auth, so it renders only when eligible). -->
			{#if page.entity_id}
				<SuggestEdit
					entityType={page.kind}
					entityId={page.entity_id}
					groupId={page.field_group_id}
					proposeGroupSlug={page.field_group_slug}
				/>
			{/if}
		</div>
	</header>

	{#if page.categories && page.categories.length > 1}
		<nav
			class="flex gap-1 overflow-x-auto border-b border-[var(--color-border)] pb-2"
			aria-label={t('wiki.categories', 'Categories')}
		>
			{#each page.categories as cat (cat.slug)}
				{@const active = cat.category === page.category}
				<a
					href={entityPageHref(page.type_slug, page.entity_slug, cat.slug)}
					rel="nofollow"
					class="whitespace-nowrap rounded-full px-3 py-1 text-xs font-medium transition-colors {active
						? 'bg-[var(--color-primary)] text-white'
						: 'text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]'}"
				>
					{cat.label}
				</a>
			{/each}
		</nav>
	{/if}


	{#if editing}
		<form onsubmit={submitRevision} class="space-y-4 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-5" novalidate>
			<div class="flex flex-col gap-1.5">
				<span class="text-sm font-medium text-[var(--color-text)]">{t('wiki.content_label', 'Content')}</span>
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
				<label for="entity-wiki-summary" class="text-sm font-medium text-[var(--color-text)]">{t('wiki.edit_summary_label', 'Edit summary')}</label>
				<input
					id="entity-wiki-summary"
					type="text"
					value={summary}
					placeholder={t('wiki.edit_summary_placeholder', 'Briefly describe your changes')}
					oninput={(e) => (summary = e.currentTarget.value)}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
				/>
			</div>
			{#if errorKey}
				<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
					{t(errorKey, t('wiki.submit_failed', 'Failed to submit revision. Please try again.'))}
				</p>
			{/if}
			<div class="flex justify-end gap-2 pt-1">
				<button type="button" onclick={() => (editing = false)} class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">{t('wiki.cancel', 'Cancel')}</button>
				<button type="submit" disabled={loading} class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition hover:opacity-90 disabled:opacity-50">{loading ? t('wiki.submitting', 'Submitting…') : t('wiki.submit_revision', 'Submit revision')}</button>
			</div>
		</form>
	{:else}
		{#if submitted}
			<p class="rounded-[var(--radius)] bg-green-500/10 px-3 py-2 text-sm text-green-600 dark:text-green-400" role="status">
				{autoApproved ? t('wiki.revision_published', 'Your edit is live.') : t('wiki.revision_submitted', 'Revision submitted for review.')}
			</p>
		{/if}
		{#if htmlContent}
			<div class="prose max-w-none">{@html htmlContent}</div>
		{:else}
			<p class="text-sm text-[var(--color-muted)]">{t('wiki.empty_content', 'This page has no content yet.')}</p>
		{/if}
		{#if canEdit}
			<button
				type="button"
				onclick={startEdit}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
			>
				<Pencil size={14} />
				{t('wiki.edit_content', 'Edit content')}
			</button>
		{:else if page.is_locked}
			<span class="text-xs text-[var(--color-muted)]">{t('wiki.locked_body', 'This page is locked and cannot be edited.')}</span>
		{/if}
	{/if}

	{#if page.fields && page.fields.fields.length > 0}
		<section class="space-y-2">
			<h2 class="text-sm font-semibold uppercase tracking-wide text-[var(--color-muted)]">
				{page.fields.name}
			</h2>
			<dl class="divide-y divide-[var(--color-border)] rounded-lg border border-[var(--color-border)]">
				{#each page.fields.fields as field}
					<div class="grid grid-cols-3 gap-4 px-4 py-2.5">
						<dt class="text-sm text-[var(--color-muted)]">{field.label}</dt>
						<dd class="col-span-2 text-sm text-[var(--color-text)]">
							{#if field.value != null}
								{typeof field.value === 'object' ? JSON.stringify(field.value) : String(field.value)}
							{:else}
								<span class="italic text-[var(--color-muted)]">—</span>
							{/if}
						</dd>
					</div>
				{/each}
			</dl>
		</section>
	{/if}

	{#if !editing && page.id}
		<WikiHelpfulWidget spaceSlug={page.entity_slug} pageSlug={page.slug} helpful={page.helpful} {authed} endpoint={entityEndpoint + '/feedback'} />
	{/if}

	{#if !editing && (prevSibling || nextSibling)}
		<nav class="flex items-center justify-between gap-3 border-t border-[var(--color-border)] pt-4" aria-label={t('wiki.page_nav', 'Page navigation')}>
			{#if prevSibling}
				<a href={entityPageHref(page.type_slug, page.entity_slug, prevSibling.slug)} rel="nofollow" class="min-w-0 flex-1 truncate text-sm text-[var(--color-primary-text)] hover:underline">← {prevSibling.title}</a>
			{:else}
				<span class="flex-1"></span>
			{/if}
			{#if nextSibling}
				<a href={entityPageHref(page.type_slug, page.entity_slug, nextSibling.slug)} rel="nofollow" class="min-w-0 flex-1 truncate text-end text-sm text-[var(--color-primary-text)] hover:underline">{nextSibling.title} →</a>
			{/if}
		</nav>
	{/if}

	{#if page.related && page.related.items.length > 0}
		<section class="space-y-3">
			<h2 class="text-sm font-semibold uppercase tracking-wide text-[var(--color-muted)]">
				{relatedHeading}
			</h2>
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
				{#each page.related.items as uni (uni.slug)}
					<div class="flex flex-col gap-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3 transition-colors hover:border-[var(--color-primary)]">
						<a
							href="/wiki/{encodeURIComponent(uni.slug)}"
							class="flex items-center gap-2 text-sm font-medium text-[var(--color-text)] hover:text-[var(--color-primary-text)]"
							data-sveltekit-preload-data="hover"
						>
							{uni.name}
							{#if uni.geo_code}
								<span class="rounded bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10px] uppercase text-[var(--color-muted)]">{uni.geo_code}</span>
							{/if}
						</a>
						{#if uni.offering_slug}
							<a
								href={entityPageHref('course', uni.offering_slug)}
								rel="nofollow"
								class="text-xs text-[var(--color-muted)] hover:text-[var(--color-primary-text)]"
							>
								{t('wiki.view_at_university', 'View at {name}', { name: uni.name })}
							</a>
						{/if}
					</div>
				{/each}
			</div>
			{#if page.related.total > page.related.items.length}
				<p class="text-xs text-[var(--color-muted)]">
					{t('wiki.directory_truncated', 'Showing {shown} of {total} — search to find the rest.', {
						shown: page.related.items.length,
						total: page.related.total
					})}
				</p>
			{/if}
		</section>
	{/if}
</article>
