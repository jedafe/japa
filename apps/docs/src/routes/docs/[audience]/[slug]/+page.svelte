<script lang="ts">
	import { untrack } from 'svelte';
	import { enhance } from '$app/forms';
	import { invalidateAll, goto } from '$app/navigation';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import TableOfContents from '$lib/components/shell/TableOfContents.svelte';
	import DocContent from '$lib/components/docs/DocContent.svelte';
	import DocEditor from '$lib/components/docs/DocEditor.svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { persistNavCollapse } from '$lib/nav-collapse';
	import { ArrowLeft, ArrowRight, CalendarClock, ChevronRight, Clock, ListTree, Pencil } from 'lucide-svelte';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';
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

	// Edit form state — reseeded from the page each time edit mode opens.
	let editTitle = $state('');
	let editCategory = $state('');
	let editSubcategory = $state('');
	let editStatus = $state('draft');
	let contentJson = $state<unknown>(null);
	let saving = $state(false);

	$effect(() => {
		if (editing) {
			editTitle = data.page.title;
			editCategory = data.page.category ?? '';
			editSubcategory = data.page.subcategory ?? '';
			editStatus = data.page.review_status;
		}
	});

	// The backend DocPage response has no breadcrumb/prev/next fields — derive
	// them from the nav tree (GET /docs/nav), which carries parent_id nesting
	// and position ordering for the current audience (Rule 16).
	const audienceTree = $derived(data.nav.find((tree) => tree.audience === data.audience));
	const flatNav = $derived(
		(audienceTree?.items ?? []).flatMap((item) => [item, ...(item.children ?? [])])
	);

	// Category/subcategory suggestions from sibling pages in this audience.
	const existingCategories = $derived(
		[...new Set(flatNav.map((i) => i.category).filter((c): c is string => !!c))]
	);
	const existingSubcategories = $derived(
		[
			...new Set(
				flatNav
					.filter((i) => !editCategory || i.category === editCategory)
					.map((i) => i.subcategory)
					.filter((c): c is string => !!c)
			)
		]
	);

	// Dynamic hreflang from enabled languages (§18, ADR-013). The canonical
	// list lives in the public site settings (i18n.enabled_languages). If the
	// setting is not available, fall back to the active locale rather than a
	// hardcoded list (Rule 16).
	const enabledLanguages = $derived(
		((page.data?.siteSettings?.['i18n.enabled_languages'] as string | undefined) ?? data.locale)
			.split(',').map((l) => l.trim()).filter(Boolean)
	);
	const defaultLanguage = $derived(
		(page.data?.siteSettings?.['i18n.default_language'] as string | undefined) ?? 'en'
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
</script>

<svelte:head>
	<title>{data.meta.title}</title>
	<meta name="description" content={data.meta.description} />
	<link rel="canonical" href={data.meta.ogUrl} />
	{#if data.isGated}
		<meta name="robots" content="noindex, nofollow" />
	{:else}
		<meta name="robots" content="index, follow" />
		<!-- hreflang for public audiences (Ref: §18 ROADMAP 07.05) -->
		{#each enabledLanguages as lang (lang)}
			<link
				rel="alternate"
				hreflang={lang}
				href={lang === defaultLanguage
					? data.meta.ogUrl
					: data.meta.ogUrl.replace(/\/docs\//, `/${lang}/docs/`)}
			/>
		{/each}
		<link rel="alternate" hreflang="x-default" href={data.meta.ogUrl} />
	{/if}
	<meta property="og:title" content={data.meta.title} />
	<meta property="og:description" content={data.meta.description} />
	<meta property="og:type" content={data.meta.ogType} />
	<meta property="og:url" content={data.meta.ogUrl} />
	{#if data.meta.ogImage}
		<meta property="og:image" content={data.meta.ogImage} />
	{/if}
	<meta name="twitter:card" content={data.meta.twitterCard} />
	<meta name="twitter:title" content={data.meta.twitterTitle} />
	<meta name="twitter:description" content={data.meta.twitterDescription} />
	{#if data.meta.twitterImage}
		<meta name="twitter:image" content={data.meta.twitterImage} />
	{/if}
	{#if data.meta.jsonLdString}
		{@html safeJsonLdScript(data.meta.jsonLdString)}
	{/if}
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
	{#snippet sidebar()}
		<TableOfContents headings={data.toc} />
	{/snippet}

	<article class="doc-article">
		<header class="doc-header">
			<!-- Breadcrumb (Ref: §44 ROADMAP 02.03) — Docs / Audience / parents -->
			<nav class="breadcrumb" aria-label="Breadcrumb">
				<a href="/docs" class="crumb">{t('audience.overview')}</a>
				<ChevronRight size={12} class="crumb-sep" />
				<a href="/docs/{data.audience}" class="crumb">
					{t(data.audienceLabelKey, data.audience)}
				</a>
				{#each breadcrumb as crumb (crumb.slug)}
					<ChevronRight size={12} class="crumb-sep" />
					<a href="/docs/{data.audience}/{crumb.slug}" class="crumb">{crumb.title}</a>
				{/each}
			</nav>
			<h1>{data.page.title}</h1>
			<div class="doc-meta">
				<span class="meta-item"><CalendarClock size={13} />{updatedLabel}</span>
				<span class="meta-item"><Clock size={13} />{t('page.read_time', '{min} min read', { min: String(readMinutes) })}</span>
				{#if data.page.ai_generated}
					<span class="ai-flag">{t('page.ai_assisted_draft')}</span>
				{/if}
			</div>
			{#if data.locale !== 'en' && data.page.content_lang === 'en'}
				<div class="translation-notice">
					{t('page.not_translated', 'This page is not yet translated. Showing the English version.')}
				</div>
			{/if}
		</header>

		{#if editing}
			<!-- Inline edit mode (§44 in-app authoring — docs.manage). -->
			{#if form?.error}
				<p class="form-error" role="alert">
					{form.message || t(`new_page.error.${form.error}`, 'Could not save the page.')}
				</p>
			{/if}
			<form
				method="POST"
				action="?/save"
				class="edit-form"
				use:enhance={({ formData }) => {
					formData.set('content', JSON.stringify(contentJson ?? {}));
					saving = true;
					return async ({ result, update }) => {
						saving = false;
						if (result.type === 'success') {
							await invalidateAll();
							await goto(viewPath);
						} else {
							// errors populate `form`; redirects are followed by update()
							await update();
						}
					};
				}}
			>
				<input type="hidden" name="page_id" value={data.page.id} />

				<label class="ef-field">
					<span class="ef-label">{t('new_page.field_title', 'Title')}</span>
					<input type="text" name="title" class="ef-input ef-title" required maxlength="200" bind:value={editTitle} />
				</label>

				<div class="ef-grid">
					<label class="ef-field">
						<span class="ef-label">{t('new_page.field_category', 'Category')}</span>
						<input
							type="text"
							name="category"
							class="ef-input"
							list="doc-categories"
							maxlength="120"
							bind:value={editCategory}
							placeholder={t('new_page.optional', 'Optional')}
						/>
						<datalist id="doc-categories">
							{#each existingCategories as c (c)}
								<option value={c}></option>
							{/each}
						</datalist>
					</label>

					<label class="ef-field">
						<span class="ef-label">{t('new_page.field_subcategory', 'Subcategory')}</span>
						<input
							type="text"
							name="subcategory"
							class="ef-input"
							list="doc-subcategories"
							maxlength="120"
							bind:value={editSubcategory}
							placeholder={t('new_page.optional', 'Optional')}
						/>
						<datalist id="doc-subcategories">
							{#each existingSubcategories as s (s)}
								<option value={s}></option>
							{/each}
						</datalist>
					</label>

					<label class="ef-field">
						<span class="ef-label">{t('new_page.field_status', 'Review status')}</span>
						<select name="review_status" class="ef-input" bind:value={editStatus}>
							{#each data.statuses as s (s)}
								<option value={s}>{t(`new_page.status.${s}`, s)}</option>
							{/each}
						</select>
					</label>
				</div>

				<DocEditor
					initialContent={data.editSeed}
					bind:value={contentJson}
					placeholder={t('new_page.content_placeholder', 'Start writing the documentation…')}
				/>

				<div class="ef-actions">
					<button type="submit" class="ef-save" disabled={saving}>
						{saving ? t('edit.saving', 'Saving…') : t('edit.save', 'Save changes')}
					</button>
					<a href={viewPath} class="ef-cancel">{t('editor.cancel', 'Cancel')}</a>
				</div>
			</form>
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
				/>
			</div>

			<!-- Prev/Next navigation (Ref: §44 ROADMAP 02.05) -->
			{#if prev || next}
				<nav class="prev-next" aria-label="Page navigation">
					{#if prev}
						<a href="/docs/{data.audience}/{prev.slug}" class="pn-card pn-prev">
							<span class="pn-label"><ArrowLeft size={14} /> {t('page.previous', 'Previous')}</span>
							<span class="pn-title">{prev.title}</span>
						</a>
					{:else}
						<span class="pn-card pn-placeholder"></span>
					{/if}
					{#if next}
						<a href="/docs/{data.audience}/{next.slug}" class="pn-card pn-next">
							<span class="pn-label">{t('page.next', 'Next')} <ArrowRight size={14} /></span>
							<span class="pn-title">{next.title}</span>
						</a>
					{:else}
						<span class="pn-card pn-placeholder"></span>
					{/if}
				</nav>
			{/if}

			<!-- Edit this page link (§44 in-app authoring — docs.manage only) -->
			{#if canEdit}
				<div class="edit-link-row">
					<a href="?edit=1" class="edit-link">
						<Pencil size={14} />
						{t('page.edit_this_page', 'Edit this page')}
					</a>
				</div>
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
		font-family: var(--font-sans);
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
	.translation-notice {
		margin-top: 0.875rem;
		padding: 0.5rem 0.75rem;
		border: 1px solid var(--method-patch-bg);
		background: var(--method-patch-bg);
		color: var(--method-patch);
		border-radius: var(--radius);
		font-size: 0.8125rem;
	}
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
		.mobile-toc li.level-3 a { padding-left: 1rem; font-size: 0.8125rem; }
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
	.pn-next { text-align: right; }
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
	.edit-link-row {
		margin-top: 1.5rem;
		padding-top: 1.5rem;
		border-top: 1px solid var(--color-border);
	}
	.edit-link {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		font-size: 0.8125rem;
		color: var(--color-muted);
		text-decoration: none;
	}
	.edit-link:hover { color: var(--color-primary); }

	/* Inline edit form (§44 in-app authoring) */
	.edit-form { display: flex; flex-direction: column; gap: 1rem; }
	.ef-field { display: flex; flex-direction: column; gap: 0.375rem; min-width: 0; }
	.ef-label {
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-text-subtle);
	}
	.ef-input {
		padding: 0.5rem 0.75rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.875rem;
		font-family: var(--font-sans);
	}
	.ef-input:focus { outline: none; border-color: var(--color-primary); }
	.ef-title { font-size: 1.25rem; font-weight: 600; }
	.ef-grid {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 0.875rem;
	}
	.ef-actions {
		display: flex;
		align-items: center;
		gap: 1rem;
		margin-top: 0.25rem;
	}
	.ef-save {
		padding: 0.625rem 1.5rem;
		border: none;
		border-radius: var(--radius);
		background: var(--color-primary);
		color: var(--color-primary-contrast);
		font-size: 0.875rem;
		font-weight: 600;
		cursor: pointer;
	}
	.ef-save:hover { filter: brightness(1.05); }
	.ef-save:disabled { opacity: 0.6; cursor: default; }
	.ef-cancel {
		font-size: 0.875rem;
		color: var(--color-muted);
		text-decoration: none;
	}
	.ef-cancel:hover { color: var(--color-text); }
	.form-error {
		margin: 0 0 0.25rem;
		padding: 0.75rem 1rem;
		border-radius: var(--radius);
		border: 1px solid color-mix(in srgb, var(--color-danger, #dc2626) 40%, var(--color-border));
		background: color-mix(in srgb, var(--color-danger, #dc2626) 8%, transparent);
		color: var(--color-danger, #dc2626);
		font-size: 0.875rem;
	}
	@media (max-width: 640px) {
		.ef-grid { grid-template-columns: 1fr; }
	}
	@media (max-width: 560px) {
		.prev-next { grid-template-columns: 1fr; }
		.pn-next { text-align: left; }
		.pn-next .pn-label { justify-content: flex-start; }
	}
</style>
