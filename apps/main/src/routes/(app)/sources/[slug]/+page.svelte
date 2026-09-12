<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { Globe, ExternalLink, Users, FileText, Tag } from 'lucide-svelte';
	import { FollowButton, UrlTabs, type UrlTab } from '$lib/components/ui';
	import SourceActions from '$lib/components/source/SourceActions.svelte';
	import { api } from '$lib/api/client';
	import type { PageData } from './$types';
	import type { SourceItemEnvelope } from '$lib/types/sources';

	let { data }: { data: PageData } = $props();

	const source = $derived(data.source);
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
		const res = await api.get<SourceItemEnvelope>(`/sources/${source.slug}/items?${params.toString()}`);
		if (res.data) {
			items = [...items, ...res.data.items];
			feedMeta = { ...feedMeta, ...res.data.meta };
		}
		loadingMore = false;
	}

	const sortTabs = $derived<UrlTab[]>(
		(data.itemSorts ?? []).map((s) => ({ value: s.value, label: s.label }))
	);

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

	function entityTypeLabel(type: string) {
		return data.entityTypes?.find((e) => e.value === type)?.label ?? type.replace(/_/g, ' ');
	}
</script>

<svelte:head>
	<title>{title} — {t('sources.title', 'Sources — JapaConnect')}</title>
	<meta name="description" content={source.description ?? source.domain} />
</svelte:head>

<div class="flex flex-col gap-4 p-4">
	<div class="flex items-start gap-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		{#if source.logo_url && !logoError}
			<img
				src={source.logo_url}
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
							{t('sources.trust.level', 'Trust {level}').replace('{level}', String(source.trust_level))}
						</span>
					{/if}
				</div>

				{#if page.data.user}
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
							onBlockChange={(v) => { isBlocked = v; }}
						/>
					</div>
				{/if}
			</div>
			<a
				href={source.source_url || (source.domain ? `https://${source.domain}` : '#')}
				target="_blank"
				rel="noopener noreferrer"
				class="inline-flex items-center gap-1 text-sm text-[var(--color-text-muted)] hover:underline"
			>
				<Globe class="h-3.5 w-3.5" />
				{source.domain}
				<ExternalLink class="h-3 w-3" />
			</a>
			{#if source.description}
				<p class="text-sm text-[var(--color-text-secondary)]">{source.description}</p>
			{/if}

			{#if source.keywords && source.keywords.length > 0}
				<div class="mt-1 flex flex-wrap items-center gap-1.5">
					<Tag class="h-3.5 w-3.5 text-[var(--color-text-muted)]" />
					{#each source.keywords as kw (kw)}
						<span class="rounded bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-xs text-[var(--color-text-subtle)]">
							{kw}
						</span>
					{/each}
				</div>
			{/if}

			{#if source.linked_entity}
				<a
					href={source.linked_entity.href}
					class="mt-1 inline-flex items-center gap-1 text-sm text-[var(--color-primary)] hover:underline"
				>
					{t('sources.view_profile', 'View {type} profile').replace('{type}', source.linked_entity.type)}
					<ExternalLink class="h-3 w-3" />
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

			{#if !page.data.user}
				<p class="mt-2 text-xs text-[var(--color-text-muted)]">
					{t('sources.sign_in_to_follow', 'Sign in to follow this source.')}
				</p>
			{/if}
		</div>
	</div>

	<div class="flex flex-col gap-3">
		<UrlTabs tabs={sortTabs} value={data.sort} onValueChange={(v) => setQuery('sort', v)} />
	</div>

	{#if isBlocked}
		<p class="py-8 text-center text-sm text-[var(--color-text-muted)]">
			{t('sources.items.blocked', 'You have blocked this source.')}
		</p>
	{:else if items.length === 0}
		<p class="py-8 text-center text-sm text-[var(--color-text-muted)]">
			{t('sources.items.empty', 'No items from this source yet.')}
		</p>
	{:else}
		<div class="flex flex-col gap-3" role="list">
			{#each items as item (item.id)}
				<article
					class="group relative flex flex-col gap-1 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-shadow hover:shadow-md"
				>
					<a
						href={item.href}
						class="absolute inset-0 z-0 rounded-[var(--radius-card)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
						aria-label={item.title}
					>
						<span class="sr-only">{item.title}</span>
					</a>
					<div class="pointer-events-none relative z-10 flex flex-col gap-1">
						<div class="flex items-center gap-2">
							<span class="rounded bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10.5px] font-semibold uppercase tracking-wide text-[var(--color-text-subtle)]">
								{entityTypeLabel(item.entity_type)}
							</span>
						</div>
						<h3 class="font-semibold group-hover:text-[var(--color-primary)]">{item.title}</h3>
						{#if item.excerpt}
							<p class="text-sm text-[var(--color-text-secondary)] line-clamp-2">{item.excerpt}</p>
						{/if}
						<div class="mt-1 flex items-center gap-3 text-xs text-[var(--color-text-muted)]">
							<span>{new Date(item.published_at).toLocaleDateString()}</span>
							{#if item.original_url}
								<a
									href={item.original_url}
									target="_blank"
									rel="noopener noreferrer"
									class="pointer-events-auto relative z-20 inline-flex items-center gap-0.5 hover:underline"
									aria-label={t('sources.original_label', 'Original source')}
								>
									{t('sources.original', 'Original')}
									<ExternalLink class="h-3 w-3" />
								</a>
							{/if}
						</div>
					</div>
				</article>
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
