<script lang="ts">
	import { page } from '$app/state';
	import { goto, invalidate } from '$app/navigation';
	import { t } from '$lib/i18n';
	import { ArrowLeft, Check, Clock, Eye, Loader2, Pencil, Send, Trash2 } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import ReactionRail from '$lib/components/blog/ReactionRail.svelte';
	import CreatorCard from '$lib/components/blog/CreatorCard.svelte';
	import SidebarWidgetRenderer from '$lib/components/shell/SidebarWidgetRenderer.svelte';
	import CommentThread from '$lib/components/comments/CommentThread.svelte';
	import BlogSeriesNav from '$lib/components/blog/BlogSeriesNav.svelte';
	import { hydrateLinkPreviews } from '$lib/tiptap/hydrateLinkPreviews';
	import { hydrateMentions } from '$lib/tiptap/hydrateMentions';
	import { hydrateVideoEmbeds } from '$lib/tiptap/hydrateVideoEmbeds';
	import { safeJsonLdScript, sanitizeContentHtml } from '$lib/utils/safeHtml';
	import { breadcrumbJsonLd } from '$lib/seo/breadcrumbs';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const article = $derived(data.article);
	const deletedUserIds = $derived(
		data.deletedUserIds ? new Set<string>(data.deletedUserIds as string[]) : undefined
	);

	const isDraft = $derived(article.status === 'draft');
	const canSubmitForReview = $derived(data.canEdit && isDraft);
	let submittingForReview = $state(false);
	let reviewSubmitted = $state(false);

	async function submitForReview() {
		if (!canSubmitForReview || submittingForReview) return;
		submittingForReview = true;
		const res = await api.post(`/blog/${article.id}/submit-for-review`);
		if (!res.error) {
			reviewSubmitted = true;
			await invalidate(`blog:${article.slug}`);
		}
		submittingForReview = false;
	}

	let deleting = $state(false);
	async function deletePost() {
		if (!data.canDelete || deleting) return;
		const confirmed = window.confirm(t('blog.delete_confirm', 'Are you sure you want to delete this post?'));
		if (!confirmed) return;
		deleting = true;
		const res = await api.delete(`/blog/${article.id}`);
		if (!res.error) {
			await goto('/blog');
		} else {
			deleting = false;
		}
	}

	// Normalize sidebar widgets to the typed shape expected by SidebarWidgetRenderer.
	const sidebarWidgets = $derived(
		(data.rightSidebarWidgets as { id: string; widget_type: string; label: string; props: Record<string, unknown>; sort_order: number }[]).filter(
			(w) => w && typeof w.widget_type === 'string'
		)
	);
	const publishedDate = $derived(new Date(article.published_at || article.created_at).toLocaleDateString(undefined, { year: 'numeric', month: 'long', day: 'numeric' }));
	const title = $derived(`${article.title} — ${t('app.name')}`);
	const description = $derived(article.excerpt ?? article.body.slice(0, 160));
	const allowedIframeHosts = $derived(
		(data.siteSettings?.['content.sanitization.allowed_iframe_hosts'] as string | undefined) ?? undefined
	);
	const sanitizedBody = $derived(sanitizeContentHtml(article.body_html || article.body, allowedIframeHosts));

	// JSON-LD structured data (Section 14, Section 26 — SEO). The backend
	// resolves the admin-managed Article template server-side and returns it
	// in the response envelope's `json_ld` sidecar. Fall back to a basic
	// BlogPosting schema when the backend didn't emit one (no active template).
	const jsonLd = $derived(
		data.jsonLd ?? {
			'@context': 'https://schema.org',
			'@type': 'BlogPosting',
			headline: article.title,
			datePublished: article.published_at || article.created_at,
			author: { '@type': 'Person', name: article.author_name || t('app.name') },
			url: page.url.href,
			description
		}
	);

	// View tracking (Section 14 — POST /content/{id}/view). Fire-and-forget
	// on mount; the backend deduplicates per user/session per day.
	$effect(() => {
		if (!article.id) return;
		api.post(`/content/${article.id}/view`, {});
	});

	// BreadcrumbList JSON-LD (Section 26 — all pages with hierarchy).
	const breadcrumbLd = $derived(
		breadcrumbJsonLd([
			{ name: 'Home', path: page.url.origin },
			{ name: 'Blog', path: `${page.url.origin}/blog` },
			{ name: article.title, path: page.url.href }
		])
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={article.title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="article" />
	<meta property="og:url" content={page.url.href} />
	{#if article.featured_image_url}<meta property="og:image" content={article.featured_image_url} />{/if}
	<meta name="twitter:card" content="summary_large_image" />
	<meta name="twitter:title" content={article.title} />
	<meta name="twitter:description" content={description} />
	{@html safeJsonLdScript(jsonLd)}
	{@html safeJsonLdScript(breadcrumbLd)}
</svelte:head>

<!-- ADR-058 Exception #1: dev.to reading layout.
     Left nav is auto-collapsed by the layout (isExceptionRoute handles /create,
     but blog single uses a responsive grid here that works within the shell).
     On mobile the right sidebar is stacked below the article body. -->
{#snippet rightSidebar()}
	<div class="flex flex-col gap-4">
		<CreatorCard
			authorName={article.author_name}
			authorUsername={article.author_username}
			authorAvatarUrl={article.author_avatar_url}
			spaceName={article.space_name}
			spaceSlug={article.space_slug}
			authorId={article.author_id}
			initiallyFollowing={data.initiallyFollowing}
			authed={data.authed}
			currentUserId={data.currentUserId}
		/>
		<!-- Admin-managed sidebar widgets (Section 59, ADR-066). -->
		{#each sidebarWidgets as widget (widget.id)}
			<SidebarWidgetRenderer
				{widget}
				user={data.user}
				trendingTags={data.trendingTags}
				suggestedSpaces={data.suggestedSpaces}
				enabledLanguages={data.siteSettings?.['i18n.enabled_languages'] as string | undefined}
				isDashboard={false}
			/>
		{/each}
	</div>
{/snippet}

<!--
	Content single pages render the unified shell's left ad rail (Section 59) instead of a
	page-owned left reaction rail. The action bar is now a horizontal rail under the body.
-->
<div class="mx-auto grid max-w-[65rem] gap-8 py-8 pb-24 lg:pb-8 lg:grid-cols-1 xl:grid-cols-[1fr_20rem]">
	<!-- Center: article -->
	<article class="min-w-0 max-w-[41rem]">
		<div class="mb-8 flex flex-wrap items-center justify-between gap-3">
			<a href="/blog" class="inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]">
				<ArrowLeft size={16} /> {t('blog.back', 'Back to blog')}
			</a>
			<div class="flex flex-wrap items-center gap-2">
				{#if data.canEdit}
					<a
						href={`/create?draft=${article.id}`}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)]/40 hover:text-[var(--color-primary)]"
					>
						<Pencil size={14} /> {t('content.edit', 'Edit')}
					</a>
				{/if}
				{#if canSubmitForReview}
					<button
						type="button"
						onclick={submitForReview}
						disabled={submittingForReview || reviewSubmitted}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)]/40 hover:text-[var(--color-primary)] disabled:opacity-50"
					>
						{#if reviewSubmitted}
							<Check size={14} /> {t('blog.submitted', 'Submitted')}
						{:else if submittingForReview}
							<Loader2 size={14} class="animate-spin" /> {t('blog.submitting', 'Submitting…')}
						{:else}
							<Send size={14} /> {t('blog.submit_for_review', 'Submit for review')}
						{/if}
					</button>
				{/if}
				{#if data.canDelete}
					<button
						type="button"
						onclick={deletePost}
						disabled={deleting}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm font-medium text-[var(--color-destructive)] transition-colors hover:border-[var(--color-destructive)]/40 hover:bg-[var(--color-destructive)]/10 disabled:opacity-50"
					>
						{#if deleting}
							<Loader2 size={14} class="animate-spin" />
						{:else}
							<Trash2 size={14} />
						{/if}
						{t('content.delete', 'Delete')}
					</button>
				{/if}
			</div>
		</div>

		{#if article.featured_image_url}
			<img src={article.featured_image_url} alt={article.title} class="mb-8 aspect-[16/9] w-full rounded-[var(--radius-card)] object-cover" />
		{/if}

		<h1 class="text-balance text-4xl font-bold leading-tight tracking-tight text-[var(--color-text)] md:text-5xl" style="font-family: var(--font-serif);">{article.title}</h1>

		<div class="mt-6 flex flex-wrap items-center gap-4 border-b border-[var(--color-border)] pb-6 text-sm text-[var(--color-text-subtle)]">
			{#if article.author_name}
				<svelte:element
					this={article.author_username ? 'a' : 'span'}
					href={article.author_username ? `/u/${article.author_username}` : undefined}
					class="flex items-center gap-2 {article.author_username ? 'transition-colors hover:text-[var(--color-primary)]' : ''}"
				>
					{#if article.author_avatar_url}
						<img src={article.author_avatar_url} alt="" class="h-9 w-9 rounded-full object-cover" />
					{:else}
						<span class="grid h-9 w-9 place-items-center rounded-full bg-[var(--color-primary)]/10 text-sm font-bold text-[var(--color-primary)]">{article.author_name.charAt(0)}</span>
					{/if}
					<span class="font-medium text-[var(--color-text)]">{article.author_name}</span>
				</svelte:element>
			{/if}
			<span>·</span>
			<span>{publishedDate}</span>
			{#if article.read_time_minutes > 0}
				<span class="flex items-center gap-1"><Clock size={14} /> {article.read_time_minutes} {t('blog.min_read', 'min read')}</span>
			{/if}
			<span class="flex items-center gap-1"><Eye size={14} /> {article.view_count} views</span>
			{#if data.canEdit && article.status !== 'published'}
				{@const statusClass =
					article.status === 'draft'
						? 'bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]'
						: article.status === 'submitted' || article.status === 'in_review'
							? 'bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
							: 'bg-[var(--color-warning)]/10 text-[var(--color-warning)]'}
				<span class="rounded-full px-2.5 py-0.5 text-xs font-medium capitalize {statusClass}">
					{article.status.replace('_', ' ')}
				</span>
			{/if}
		</div>

		{#if article.tags && article.tags.length > 0}
			<div class="mt-4 flex flex-wrap gap-2">
				{#each article.tags as tag (tag)}
					<a href={`/tags/${tag}`} class="rounded-full bg-[var(--color-surface-subtle)] px-2.5 py-1 text-xs text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-primary)]/10 hover:text-[var(--color-primary)]">#{tag}</a>
				{/each}
			</div>
		{/if}

		{#if article.series_id && article.series_title && article.series_slug}
			<div class="mt-6">
				<BlogSeriesNav
					title={article.series_title}
					slug={article.series_slug}
					total={data.seriesPosts?.length ?? article.series_total ?? 0}
					posts={data.seriesPosts ?? []}
					currentSlug={article.slug}
					currentPart={article.series_part ?? 0}
				/>
			</div>
		{/if}

		<!-- Article body: prose TipTap HTML with serif reading font -->
		<div class="prose mt-8" use:hydrateLinkPreviews use:hydrateMentions use:hydrateVideoEmbeds>
			{@html sanitizedBody}
		</div>

		<!-- Horizontal reaction/action bar under the article body. -->
		<div class="mt-8">
			<ReactionRail
				contentId={article.id}
				voteScore={article.reshare_count}
				commentCount={article.comment_count}
				authed={data.authed}
				horizontal
			/>
		</div>

		<!-- Mobile right sidebar: author card + widgets, stacked below the body on small screens. -->
		<div class="mt-8 flex flex-col gap-4 xl:hidden">
			{@render rightSidebar()}
		</div>

		<!-- Unified comment engine (Section 49) — Reddit-style nested comments. -->
		<CommentThread
			comments={data.comments}
			totalCount={article.comment_count}
			contentId={article.id}
			contentType={article.content_type}
			authed={data.authed}
			currentUserId={data.currentUserId}
			canModerate={data.canModerate}
			loadError={data.commentsLoadError}
			deletedUserIds={deletedUserIds}
		/>

		<!-- Related content (Section 14 — GET /content/{id}/related). -->
		{#if data.relatedContent && data.relatedContent.length > 0}
			<section class="mt-8">
				<h2 class="mb-4 text-lg font-semibold text-[var(--color-text)]">{t('blog.related', 'Related')}</h2>
				<div class="grid gap-4 sm:grid-cols-2">
					{#each data.relatedContent as rel (rel.id)}
						<a
							href={rel.content_type === 'blog_post' ? `/blog/${rel.slug}` : `/content/${rel.slug}`}
							class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-primary)]/30"
						>
							{#if rel.featured_image_url}
								<img src={rel.featured_image_url} alt="" class="mb-3 aspect-[16/9] w-full rounded-[var(--radius)] object-cover" />
							{/if}
							<h3 class="line-clamp-2 font-medium text-[var(--color-text)]">{rel.title}</h3>
							{#if rel.author_name}
								<p class="mt-1 text-xs text-[var(--color-text-subtle)]">{rel.author_name}</p>
							{/if}
						</a>
					{/each}
				</div>
			</section>
		{/if}
	</article>

	<!-- Right: CreatorCard + admin-managed widgets. Hidden on mobile because a
	     stacked copy is rendered inside the article body. -->
	<aside class="hidden w-80 shrink-0 xl:block">
		<div class="flex flex-col gap-4 xl:sticky xl:top-24">
			{@render rightSidebar()}
		</div>
	</aside>
</div>
