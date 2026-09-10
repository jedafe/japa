<script lang="ts">
	import { page } from '$app/state';
	import { Heart, MessageSquare, Eye, Bookmark, Share, Play, Clock } from 'lucide-svelte';
	import { t, i18n } from '$lib/i18n';
	import { relativeTime } from '$lib/feed/time';
	import { contentTypeMeta, readPostLabel } from '$lib/feed/contentTypeMeta';
	import { contentHref } from '$lib/utils/contentLink';
	import { sanitizeContentHtml, safeJsonLdScript } from '$lib/utils/safeHtml';
	import { formatDuration } from '$lib/utils/duration';
	import EntityHoverChip from '$lib/components/feed/EntityHoverChip.svelte';
	import CommentThread from '$lib/components/comments/CommentThread.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const post = $derived(data.post);
	const meta = $derived(contentTypeMeta(post.content_type));
	const accentVar = $derived(`var(${meta.accentVar}, var(--color-primary))`);
	const Icon = $derived(meta.icon);
	const actionLabel = $derived(readPostLabel(post.content_type));
	const canonicalHref = $derived(data.canonicalHref);
	const time = $derived(relativeTime(post.published_at ?? post.created_at, new Date(), i18n.locale));
	const duration = $derived(formatDuration(post.duration_secs));
	const deletedUserIds = $derived(
		data.deletedUserIds ? new Set<string>(data.deletedUserIds) : undefined
	);

	const allowedIframeHosts = $derived(
		(data.siteSettings?.['content.sanitization.allowed_iframe_hosts'] as string | undefined) ?? undefined
	);
	const sanitizedBody = $derived(sanitizeContentHtml(post.body_html || post.body, allowedIframeHosts));
	const hasPreview = $derived(!!post.excerpt);
	let showFull = $state(false);
	const preview = $derived(post.excerpt ? sanitizeContentHtml(post.excerpt, undefined) : sanitizedBody);

	const regionNames = $derived(
		(() => {
			try {
				return new Intl.DisplayNames([i18n.locale], { type: 'region' });
			} catch {
				return null;
			}
		})()
	);
	function countryName(code?: string | null) {
		if (!code) return '';
		const upper = code.toUpperCase();
		return regionNames?.of(upper) ?? upper;
	}
	const routeLabel = $derived(
		post.source_country && post.target_country
			? `${countryName(post.source_country)} → ${countryName(post.target_country)}`
			: post.target_country
				? countryName(post.target_country)
				: post.source_country
					? countryName(post.source_country)
					: null
	);
</script>

