<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { Hash, ArrowLeft, Clock, Eye, UserPlus, UserCheck } from 'lucide-svelte';
	import SetSidebar from '$lib/components/shell/SetSidebar.svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const tag = $derived(data.tag);
	const authed = $derived(!!page.data.user);

	const title = $derived(t('tag.title', '#{name} — JapaConnect').replace('{name}', tag.name));
	const description = $derived(
		t('tag.description', 'Content tagged with #{name} on JapaConnect.').replace('{name}', tag.name)
	);
	const followerLabel = $derived(
		t('tag.followers', '{n} followers').replace('{n}', String(tag.follower_count))
	);

	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'CollectionPage',
			name: `#${tag.name}`,
			description,
			url: page.url.href
		})
	);

	// Follow state for the tag. Hydrated server-side from
	// /users/{tag.id}/follow-status?entity_type=tag so the button reflects
	// the caller's actual follow state on SSR load. The backend exposes
	// POST/DELETE /tags/{slug}/follow (auth-gated).
	let following = $state(untrack(() => data.initiallyFollowing ?? false));
	let pending = $state(false);

	async function toggleFollow() {
		if (!authed || pending) return;
		const was = following;
		following = !was;
		pending = true;
		const res = was
			? await api.delete<{ following: boolean }>(`/tags/${tag.slug}/follow`)
			: await api.post<{ following: boolean }>(`/tags/${tag.slug}/follow`);
		if (res.error) {
			following = was;
		} else if (res.data && typeof res.data.following === 'boolean') {
			following = res.data.following;
		}
		pending = false;
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
	{@html `<script type="application/ld+json">${jsonLd}</script>`}
</svelte:head>

<SetSidebar>
	<!-- Route-specific right sidebar: tag card with follow action -->
	<section
		class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
	>
		<div class="flex items-center gap-3">
			{#if tag.logo_url}
				<img
					src={tag.logo_url}
					alt={tag.name}
					class="h-12 w-12 shrink-0 rounded-[var(--radius)] border border-[var(--color-border)] object-cover"
				/>
			{:else}
				<span
					class="grid h-12 w-12 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)]"
				>
					<Hash size={24} />
				</span>
			{/if}
			<div class="min-w-0 flex-1">
				<h2 class="truncate text-base font-semibold text-[var(--color-text)]">#{tag.name}</h2>
				<p class="text-xs text-[var(--color-muted)]">{followerLabel}</p>
			</div>
		</div>

		{#if tag.description}
			<p class="mt-3 text-sm text-[var(--color-text-subtle)]">{tag.description}</p>
		{/if}

		{#if authed}
			<button
				type="button"
				onclick={toggleFollow}
				disabled={pending}
				aria-busy={pending}
				class="mt-4 flex w-full items-center justify-center gap-1.5 rounded-[var(--radius)] px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50 {following
					? 'border border-[var(--color-border)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'
					: 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]'}"
			>
				{#if following}
					<UserCheck size={16} />
					{t('tag.following', 'Following')}
				{:else}
					<UserPlus size={16} />
					{t('tag.follow', 'Follow tag')}
				{/if}
			</button>
		{/if}
	</section>
</SetSidebar>

<!-- Center column: tag header + content grid -->
<div class="mx-auto w-full max-w-full">
	<div class="border-b border-[var(--color-border)] px-4 py-4">
		<a
			href="/discover?tab=tags"
			class="mb-2 inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
		>
			<ArrowLeft size={16} /> {t('tag.back', 'All tags')}
		</a>
		<div class="flex items-center gap-3">
			{#if tag.logo_url}
				<img
					src={tag.logo_url}
					alt={tag.name}
					class="h-12 w-12 shrink-0 rounded-[var(--radius)] border border-[var(--color-border)] object-cover"
				/>
			{:else}
				<span
					class="grid h-12 w-12 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)]"
				>
					<Hash size={24} />
				</span>
			{/if}
			<div class="min-w-0 flex-1">
				<h1 class="truncate text-xl font-semibold text-[var(--color-text)]">#{tag.name}</h1>
				<p class="text-xs text-[var(--color-muted)]">{followerLabel}</p>
			</div>
		</div>
		{#if tag.description}
			<p class="mt-3 text-sm text-[var(--color-text-subtle)]">{tag.description}</p>
		{/if}
	</div>

	<div class="p-4">
		{#if data.loadError}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">
				{t('tag.load_error', 'Failed to load tagged content. Please try again later.')}
			</p>
		{:else if data.items.length === 0}
			<EmptyState icon={Hash} title={t('tag.empty', 'No content with this tag yet.')} />
		{:else}
			<!-- Magazine card grid (Ghost.org style, same as blog list) -->
			<div class="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
				{#each data.items as post (post.id)}
					<a
						href={post.content_type === 'blog_post' ? `/blog/${post.slug}` : `/content/${post.slug}`}
						class="group flex flex-col overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] transition-all hover:border-[var(--color-primary)]/40 hover:shadow-lg"
					>
						{#if post.featured_image_url}
							<div class="aspect-[16/9] overflow-hidden">
								<img
									src={post.featured_image_url}
									alt={post.title}
									loading="lazy"
									class="h-full w-full object-cover transition-transform duration-300 group-hover:scale-105"
								/>
							</div>
						{:else}
							<div
								class="aspect-[16/9] bg-gradient-to-br from-[var(--color-primary)]/12 via-[var(--color-surface-subtle)] to-[var(--color-surface-subtle)]"
							></div>
						{/if}
						<div class="flex flex-1 flex-col p-5">
							<span
								class="mb-2 text-xs font-medium uppercase tracking-wide text-[var(--color-primary)]"
							>
								{t('tag.card_label', 'Tagged post')}
							</span>
							<h2
								class="text-xl font-semibold leading-snug text-[var(--color-text)] group-hover:text-[var(--color-primary)]"
								style="font-family: var(--font-serif);"
							>
								{post.title}
							</h2>
							<p class="mt-2 line-clamp-3 flex-1 text-sm text-[var(--color-text-subtle)]">
								{post.body.slice(0, 160)}
							</p>
							<div class="mt-4 flex items-center gap-3 text-xs text-[var(--color-muted)]">
								{#if post.read_time_minutes > 0}
									<span class="flex items-center gap-1">
										<Clock size={12} /> {post.read_time_minutes} {t('tag.min_read', 'min read')}
									</span>
								{/if}
								<span class="flex items-center gap-1">
									<Eye size={12} /> {post.view_count}
								</span>
							</div>
						</div>
					</a>
				{/each}
			</div>
		{/if}
	</div>
</div>
