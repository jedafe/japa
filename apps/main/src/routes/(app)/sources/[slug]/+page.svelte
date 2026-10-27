<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { Globe, ExternalLink, Users, FileText, Tag } from 'lucide-svelte';
	import { FollowButton, UrlTabs, type UrlTab } from '$lib/components/ui';
	import SourceActions from '$lib/components/source/SourceActions.svelte';
	import SourceBlockButton from '$lib/components/source/SourceBlockButton.svelte';
	import SourceDailyCard from '$lib/components/source/SourceDailyCard.svelte';
	import { sourceTrustLabel } from '$lib/sources';
	import { api } from '$lib/api/client';
	import { remoteImageUrl } from '$lib/utils/remoteImage';
	import { breadcrumbJsonLd } from '$lib/seo/breadcrumbs';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';
	import type { PageData } from './$types';
	import type { Source, SourceItemEnvelope } from '$lib/types/sources';

	let { data }: { data: PageData } = $props();

	// svelte-ignore state_referenced_locally
	let source = $state(data.source);
	// svelte-ignore state_referenced_locally
	let followerCount = $state(data.source.follower_count);
	// svelte-ignore state_referenced_locally
	let isBlocked = $state(data.source.is_blocked ?? false);
	let logoError = $state(false);
	// svelte-ignore state_referenced_locally
	let items = $state(data.feed.items);
	// svelte-ignore state_referenced_locally
	let feedMeta = $state(data.feed.meta);
	let loadingMore = $state(false);

	$effect(() => {
		source = data.source;
		followerCount = data.source.follower_count;
		isBlocked = data.source.is_blocked ?? false;
		items = data.feed.items;
		feedMeta = data.feed.meta;
	});

	const hasMore = $derived((feedMeta.total ?? 0) > items.length);

	async function loadMore() {
		if (loadingMore || !hasMore) return;
		loadingMore = true;
		const params = new URLSearchParams();
		if (data.entityType) params.set('entity_type', data.entityType);
		params.set('sort', data.sort);
		params.set('page_size', String(data.feed.meta.page_size ?? 24));
		params.set('page_offset', String(items.length));
		const res = await api.get<SourceItemEnvelope>(`/sources/${encodeURIComponent(source.slug)}/items?${params.toString()}`);
		if (res.data) {
			items = [...items, ...res.data.items];
			feedMeta = { ...feedMeta, ...res.data.meta };
		}
		loadingMore = false;
	}

	// Sort labels are localized via sources.items.* keys; the backend label is
	// the fallback for sorts added without an i18n entry yet.
	const sortTabs = $derived<UrlTab[]>(
		(data.itemSorts ?? []).map((s) => ({ value: s.value, label: t(`sources.items.${s.value}`, s.label) }))
	);

	// Entity-type filter pills (v2 design: "the detail page groups or filters
	// by entity_type"). Options come from /sources/meta (Rule 16); labels fall
	// back to the backend-provided label when no i18n key covers the type.
	function entityTypePillLabel(value: string): string {
		const key = `sources.items.${value === 'wiki_page' ? 'wiki' : value === 'scholarship' ? 'scholarships' : value === 'job' ? 'jobs' : value}`;
		const backend = data.entityTypes?.find((e) => e.value === value)?.label ?? value.replace(/_/g, ' ');
		return t(key, backend);
	}

	const title = $derived(source.name);

	function setQuery(key: string, value: string) {
		const url = new URL(page.url);
		if (value === '') {
			url.searchParams.delete(key);
		} else {
			url.searchParams.set(key, value);
		}
		if (key !== 'sort') url.searchParams.delete('page_offset');
		goto(url, { keepFocus: true });
	}

	// Item-card badges reuse the same localized label as the filter pills —
	// the raw backend label previously rendered English ("Scholarships") next
	// to a localized pill ("Bourses") on the same screen.
	function entityTypeLabel(type: string) {
		return entityTypePillLabel(type);
	}

	// Section 26 — self-referencing canonical on the clean slug URL (the
	// backend resolves UUIDs too, but the load() 301 already canonicalizes).
	const canonical = $derived(`${page.url.origin}/sources/${source.slug}`);

	// BreadcrumbList JSON-LD (Section 26 — all pages with hierarchy).
	const breadcrumbLd = $derived(
		JSON.stringify(
			breadcrumbJsonLd([
				{ name: 'Home', path: page.url.origin },
				{ name: t('sources.heading', 'Sources'), path: `${page.url.origin}/sources` },
				{ name: source.name, path: canonical }
			])
		)
	);

	const linkableKeywords = $derived(
		(source.keywords ?? []).filter((kw) => source.tag_slugs?.[kw] !== undefined)
	);
	const visibleKeywords = $derived(linkableKeywords.slice(0, 4));

	function onEdit(updated: Source) {
		source = updated;
	}
