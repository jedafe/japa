<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { Globe, ExternalLink, Users, FileText } from 'lucide-svelte';
	import { FollowButton, UrlTabs, type UrlTab } from '$lib/components/ui';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const source = $derived(data.source);
	const feed = $derived(data.feed);

	const sortTabs = $derived<UrlTab[]>([
		{ value: 'latest', label: t('sources.items.latest', 'Latest') },
		{ value: 'popular', label: t('sources.items.popular', 'Popular') },
		{ value: 'discussed', label: t('sources.items.discussed', 'Discussed') },
		{ value: 'upvoted', label: t('sources.items.upvoted', 'Upvoted') }
	]);

	const entityTabs = $derived<UrlTab[]>([
		{ value: '', label: t('sources.items.all', 'All') },
		{ value: 'scholarship', label: t('sources.items.scholarships', 'Scholarships') },
		{ value: 'job', label: t('sources.items.jobs', 'Jobs') },
		{ value: 'remittance', label: t('sources.items.remittance', 'Remittance') },
		{ value: 'news', label: t('sources.items.news', 'News') },
		{ value: 'wiki_page', label: t('sources.items.wiki', 'Wiki') }
	]);

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
		return t(`sources.items.${type}`, type.replace(/_/g, ' '));
	}
</script>

<svelte:head>
	<title>{title} — {t('sources.title', 'Sources — JapaConnect')}</title>
	<meta name="description" content={source.description ?? source.domain} />
</svelte:head>

<div class="flex flex-col gap-4 p-4">
	<div class="flex items-start gap-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		{#if source.logo_url}
			<img
				src={source.logo_url}
				alt=""
				class="h-16 w-16 rounded-md object-cover"
				loading="lazy"
			/>
		{:else}
			<span class="grid h-16 w-16 place-items-center rounded-md bg-[var(--color-primary)] text-2xl font-bold text-[var(--color-primary-contrast)]">
				{source.name.charAt(0).toUpperCase()}
			</span>
		{/if}

		<div class="flex flex-1 flex-col gap-1">
			<div class="flex items-center gap-2">
				<h1 class="text-lg font-bold">{source.name}</h1>
				{#if source.trust_level > 0}
					<span class="rounded bg-[var(--color-primary)]/10 px-1.5 py-0.5 text-xs font-medium text-[var(--color-primary)]">
						{t('sources.trust.level', 'Trust {level}').replace('{level}', String(source.trust_level))}
					</span>
				{/if}
			</div>
			<a
				href="https://{source.domain}"
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
					{source.follower_count} {t('sources.followers', 'followers')}
				</span>
				<span class="inline-flex items-center gap-1">
					<FileText class="h-4 w-4" />
					{source.post_count} {t('sources.posts', 'posts')}
				</span>
			</div>

			{#if page.data.user}
				<div class="mt-3">
					<FollowButton
						entityType="source"
						entityId={source.id}
						entitySlug={source.slug}
						initiallyFollowing={source.is_following ?? false}
					/>
				</div>
			{:else}
				<p class="mt-2 text-xs text-[var(--color-text-muted)]">
					{t('sources.sign_in_to_follow', 'Sign in to follow this source.')}
				</p>
			{/if}
		</div>
	</div>

	<div class="flex flex-col gap-3">
		<UrlTabs tabs={entityTabs} value={data.entityType} onValueChange={(v) => setQuery('entity_type', v)} />
		<UrlTabs tabs={sortTabs} value={data.sort} onValueChange={(v) => setQuery('sort', v)} />
	</div>

	{#if feed.items.length === 0}
		<p class="py-8 text-center text-sm text-[var(--color-text-muted)]">
			{t('sources.items.empty', 'No items from this source yet.')}
		</p>
	{:else}
		<div class="flex flex-col gap-3">
			{#each feed.items as item (item.id)}
				<div
					role="link"
					tabindex="0"
					class="group flex flex-col gap-1 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-shadow hover:shadow-md cursor-pointer"
					onclick={() => goto(item.href)}
					onkeydown={(e) => { if (e.key === 'Enter') goto(item.href); }}
				>
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
						<a
							href={item.original_url}
							target="_blank"
							rel="noopener noreferrer"
							class="inline-flex items-center gap-0.5 hover:underline"
							onclick={(e) => e.stopPropagation()}
						>
							{t('sources.original', 'Original')}
							<ExternalLink class="h-3 w-3" />
						</a>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
