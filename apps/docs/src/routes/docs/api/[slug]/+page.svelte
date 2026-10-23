<script lang="ts">
	import { untrack } from 'svelte';
	import { onMount } from 'svelte';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import TableOfContents from '$lib/components/shell/TableOfContents.svelte';
	import DocContent from '$lib/components/docs/DocContent.svelte';
	import DocEditForm from '$lib/components/docs/DocEditForm.svelte';
	import ApiRefTabs from '$lib/components/docs/ApiRefTabs.svelte';
	import MethodBadge from '$lib/components/docs/MethodBadge.svelte';
	import { t } from '$lib/i18n';
	import SeoHead from '$lib/seo/SeoHead.svelte';
	import { parseManagedCategories } from '$lib/docs/catalog';
	import { persistNavCollapse } from '$lib/nav-collapse';
	import { page } from '$app/state';
	import { CalendarClock, ChevronRight, Clock, Eye, ListTree, Pencil } from 'lucide-svelte';
	import { env } from '$env/dynamic/public';
	import type { PageData, ActionData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();

	// Inline editing parity with /docs/[audience]/[slug] (§44 amendment 2).
	let canEdit = $derived(data.canEditDocs === true);
	let editing = $derived(canEdit && page.url.searchParams.get('edit') === '1');
	const viewPath = $derived(`/docs/api/${data.page.slug}`);

	// One-shot notices (?created=…/?saved=…) — strip after display.
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

	// Category/subcategory datalist suggestions — case-folded dedupe, merged
	// with the admin-managed catalog (docs.categories).
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


	let collapsed = $state(untrack(() => data.navCollapsed));

	async function toggleCollapse() {
		collapsed = !collapsed;
		await persistNavCollapse(collapsed);
	}

	// OpenAPI paths are relative to the spec's server URL (…/api/v1) — prepend
	// the configured API base so the sample is copy-paste runnable. Join with
	// PUBLIC_MAIN_ORIGIN when the API base is a relative path.
	const apiBaseUrl = env.PUBLIC_JC_API_URL ?? '/api/v1';
	const mainOrigin = (env.PUBLIC_MAIN_ORIGIN ?? '').replace(/\/$/, '');
	const curlBaseUrl = apiBaseUrl.startsWith('http')
		? apiBaseUrl.replace(/\/$/, '')
		: mainOrigin
			? `${mainOrigin}${apiBaseUrl}`
			: apiBaseUrl;

	// Derive code-pane samples from the matched operation (best-effort).
	let method = $derived(
		data.operation ? (data.operation.method as 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE') : 'GET'
	);
	let path = $derived(data.operation?.path ?? '');
	// DOC-005: public API key auth uses the X-API-Key header.
	let requestSample = $derived(
		data.operation
			? `curl -X ${method} "${curlBaseUrl}${path}" \\\n  -H "X-API-Key: $JC_API_KEY"${method !== 'GET' && method !== 'DELETE' ? ' \\\n  -H "Content-Type: application/json"' : ''}`
			: ''
	);
	let responseSample = $derived(
		data.operation
			? `// 200 OK\n{\n  "data": { ... }\n}`
			: ''
	);
	let exampleSample = $derived(requestSample);

	// Doc-page meta parity with [audience]/[slug] — updated date + read time.
	const localeTag = $derived((page.data?.locale as string | undefined) ?? 'en');
	const readMinutes = $derived.by(() => {
		const words = data.html.replace(/<[^>]+>/g, ' ').trim().split(/\s+/).filter(Boolean).length;
		return Math.max(1, Math.round(words / 200));
	});
	const updatedLabel = $derived(
		t('page.updated', 'Updated {date}', {
			date: new Date(data.page.updated_at).toLocaleDateString(localeTag, {
				year: 'numeric',
				month: 'short',
				day: 'numeric'
			})
		})
	);
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
	activeAudience="api"
	user={data.user}
	collapsed={collapsed}
	onToggleCollapse={toggleCollapse}
	variant="api"
>
	{#snippet apiPane()}
		<div class="api-pane">
			{#if editing}
				<!-- No TOC while editing — it reflects the published content, not
				     the in-progress draft. -->
			{:else if data.operation}
				<div class="api-pane-head">
					<MethodBadge method={method} />
					<code class="api-pane-path">{path}</code>
				</div>
				<p class="api-pane-summary">{data.operation.summary ?? ''}</p>
				<ApiRefTabs
					method={method}
					path={path}
					{requestSample}
					{responseSample}
					{exampleSample}
				/>
			{:else}
				<TableOfContents headings={data.toc} />
			{/if}
		</div>
	{/snippet}

	<article class="doc-article">
		<header class="doc-header">
			<nav class="breadcrumb" aria-label={t('a11y.breadcrumb', 'Breadcrumb')}>
				<a href="/docs" class="crumb">{t('audience.overview')}</a>
				<ChevronRight size={12} class="crumb-sep rtl:-scale-x-100" />
				<a href="/docs/api" class="crumb">{t('audience.api')}</a>
			</nav>
			<h1>{data.page.title}</h1>
			{#if data.operation}
				<div class="op-line">
					<MethodBadge method={method} />
					<code class="op-path">{path}</code>
				</div>
			{/if}
			<div class="doc-meta">
				<span class="meta-item"><CalendarClock size={13} />{updatedLabel}</span>
				<span class="meta-item"><Clock size={13} />{t('page.read_time', '{min} min read', { min: String(readMinutes) })}</span>
				{#if data.page.review_status !== 'published'}
					<span class="status-badge status-{data.page.review_status}">
						{t(`new_page.status.${data.page.review_status}`, data.page.review_status)}
					</span>
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
		</header>

		{#if editing}
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
					audience={data.page.audience}
					slug={data.page.slug}
					html={data.html}
					lastReviewedAt={data.page.last_reviewed_at}
					lastReviewedBy={data.page.last_reviewed_by}
					published={data.page.review_status === 'published'}
				/>
			</div>
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
	/* Mobile TOC — the right code pane hides below 1280px. */
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
	.op-line {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-top: 0.75rem;
		flex-wrap: wrap;
	}
	.op-path {
		font-family: var(--font-mono);
		font-size: 0.875rem;
		color: var(--color-text);
		background: var(--color-surface-subtle);
		padding: 0.25rem 0.5rem;
		border-radius: 4px;
	}
	.api-pane { display: flex; flex-direction: column; gap: 0.75rem; }
	.api-pane-head {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}
	.api-pane-path {
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--color-text);
		background: var(--color-surface-subtle);
		padding: 0.125rem 0.375rem;
		border-radius: 4px;
		word-break: break-all;
	}
	.api-pane-summary {
		font-size: 0.8125rem;
		color: var(--color-text-subtle);
		margin: 0;
	}
</style>