</script>

<svelte:head>
	<title>{title} — {t('sources.title', 'Sources — {site}')}</title>
	<meta name="description" content={source.description ?? source.domain} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={source.description ?? source.domain} />
	<meta property="og:type" content="profile" />
	<link rel="canonical" href={canonical} />
	<meta property="og:url" content={canonical} />
	{@html safeJsonLdScript(breadcrumbLd)}
</svelte:head>

<div class="mx-auto w-full max-w-3xl flex flex-col gap-4 p-4">
	<div class="flex items-start gap-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		{#if source.logo_url && !logoError}
			<img
				src={remoteImageUrl(source.logo_url)}
				alt={source.name}
				class="h-16 w-16 shrink-0 rounded-md object-cover"
				loading="lazy"
				onerror={() => { logoError = true; }}
			/>
		{:else}
			<span class="grid h-16 w-16 shrink-0 place-items-center rounded-md bg-[var(--color-primary)] text-2xl font-bold text-[var(--color-primary-contrast)]">
				{source.name.charAt(0).toUpperCase()}
			</span>
		{/if}

		<div class="flex flex-1 flex-col gap-1">
			<div class="flex items-start justify-between gap-3">
				<div class="flex flex-wrap items-center gap-2">
					<h1 class="text-lg font-bold">{source.name}</h1>
					{#if source.trust_level > 0}
						<span class="rounded bg-[var(--color-primary)]/10 px-1.5 py-0.5 text-xs font-medium text-[var(--color-primary)]">
							{sourceTrustLabel(source.trust_level)}
						</span>
					{/if}
				</div>

				<div class="flex items-center gap-2">
					<FollowButton
						entityType="source"
						entityId={source.id}
						entitySlug={source.slug}
						initiallyFollowing={source.is_following ?? false}
						onFollowerCountChange={(count) => { followerCount = count; }}
					/>
					<SourceActions
						source={source}
						isBlocked={isBlocked}
						showDelete={true}
						onBlockChange={(v) => { isBlocked = v; }}
						onEdit={onEdit}
					/>
				</div>
			</div>
			<a
				href={source.source_url || (source.domain ? `https://${source.domain}` : '#')}
				target="_blank"
				rel="noopener noreferrer"
				class="inline-flex items-center gap-1 text-sm text-[var(--color-text-muted)] hover:underline"
			>
				<Globe class="h-3.5 w-3.5" />
				{source.domain}
				<ExternalLink class="h-3 w-3 rtl:-scale-x-100" />
			</a>
			{#if source.description}
				<p class="text-sm text-[var(--color-text-secondary)]">{source.description}</p>
			{/if}

			{#if visibleKeywords.length > 0}
				<div class="mt-1 flex flex-wrap items-center gap-1.5">
					<Tag class="h-3.5 w-3.5 text-[var(--color-text-muted)]" />
					{#each visibleKeywords as kw (kw)}
						{@const tagSlug = source.tag_slugs?.[kw] ?? ''}
						<a
							href="/tags/{tagSlug}"
							class="rounded bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-xs text-[var(--color-text-subtle)] transition hover:bg-[var(--color-primary)]/10 hover:text-[var(--color-primary)]"
						>
							{kw}
						</a>
					{/each}
				</div>
			{/if}

			{#if source.linked_entity}
				<a
					href={source.linked_entity.href}
					class="mt-1 inline-flex items-center gap-1 text-sm text-[var(--color-primary)] hover:underline"
				>
					{t('sources.view_profile', 'View {type} profile').replace('{type}', source.linked_entity.type)}
					<ExternalLink class="h-3 w-3 rtl:-scale-x-100" />
				</a>
			{/if}

			<!-- §106 — video channels have no public profile; the claim/report
			     entry point lives here on the source page instead. -->
			{#if source.source_type === 'video_channel' && source.channel_username}
				<a
					href="/u/{source.channel_username}/claim"
					class="mt-1 inline-flex items-center gap-1 text-sm text-[var(--color-primary)] hover:underline"
				>
					{t('video.claim_or_report', 'Is this your channel? Claim or report it')}
				</a>
			{/if}

			<div class="mt-2 flex items-center gap-4 text-sm text-[var(--color-text-muted)]">
				<span class="inline-flex items-center gap-1">
					<Users class="h-4 w-4" />
					{followerCount} {t('sources.followers', 'followers')}
				</span>
				<span class="inline-flex items-center gap-1">
					<FileText class="h-4 w-4" />
					{source.post_count} {t('sources.posts', 'posts')}
				</span>
			</div>

			<!-- About: provenance metadata from the detail endpoint (v2 spec
			     "About" tab — license, attribution, crawl cadence, last crawl). -->
			{#if source.source_license || source.attribution_required || source.crawl_frequency || source.last_crawled_at}
				<details class="mt-2 text-xs text-[var(--color-text-muted)]">
					<summary class="cursor-pointer font-medium hover:text-[var(--color-text)]">
						{t('sources.about', 'About this source')}
					</summary>
					<dl class="mt-1.5 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">
						{#if source.source_license}
							<dt>{t('sources.about.license', 'License')}</dt>
							<dd>{source.source_license}</dd>
						{/if}
						{#if source.attribution_required}
							<dt>{t('sources.about.attribution', 'Attribution')}</dt>
							<dd>{t('sources.about.attribution_required', 'Required')}</dd>
						{/if}
						{#if source.crawl_frequency && source.crawl_frequency !== 'manual'}
							<dt>{t('sources.about.crawl_frequency', 'Updated')}</dt>
							<dd class="capitalize">{source.crawl_frequency}</dd>
						{/if}
						{#if source.last_crawled_at}
							<dt>{t('sources.about.last_crawled', 'Last checked')}</dt>
							<dd>{new Date(source.last_crawled_at).toLocaleDateString()}</dd>
						{/if}
					</dl>
				</details>
			{/if}
		</div>
	</div>

	<div class="flex flex-col gap-3">
		<UrlTabs tabs={sortTabs} value={data.sort} onValueChange={(v) => setQuery('sort', v)} />

		{#if (data.entityTypes ?? []).length > 0}
			<div class="flex flex-wrap items-center gap-1.5" role="group" aria-label={t('sources.items.filter_label', 'Filter by type')}>
				<button
					type="button"
					onclick={() => setQuery('entity_type', '')}
					aria-pressed={!data.entityType}
					class="rounded-full px-3 py-1 text-xs font-medium transition-colors {!data.entityType
						? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
						: 'bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)] hover:text-[var(--color-primary)]'}"
				>
					{t('sources.items.all', 'All')}
				</button>
				{#each data.entityTypes as et (et.value)}
					<button
						type="button"
						onclick={() => setQuery('entity_type', data.entityType === et.value ? '' : et.value)}
						aria-pressed={data.entityType === et.value}
						class="rounded-full px-3 py-1 text-xs font-medium transition-colors {data.entityType === et.value
							? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
							: 'bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)] hover:text-[var(--color-primary)]'}"
					>
						{entityTypePillLabel(et.value)}
					</button>
				{/each}
			</div>
		{/if}
	</div>

	{#if isBlocked}
		<div class="flex flex-col items-center gap-3 py-8">
			<p class="text-sm text-[var(--color-text-muted)]">
				{t('sources.items.blocked', 'You have blocked this source.')}
			</p>
			<SourceBlockButton slug={source.slug} initial={true} onChange={(v) => { isBlocked = v; }} />
		</div>
	{:else if items.length === 0}
		<p class="py-8 text-center text-sm text-[var(--color-text-muted)]">
			{t('sources.items.empty', 'No items from this source yet.')}
		</p>
	{:else}
		<div class="grid grid-cols-1 gap-3 md:grid-cols-2" role="list">
			{#each items as item (item.id)}
				<SourceDailyCard
					{item}
					{source}
					sourceName={source.name}
					sourceLogo={source.logo_url}
					sourceId={source.id}
					sourceSlug={source.slug}
					entityLabel={entityTypeLabel(item.entity_type)}
				/>
			{/each}
		</div>
		{#if hasMore}
			<button
				type="button"
				onclick={loadMore}
				disabled={loadingMore}
				class="mt-3 w-full rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] py-2 text-sm font-medium transition hover:bg-[var(--color-surface-hover)] disabled:opacity-50"
			>
				{loadingMore ? t('sources.items.loading', 'Loading…') : t('sources.items.load_more', 'Load more')}
			</button>
		{/if}
	{/if}
</div>
