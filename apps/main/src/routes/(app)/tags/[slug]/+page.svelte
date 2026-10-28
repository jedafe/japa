<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto, invalidateAll } from '$app/navigation';
	import { t } from '$lib/i18n';
	import UserHoverCard from '$lib/components/ui/UserHoverCard.svelte';
	import { Modal } from '$lib/components/ui';
	import { Hash, ArrowLeft, UserPlus, UserCheck, ExternalLink, BookOpen, Pencil, X, Loader2, Image as ImageIcon, Search, UserCircle, ShieldCheck } from 'lucide-svelte';
	import SetSidebar from '$lib/components/shell/SetSidebar.svelte';
	import { EmptyState, UrlTabs, type UrlTab } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import MediaLibraryModal from '$lib/components/media/MediaLibraryModal.svelte';
	import type { MediaAsset, FeedItem } from '$lib/types';
	import type { SearchResult } from '$lib/types/search';
	import FeedCard from '$lib/components/feed/FeedCard.svelte';
	import type { PageData } from './$types';
	import { sanitizeContentHtml, safeJsonLdScript } from '$lib/utils/safeHtml';

	let { data }: { data: PageData } = $props();

	// Map ContentArticle → FeedItem so tagged posts render through FeedCard.
	function toFeedItem(post: typeof data.items[number]): FeedItem {
		return {
			id: post.id,
			title: post.title,
			slug: post.slug,
			content_type: post.content_type,
			author_id: post.author_id ?? '',
			author_name: post.author_name,
			author_avatar_url: post.author_avatar_url,
			space_id: post.space_id,
			space_name: post.space_name,
			space_slug: post.space_slug,
			excerpt: post.excerpt,
			created_at: post.created_at,
			view_count: post.view_count,
			comment_count: post.comment_count,
			reading_time_minutes: post.read_time_minutes,
			cover_image_url: post.featured_image_url,
			tags: post.tags
		};
	}

	// Cursor-based pagination state (Section 14). The initial page is SSR'd;
	// "Load More" fetches the next page client-side and appends. Uses a
	// composite (created_at, id) cursor to avoid skipping items that share
	// the same created_at timestamp.
	let extraItems: typeof data.items = $state([]);
	let nextCursor = $state(untrack(() => data.nextCursor ?? ''));
	let nextCursorId = $state(untrack(() => data.nextCursorId ?? ''));
	let loadingMore = $state(false);

	// Reset client-side pagination whenever load() re-runs. data.items is a
	// fresh array per load, so this covers tag-to-tag navigation and ?cursor=
	// SSR loads — without it, tag B would inherit tag A's appended items and
	// cursor after client-side navigation (the component is reused).
	$effect(() => {
		void data.items;
		void data.nextCursor;
		void data.nextCursorId;
		extraItems = [];
		nextCursor = data.nextCursor ?? '';
		nextCursorId = data.nextCursorId ?? '';
	});

	const allItems = $derived([...data.items, ...extraItems]);
	const allFeedItems = $derived(allItems.map(toFeedItem));

	async function loadMore() {
		if (!nextCursor || loadingMore) return;
		loadingMore = true;
		try {
			let url = `/tags/${tag.slug}/content?page_size=${data.pageSize}&cursor=${encodeURIComponent(nextCursor)}`;
			if (nextCursorId) url += `&cursor_id=${encodeURIComponent(nextCursorId)}`;
			const res = await api.get<{ data: typeof data.items }>(url);
			if (res.error || !res.data) {
				loadingMore = false;
				return;
			}
			const fetched = res.data.data ?? [];
			if (fetched.length > 0) {
				extraItems = [...extraItems, ...fetched];
			}
			// Advance the composite cursor to the last fetched item's
			// (created_at, id); if fewer than a full page came back, there
			// are no more pages.
			if (fetched.length >= data.pageSize && fetched.length > 0) {
				const last = fetched[fetched.length - 1];
				nextCursor = last?.created_at ?? '';
				nextCursorId = last?.id ?? '';
			} else {
				nextCursor = '';
				nextCursorId = '';
			}
		} catch {
			// leave nextCursor so the user can retry
		} finally {
			loadingMore = false;
		}
	}

	const tag = $derived(data.tag);
	const authed = $derived(!!page.data.user);
	const canManage = $derived(!!data.canManage);
	const fieldDefs = $derived(data.fieldDefs ?? []);
	const fieldValues = $derived(tag.field_values ?? {});

	// Tab state for the tag hub: content (default), search, followers, and
	// moderators (only when the tag has at least one assigned moderator —
	// an always-visible empty tab would be dead weight on unmoderated tags).
	const tab = $derived(data.tab ?? 'content');
	const moderators = $derived(data.moderators ?? []);
	const tabs = $derived<UrlTab[]>([
		{ value: 'content', label: t('tag.tab_content', 'Content') },
		{ value: 'search', label: t('tag.tab_search', 'Search') },
		{ value: 'followers', label: t('tag.tab_followers', 'Followers') },
		...(moderators.length > 0
			? [{ value: 'moderators', label: t('tag.tab_moderators', 'Moderators') }]
			: [])
	]);

	// Search state. Use untrack() so the $state() initializer intentionally
	// reads the load data once without a reactivity warning.
	let searchInput = $state(untrack(() => data.query ?? ''));
	let searching = $state(false);
	// Initialized from load() so SSR renders results directly (no
	// empty-state flash before the sync effect runs on hydration).
	let searchResults: SearchResult[] = $state(
		untrack(() => (data.searchResults as SearchResult[] | undefined) ?? [])
	);

	function runSearch() {
		const q = searchInput.trim();
		if (!q) {
			goto(`?tab=search`);
			return;
		}
		goto(`?tab=search&q=${encodeURIComponent(q)}`);
	}

	// Sync search state from load() data. data.tab/data.query/data.searchResults
	// change on every ?tab=/?q= navigation (the page component is reused for
	// query-param navigations), so this must re-run whenever they change —
	// latching once leaves a submitted search stuck on "No matching posts".
	$effect(() => {
		void data.tab;
		void data.query;
		void data.searchResults;
		if (data.tab === 'search') {
			searchResults = (data.searchResults as SearchResult[] | undefined) ?? [];
			searchInput = data.query ?? '';
		}
	});

	// Helpers for search results and follower lists.
	function searchResultLink(r: SearchResult): string {
		const item = r.item;
		if (item.url) return item.url;
		if (item.content_type === 'blog_post' && item.slug) return `/blog/${item.slug}`;
		if (item.content_type === 'forum_thread' && item.slug) return `/forum/${item.slug}`;
		if (item.slug) return `/content/${item.slug}`;
		return '#';
	}

	const initials = (name: string) => name?.charAt(0)?.toUpperCase() ?? '?';

	// Filter to field defs that have a non-empty value in field_values and
	// are flagged for frontend display (show_in_frontend defaults to true).
	const visibleFields = $derived(
		fieldDefs.filter((f) => {
			if (f.show_in_frontend === false) return false;
			const v = fieldValues[f.key];
			return v != null && String(v).trim() !== '';
		})
	);

	const title = $derived(t('tag.title', '#{name} — {site}').replace('{name}', tag.name));
	const description = $derived(
		t('tag.description', 'Content tagged with #{name} on {site}.').replace('{name}', tag.name)
	);
	// Section 26 — self-referencing canonical on the clean slug URL (never
	// the query-string form) so filtered/paginated hits consolidate.
	const canonical = $derived(`${page.url.origin}/tags/${tag.slug}`);

	const followerLabel = $derived(
		t('tag.followers', '{n} followers').replace('{n}', String(tag.follower_count))
	);

	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'CollectionPage',
			name: `#${tag.name}`,
			description,
			url: canonical
		})
	);

	// Follow state for the tag. Hydrated server-side from
	// /users/{tag.id}/follow-status?entity_type=tag so the button reflects
	// the caller's actual follow state on SSR load. The backend exposes
	// POST/DELETE /tags/{slug}/follow (auth-gated). Re-synced on load() data
	// change so tag-to-tag navigation shows the new tag's follow state.
	let following = $state(untrack(() => data.initiallyFollowing ?? false));
	let pending = $state(false);

	$effect(() => {
		following = data.initiallyFollowing ?? false;
	});

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

	// ---- Edit modal state ----
	// Only users with tags.manage permission see the edit button. The form
	// submits to PATCH /admin/tags/{id} — the same admin endpoint the admin
	// panel uses. The backend enforces the permission server-side.
	let editOpen = $state(false);
	let editName = $state('');
	let editDescription = $state('');
	let editRules = $state('');
	let editColor = $state('');
	let editLogoMediaId = $state('');
	let editLogoUrl = $state('');
	let editCoverMediaId = $state('');
	let editCoverUrl = $state('');
	let editFieldValues = $state<Record<string, string>>({});
	let editSaving = $state(false);
	let editError = $state('');
	let logoPickerOpen = $state(false);
	let coverPickerOpen = $state(false);

	function openEdit() {
		editName = tag.name;
		editDescription = tag.description ?? '';
		editRules = tag.rules ?? '';
		editColor = tag.color ?? '';
		editLogoMediaId = tag.logo_media_id ?? '';
		editLogoUrl = tag.logo_url ?? '';
		editCoverMediaId = tag.cover_media_id ?? '';
		editCoverUrl = tag.cover_url ?? '';
		const fv = tag.field_values ?? {};
		const stringFv: Record<string, string> = {};
		for (const [k, v] of Object.entries(fv)) {
			stringFv[k] = v == null ? '' : String(v);
		}
		editFieldValues = stringFv;
		editError = '';
		editOpen = true;
	}

	function handleLogoSelect(media: MediaAsset) {
		editLogoMediaId = media.id;
		editLogoUrl = media.url ?? '';
		logoPickerOpen = false;
	}

	function handleCoverSelect(media: MediaAsset) {
		editCoverMediaId = media.id;
		editCoverUrl = media.url ?? '';
		coverPickerOpen = false;
	}

	async function submitEdit() {
		if (!editName.trim() || editSaving) return;
		editSaving = true;
		editError = '';
		const body: Record<string, unknown> = {
			name: editName.trim(),
			// Always send the current slug — the backend derives a slug from
			// the name when it's omitted, which would silently rewrite a
			// custom slug on any unrelated edit.
			slug: tag.slug,
			description: editDescription.trim() || undefined,
			rules: editRules.trim() || undefined,
			color: editColor.trim() || undefined,
			logo_media_id: editLogoMediaId || undefined,
			cover_media_id: editCoverMediaId || undefined
		};
		if (fieldDefs.length > 0) {
			// Merge over the stored field_values rather than building a fresh
			// object — values for keys not covered by the loaded field defs
			// (or defs that failed to load) must not be dropped on save.
			const fv: Record<string, unknown> = { ...(tag.field_values ?? {}) };
			for (const def of fieldDefs) {
				const val = editFieldValues[def.key]?.trim();
				if (val) {
					fv[def.key] = val;
				} else {
					delete fv[def.key];
				}
			}
			body.field_values = fv;
		}
		const res = await api.patch(`/admin/tags/${tag.id}`, body);
		if (res.error) {
			editError = res.error.message;
		} else {
			editOpen = false;
			// Re-fetch the tag data so the page reflects the updated settings.
			// invalidate() takes a URL relative to this app's origin, but the
			// load fetches the backend origin — a relative path would never
			// match. invalidateAll() re-runs the page load instead.
			await invalidateAll();
		}
		editSaving = false;
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<link rel="canonical" href={canonical} />
	<meta property="og:url" content={canonical} />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
	<!-- Ref: api-routes-catalog.md — GET /tags/{slug}/feed.xml (public RSS) -->
	<link
		rel="alternate"
		type="application/rss+xml"
		title="{title} RSS"
		href="/tags/{tag.slug}/feed.xml"
	/>
	{@html safeJsonLdScript(jsonLd)}
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
<div class="mx-auto w-full max-w-full" style="--page-gutter: 1rem">
	<!-- Cover image hero -->
	{#if tag.cover_url}
		<div class="relative mx-auto h-32 w-full max-w-3xl overflow-hidden border-b border-[var(--color-border)] sm:h-40">
			<img src={tag.cover_url} alt={tag.name} class="h-full w-full object-cover" />
		</div>
	{/if}

	<div class="border-b border-[var(--color-border)] px-4 py-4" style="margin-inline: max(1rem, calc((100% - 48rem)/2))">
		<a
			href="/discover?tab=tags"
			class="mb-2 inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
		>
			<ArrowLeft class="rtl:-scale-x-100" size={16} /> {t('tag.back', 'All tags')}
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
			{#if canManage}
				<button
					type="button"
					onclick={openEdit}
					class="flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
				>
					<Pencil size={14} />
					{t('tag.edit', 'Edit Tag')}
				</button>
			{/if}
		</div>
		{#if tag.description}
			<p class="mt-3 text-sm text-[var(--color-text-subtle)]">{tag.description}</p>
		{/if}
		{#if tag.rules}
			<div class="mt-4 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
				<p class="mb-1 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
					<BookOpen size={12} /> {t('tag.rules', 'Tag Rules')}
				</p>
				<p class="whitespace-pre-line text-sm text-[var(--color-text-subtle)]">{tag.rules}</p>
			</div>
		{/if}
		{#if visibleFields.length > 0}
			<div class="mt-4 space-y-3">
				{#each visibleFields as field (field.key)}
					<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
						<p class="mb-1 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">{field.label}</p>
						{#if field.field_type === 'url'}
							<a
								href={String(fieldValues[field.key])}
								target="_blank"
								rel="noopener noreferrer"
								class="inline-flex items-center gap-1 text-sm text-[var(--color-primary)] hover:underline"
							>
								{String(fieldValues[field.key])}
								<ExternalLink class="rtl:-scale-x-100" size={12} />
							</a>
						{:else if field.field_type === 'rich_text'}
							<div class="prose prose-sm max-w-none text-sm text-[var(--color-text-subtle)]">
								{@html sanitizeContentHtml(String(fieldValues[field.key]))}
							</div>
						{:else}
							<p class="whitespace-pre-line text-sm text-[var(--color-text-subtle)]">{String(fieldValues[field.key])}</p>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	</div>

	<div class="mx-auto w-full max-w-3xl"><UrlTabs {tabs} value={tab} onValueChange={(v) => goto(`?tab=${v}`)} class="mx-4" /></div>

	<div class="mx-auto w-full max-w-3xl p-4">
		{#if tab === 'search'}
			<!-- Search within this tag (auth required by the backend). -->
			<form class="mb-4 flex gap-2" onsubmit={(e) => { e.preventDefault(); runSearch(); }}>
				<div class="relative min-w-0 flex-1">
					<Search size={16} class="absolute start-3 top-1/2 -translate-y-1/2 text-[var(--color-muted)]" />
					<input
						type="search"
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-2 ps-9 pe-3 text-sm text-[var(--color-text)]"
						placeholder={t('tag.search_placeholder', 'Search posts in this tag…')}
						bind:value={searchInput}
						disabled={!authed}
					/>
				</div>
				<button
					type="submit"
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
					disabled={!authed || searching}
				>
					{t('tag.search_button', 'Search')}
				</button>
			</form>

			{#if data.searchAuthRequired}
				<EmptyState icon={Search} title={t('tag.search_signin', 'Sign in to search this tag')} />
			{:else if !data.query}
				<EmptyState icon={Search} title={t('tag.search_prompt', 'Enter a search term')} />
			{:else if searchResults.length === 0}
				<EmptyState icon={Search} title={t('tag.search_empty', 'No matching posts')} />
			{:else}
				<div class="divide-y divide-[var(--color-border)] overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
					{#each searchResults as r (r.item.id)}
						<a
							href={searchResultLink(r)}
							class="block p-4 transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							<h3 class="font-medium text-[var(--color-text)]">{r.item.title}</h3>
							{#if r.item.excerpt}
								<p class="mt-1 line-clamp-2 text-sm text-[var(--color-text-subtle)]">{r.item.excerpt}</p>
							{/if}
							<span class="mt-2 inline-block text-[10.5px] font-semibold uppercase tracking-wide text-[var(--color-muted)]">
								{r.item.content_type?.replace(/_/g, ' ')}
							</span>
						</a>
					{/each}
				</div>
			{/if}
		{:else if tab === 'followers'}
			{#if data.followersAuthRequired}
				<!-- GET /tags/{slug}/followers is auth-gated — prompt sign-in
				     instead of showing a misleading "no followers" state. -->
				<EmptyState icon={UserCircle} title={t('tag.followers_signin', 'Sign in to see followers')} />
			{:else if data.followers.length === 0}
				<EmptyState icon={UserCircle} title={t('tag.no_followers', 'No followers yet')} />
			{:else}
				<ul class="grid grid-cols-1 gap-x-3 gap-y-0.5 sm:grid-cols-2 lg:grid-cols-3">
					{#each data.followers as f (f.id)}
						<li>
							<UserHoverCard
								id={f.follower_id ?? f.id}
								slug={f.username}
								name={f.display_name}
								avatar={f.avatar_url}
								class="inline-flex w-full min-w-0 max-w-full items-center gap-3 rounded-[var(--radius)] p-2 text-start transition-colors hover:bg-[var(--color-surface-subtle)]"
							>
								{#snippet trigger()}
									{#if f.avatar_url}
										<img src={f.avatar_url} alt={f.display_name} class="h-10 w-10 shrink-0 rounded-full object-cover" loading="lazy" />
									{:else}
										<span class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/10 text-sm font-medium text-[var(--color-primary)]">
											{initials(f.display_name)}
										</span>
									{/if}
									<span class="min-w-0 flex-1">
										<span class="block truncate text-sm font-medium text-[var(--color-text)]">{f.display_name}</span>
										<span class="block truncate text-xs text-[var(--color-muted)]">@{f.username}</span>
									</span>
								{/snippet}
							</UserHoverCard>
						</li>
					{/each}
				</ul>
			{/if}
		{:else if tab === 'moderators'}
			{#if moderators.length === 0}
				<EmptyState icon={ShieldCheck} title={t('tag.no_moderators', 'No moderators assigned')} />
			{:else}
				<ul class="space-y-0.5">
					{#each moderators as m (m.user_id)}
						<li>
							<UserHoverCard
								id={m.user_id}
								slug={m.username}
								name={m.display_name || m.username}
								avatar={m.avatar_url}
								class="inline-flex w-full min-w-0 max-w-full items-center gap-3 rounded-[var(--radius)] p-2 text-start transition-colors hover:bg-[var(--color-surface-subtle)]"
							>
								{#snippet trigger()}
									{#if m.avatar_url}
										<img src={m.avatar_url} alt={m.display_name || m.username} class="h-10 w-10 shrink-0 rounded-full object-cover" loading="lazy" />
									{:else}
										<span class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/10 text-sm font-medium text-[var(--color-primary)]">
											{initials(m.display_name || m.username)}
										</span>
									{/if}
									<span class="min-w-0 flex-1">
										<span class="block truncate text-sm font-medium text-[var(--color-text)]">{m.display_name || m.username}</span>
										<span class="block truncate text-xs text-[var(--color-muted)]">@{m.username}</span>
									</span>
									<ShieldCheck size={16} class="shrink-0 text-[var(--color-primary)]" />
								{/snippet}
							</UserHoverCard>
						</li>
					{/each}
				</ul>
			{/if}
		{:else}
			{#if data.loadError}
				<p class="py-12 text-center text-[var(--color-text-subtle)]">
					{t('tag.load_error', 'Failed to load tagged content. Please try again later.')}
				</p>
			{:else if allItems.length === 0}
				<EmptyState icon={Hash} title={t('tag.empty', 'No content with this tag yet.')} />
			{:else}
				<div class="flex flex-col gap-3">
					{#each allFeedItems as item (item.id)}
						<FeedCard {item} style="daily" layout="stream" />
					{/each}
				</div>

				{#if nextCursor}
					<div class="mt-6 flex justify-center">
						<button
							type="button"
							onclick={loadMore}
							disabled={loadingMore}
							aria-busy={loadingMore}
							class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
						>
							{#if loadingMore}
								<Loader2 size={14} class="animate-spin" />
								{t('common.loading', 'Loading…')}
							{:else}
								{t('tag.load_more', 'Load More')}
							{/if}
						</button>
					</div>
				{/if}
			{/if}
		{/if}
	</div>
</div>

<!-- Edit Tag Modal — visible only to users with tags.manage permission -->
<Modal bind:open={editOpen} title={t('tag.edit_title', 'Edit Tag')} size="lg">
			<div class="space-y-4">
				<!-- Name -->
				<div>
					<label for="edit-name" class="block text-sm font-medium text-[var(--color-text)]">{t('tag.name', 'Name')}</label>
					<HintIcon text={t('tags.edit_name_hint', 'The name shown in admin lists.')} />
					<input id="edit-name" type="text" bind:value={editName} class="mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2 text-sm text-[var(--color-text)]" />
				</div>

				<!-- Description -->
				<div>
					<label for="edit-desc" class="block text-sm font-medium text-[var(--color-text)]">{t('tag.description_label', 'Description')}</label>
					<HintIcon text={t('tags.edit_desc_hint', 'Short description shown on the record.')} />
					<textarea id="edit-desc" rows="2" bind:value={editDescription} class="mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2 text-sm text-[var(--color-text)]" placeholder={t('tag.description_placeholder', 'Optional description...')}></textarea>
				</div>

				<!-- Rules -->
				<div>
					<label for="edit-rules" class="block text-sm font-medium text-[var(--color-text)]">{t('tag.rules', 'Tag Rules')}</label>
					<HintIcon text={t('tags.edit_rules_hint', 'Matching rules — one per line.')} />
					<textarea id="edit-rules" rows="3" bind:value={editRules} class="mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2 text-sm text-[var(--color-text)]" placeholder={t('tag.rules_placeholder', 'Optional community rules/guidelines...')}></textarea>
				</div>

				<!-- Color -->
				<div>
					<label for="edit-color" class="block text-sm font-medium text-[var(--color-text)]">{t('tag.color', 'Color')}</label>
					<HintIcon text={t('tags.edit_color_hint', 'The tag color shown in listings.')} />
					<div class="mt-1 flex items-center gap-2">
						<input id="edit-color" type="color" bind:value={editColor} class="h-9 w-12 cursor-pointer rounded border border-[var(--color-border)] bg-[var(--color-surface)]" />
						<input type="text" bind:value={editColor} class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2 text-sm font-mono text-[var(--color-text)]" placeholder="#3b82f6" />
					</div>
				</div>

				<!-- Cover Image -->
				<div>
					<span class="block text-sm font-medium text-[var(--color-text)]">{t('tag.cover', 'Cover Image')}</span>
					<div class="mt-1 flex items-center gap-3">
						{#if editCoverUrl}
							<div class="relative">
								<img src={editCoverUrl} alt={tag.name} class="h-12 w-20 rounded-[var(--radius)] border border-[var(--color-border)] object-cover" />
								<button type="button" onclick={() => { editCoverMediaId = ''; editCoverUrl = ''; }} class="absolute -end-1 -top-1 rounded-full p-0.5" style="background: var(--color-danger); color: white" aria-label={t('common.remove', 'Remove')}>
									<X size={12} />
								</button>
							</div>
						{:else}
							<div class="grid h-12 w-20 place-items-center rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)]">
								<ImageIcon size={18} class="text-[var(--color-muted)]" />
							</div>
						{/if}
						<button type="button" onclick={() => (coverPickerOpen = true)} class="flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]">
							<ImageIcon size={14} />
							{t('tag.choose_cover', 'Choose Cover')}
						</button>
					</div>
				</div>

				<!-- Logo -->
				<div>
					<span class="block text-sm font-medium text-[var(--color-text)]">{t('tag.logo', 'Logo')}</span>
					<div class="mt-1 flex items-center gap-3">
						{#if editLogoUrl}
							<div class="relative">
								<img src={editLogoUrl} alt={tag.name} class="h-12 w-12 rounded-[var(--radius)] border border-[var(--color-border)] object-cover" />
								<button type="button" onclick={() => { editLogoMediaId = ''; editLogoUrl = ''; }} class="absolute -end-1 -top-1 rounded-full p-0.5" style="background: var(--color-danger); color: white" aria-label={t('common.remove', 'Remove')}>
									<X size={12} />
								</button>
							</div>
						{:else}
							<div class="grid h-12 w-12 place-items-center rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)]">
								<ImageIcon size={18} class="text-[var(--color-muted)]" />
							</div>
						{/if}
						<button type="button" onclick={() => (logoPickerOpen = true)} class="flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]">
							<ImageIcon size={14} />
							{t('tag.choose_logo', 'Choose Logo')}
						</button>
					</div>
				</div>

				<!-- Custom Fields -->
				{#if fieldDefs.length > 0}
					<div class="border-t border-[var(--color-border)] pt-3">
						<p class="mb-2 text-sm font-medium text-[var(--color-text)]">{t('tag.custom_fields', 'Custom Fields')}</p>
						{#each fieldDefs as field (field.key)}
							<div class="mb-3">
								<label for="edit-field-{field.key}" class="block text-xs font-medium text-[var(--color-muted)]">
									{field.label}{#if field.is_required} <span style="color: var(--color-danger)">*</span>{/if}
								</label>
								{#if field.field_type === 'rich_text' || field.field_type === 'textarea'}
									<textarea id="edit-field-{field.key}" rows="3" bind:value={editFieldValues[field.key]} class="mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2 text-sm text-[var(--color-text)]" placeholder={field.placeholder ?? ''}></textarea>
								{:else}
									<input id="edit-field-{field.key}" type={field.field_type === 'url' ? 'url' : 'text'} bind:value={editFieldValues[field.key]} class="mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2 text-sm text-[var(--color-text)]" placeholder={field.placeholder ?? ''} />
								{/if}
							</div>
						{/each}
					</div>
				{/if}

				{#if editError}
					<p
					class="rounded-[var(--radius)] px-3 py-2 text-sm"
					style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
				>
					{editError}
				</p>
				{/if}
			</div>

			<div class="mt-6 flex justify-end gap-2">
				<button type="button" onclick={() => (editOpen = false)} class="rounded-[var(--radius)] px-4 py-2 text-sm text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]">
					{t('common.cancel', 'Cancel')}
				</button>
				<button type="button" onclick={submitEdit} disabled={editSaving} class="flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-50">
					{#if editSaving}
						<Loader2 size={14} class="animate-spin" />
					{/if}
					{t('common.save', 'Save')}
				</button>
			</div>
</Modal>

{#if logoPickerOpen}
	<MediaLibraryModal onSelect={handleLogoSelect} onClose={() => (logoPickerOpen = false)} />
{/if}
{#if coverPickerOpen}
	<MediaLibraryModal onSelect={handleCoverSelect} onClose={() => (coverPickerOpen = false)} />
{/if}