<svelte:head>
	<title>{post.title}</title>
	<meta name="description" content={post.meta_description ?? post.excerpt ?? ''} />
	<meta name="robots" content="noindex, follow" />
	<link rel="canonical" href={canonicalHref} />
	<meta property="og:title" content={post.title} />
	<meta property="og:description" content={post.meta_description ?? post.excerpt ?? ''} />
	<meta property="og:url" content={page.url.href} />
	<meta property="og:type" content="article" />
	{#if post.og_image_url || post.featured_image_url || post.header_image_url}
		<meta property="og:image" content={post.og_image_url || post.featured_image_url || post.header_image_url} />
	{/if}
	{#if data.jsonLd}
		{@html safeJsonLdScript(JSON.stringify(data.jsonLd))}
	{/if}
</svelte:head>

<article class="mx-auto w-full max-w-3xl rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 sm:p-6">
	<!-- Source row -->
	<div class="mb-4 flex flex-wrap items-center gap-2 text-xs text-[var(--color-muted)]">
		<span class="flex items-center gap-1 font-semibold uppercase tracking-wide" style:color={accentVar}>
			<Icon size={14} />
			<a href={canonicalHref} class="hover:underline">{actionLabel}</a>
		</span>
		{#if routeLabel}
			<span class="inline-flex items-center rounded-full bg-[var(--color-surface-subtle)] px-2 py-1 text-[11px] font-medium text-[var(--color-text-subtle)]">
				{routeLabel}
			</span>
		{/if}
		{#if post.organization}
			<EntityHoverChip
				type="organization"
				id={post.organization.id}
				slug={post.organization.slug}
				name={post.organization.name}
				avatar={post.organization.logo_url}
			/>
		{:else if !routeLabel && post.space_name && post.space_slug}
			<EntityHoverChip
				type="space"
				id={post.space_id ?? post.space_slug}
				slug={post.space_slug}
				name={post.space_name}
				avatar={post.space_avatar_url}
			/>
		{/if}
		{#if post.author_name}
			<EntityHoverChip
				type="user"
				id={post.author_id ?? ''}
				slug={post.author_username ?? ''}
				name={post.author_name}
				avatar={post.author_avatar_url}
			/>
		{/if}
		<span class="ml-auto">{time}</span>
		{#if post.read_time_minutes}
			<span>· {post.read_time_minutes} {t('feed.min_read', 'min read')}</span>
		{/if}
	</div>

	<h1 class="mb-3 text-2xl font-bold leading-tight tracking-tight text-[var(--color-text)] sm:text-3xl">
		{post.title}
	</h1>

	{#if post.video_source_id || post.content_type === 'video'}
		{#if post.featured_image_url || post.header_image_url}
			<a href={canonicalHref} class="relative mb-4 block aspect-video overflow-hidden rounded-[var(--radius-card)]">
				<img
					src={post.featured_image_url || post.header_image_url}
					alt={post.title}
					loading="eager"
					class="h-full w-full object-cover"
				/>
				<span class="pointer-events-none absolute inset-0 grid place-items-center bg-black/25">
					<span class="grid h-14 w-14 place-items-center rounded-full bg-black/60 text-white">
						<Play size={24} class="ml-1" />
					</span>
				</span>
				{#if duration}
					<span class="absolute bottom-2 right-2 rounded bg-black/75 px-1.5 py-0.5 text-[10.5px] font-semibold text-white">{duration}</span>
				{/if}
			</a>
		{/if}
	{/if}

	<div class="prose prose-sm max-w-none text-[var(--color-text)] sm:prose-base">
		{@html showFull ? sanitizedBody : preview}
	</div>

	{#if hasPreview && !showFull}
		<button
			type="button"
			class="mt-2 text-sm font-medium hover:underline"
			style:color={accentVar}
			onclick={() => (showFull = true)}
		>
			{t('post.read_more', 'Read more')}
		</button>
	{/if}

	{#if (showFull || !hasPreview) && post.content_type !== 'video'}
		<a href={canonicalHref} class="mt-4 inline-flex items-center gap-1 text-sm font-medium hover:underline" style:color={accentVar}>
			{t('post.view_canonical', '{action}', { action: actionLabel })}
			<Icon size={14} />
		</a>
	{/if}

	<!-- Reaction rail -->
	<div class="mt-6 flex flex-wrap items-center gap-4 border-t border-[var(--color-border)] pt-4 text-sm text-[var(--color-muted)] [font-variant-numeric:tabular-nums]">
		<button type="button" class="group inline-flex items-center gap-1.5 rounded-full px-3 py-1.5 transition-colors hover:bg-[var(--color-surface-subtle)]">
			<Heart size={18} class="text-rose-500 group-hover:fill-rose-500" />
			<span>{post.reaction_count ?? 0}</span>
		</button>
		<button type="button" class="group inline-flex items-center gap-1.5 rounded-full px-3 py-1.5 transition-colors hover:bg-[var(--color-surface-subtle)]">
			<MessageSquare size={18} />
			<span>{post.comment_count ?? 0}</span>
		</button>
		<span class="inline-flex items-center gap-1.5">
			<Eye size={18} />
			<span>{post.view_count ?? 0}</span>
		</span>
		{#if post.bookmark_count != null}
			<span class="inline-flex items-center gap-1.5">
				<Bookmark size={18} />
				<span>{post.bookmark_count}</span>
			</span>
		{/if}
		<button type="button" class="ml-auto inline-flex items-center gap-1.5 rounded-full px-3 py-1.5 transition-colors hover:bg-[var(--color-surface-subtle)]" aria-label={t('post.share', 'Share')}>
			<Share size={18} />
		</button>
	</div>
</article>

<!-- Unified comment thread -->
<section class="mx-auto mt-6 w-full max-w-3xl">
	<CommentThread
		comments={data.comments}
		totalCount={post.comment_count}
		contentId={post.id}
		contentType={post.content_type}
		authed={data.authed}
		currentUserId={data.currentUserId}
		canModerate={data.canModerate}
		loadError={data.commentsLoadError}
		deletedUserIds={deletedUserIds}
	/>
</section>
